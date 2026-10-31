package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type checkoutStarter interface {
	Execute(context.Context, application.StartCheckoutInput) (application.CheckoutOutput, error)
	Offer(context.Context, application.BillingScope) (application.CheckoutOffer, error)
}

type CheckoutHandler struct {
	start   checkoutStarter
	timeout time.Duration
	logger  *slog.Logger
}

func NewCheckoutHandler(start checkoutStarter, timeout time.Duration, logger *slog.Logger) *CheckoutHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CheckoutHandler{start: start, timeout: timeout, logger: logger}
}

func (h *CheckoutHandler) Start(w http.ResponseWriter, r *http.Request) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || actor.UserID != scope.UserID ||
		scope.ShopID == uuid.Nil || scope.Role != identity.RoleOwner {
		response.Error(w, 403, "access_denied", "owner access is required")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body *struct {
		Intent  domain.CheckoutIntent `json:"intent"`
		OfferID string                `json:"offer_id"`
	}
	err = decoder.Decode(&body)
	if err == nil && body != nil {
		if trailing := decoder.Decode(new(any)); trailing != io.EOF {
			err = trailing
			if err == nil {
				err = errors.New("extra JSON data")
			}
		}
	}
	if err != nil || body == nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			response.Error(w, 413, "body_too_large", "body exceeds 1 KiB")
		} else {
			response.Error(w, 400, "invalid_body", "one JSON object containing only intent and offer_id is required")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.start.Execute(ctx, application.StartCheckoutInput{Scope: application.BillingScope{ActorID: actor.UserID, ShopID: scope.ShopID}, Intent: body.Intent, OfferID: body.OfferID})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, 200, struct {
		AttemptID uuid.UUID             `json:"attempt_id"`
		Intent    domain.CheckoutIntent `json:"intent"`
		URL       string                `json:"url"`
		ExpiresAt time.Time             `json:"expires_at"`
		LiveMode  bool                  `json:"livemode"`
	}{result.AttemptID, result.Intent, result.URL, result.ExpiresAt, false})
}

func (h *CheckoutHandler) Offer(w http.ResponseWriter, r *http.Request) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || actor.UserID != scope.UserID || scope.ShopID == uuid.Nil || scope.Role != identity.RoleOwner {
		response.Error(w, 403, "access_denied", "owner access is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	offer, err := h.start.Offer(ctx, application.BillingScope{ActorID: actor.UserID, ShopID: scope.ShopID})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, 200, struct {
		ShopID        uuid.UUID `json:"shop_id"`
		OfferID       string    `json:"offer_id"`
		PlanKey       string    `json:"plan_key"`
		AmountCents   int64     `json:"amount_cents"`
		Currency      string    `json:"currency"`
		Interval      string    `json:"interval"`
		IntervalCount int       `json:"interval_count"`
		LiveMode      bool      `json:"livemode"`
		Recovering    bool      `json:"recovering"`
	}{offer.ShopID, offer.OfferID, offer.PlanKey, offer.AmountCents, "BRL", "month", 1, false, offer.Recovering})
}

func (h *CheckoutHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidCheckout):
		response.Error(w, 422, "invalid_checkout_intent", "intent must be publish or manual_booking and offer_id must be valid")
	case errors.Is(err, application.ErrCheckoutOfferChanged):
		response.Error(w, 409, "checkout_offer_changed", "review the current server-selected offer before continuing")
	case errors.Is(err, application.ErrBillingAccessDenied):
		response.Error(w, 403, "access_denied", "owner access is required")
	case errors.Is(err, application.ErrBillingNotRequired):
		response.Error(w, 409, "billing_not_required", "billing rollout for manually provisioned shops is not enabled")
	case errors.Is(err, application.ErrSubscriptionExists):
		response.Error(w, 409, "subscription_exists", "manage the existing subscription instead of opening another Checkout")
	case errors.Is(err, application.ErrBillingNotConfigured):
		response.Error(w, 503, "billing_not_configured", "test checkout is not configured")
	case errors.Is(err, application.ErrCheckoutPending):
		response.Error(w, 409, "checkout_confirmation_pending", "Checkout completed; payment confirmation is still required")
	case errors.Is(err, application.ErrCheckoutReview):
		response.Error(w, 409, "checkout_requires_review", "the original attempt requires reconciliation; do not open another subscription")
	case errors.Is(err, application.ErrCheckoutExpired):
		response.Error(w, 409, "checkout_expired", "Stripe confirmed expiration; a new explicit request may open another Checkout")
	default:
		// A provider request ID can locate the failure in Workbench. Never log
		// raw errors/messages, payloads, URLs, resource IDs, secrets or PII.
		fields := []any{"request_id", chimiddleware.GetReqID(r.Context())}
		var failure *application.ProviderFailure
		if errors.As(err, &failure) {
			fields = append(fields, "provider_status", failure.HTTPStatus, "provider_code", failure.Code,
				"provider_parameter", failure.Parameter, "provider_request_id", failure.RequestID)
		}
		h.logger.WarnContext(r.Context(), "checkout could not be confirmed", fields...)
		response.Error(w, 503, "checkout_unavailable", "retry this endpoint to recover the original attempt; do not create a subscription elsewhere")
	}
}
