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
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type portalOpener interface {
	Execute(context.Context, application.BillingScope) (application.PortalOutput, error)
}

type PortalHandler struct {
	open    portalOpener
	timeout time.Duration
	logger  *slog.Logger
}

func NewPortalHandler(open portalOpener, timeout time.Duration, logger *slog.Logger) *PortalHandler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PortalHandler{open, timeout, logger}
}

func (h *PortalHandler) Open(w http.ResponseWriter, r *http.Request) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || actor.UserID != scope.UserID || scope.ShopID == uuid.Nil || scope.Role != identity.RoleOwner {
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
	var body *struct{}
	err = decoder.Decode(&body)
	if err == nil && body != nil && decoder.Decode(new(any)) != io.EOF {
		err = errors.New("extra JSON data")
	}
	if err != nil || body == nil {
		response.Error(w, 400, "invalid_body", "one empty JSON object is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.open.Execute(ctx, application.BillingScope{ActorID: actor.UserID, ShopID: scope.ShopID})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrBillingAccessDenied):
			response.Error(w, 403, "access_denied", "owner access is required")
		case errors.Is(err, application.ErrBillingNotRequired):
			response.Error(w, 409, "billing_not_required", "billing is not enabled for manually provisioned shops")
		case errors.Is(err, application.ErrPortalNotAvailable):
			response.Error(w, 409, "portal_not_available", "a reconciled subscription is required")
		case errors.Is(err, application.ErrPortalNotConfigured):
			response.Error(w, 503, "portal_not_configured", "test customer portal is not configured")
		default:
			fields := []any{"request_id", chimiddleware.GetReqID(r.Context())}
			var failure *application.ProviderFailure
			if errors.As(err, &failure) {
				fields = append(fields, "provider_status", failure.HTTPStatus, "provider_code", failure.Code, "provider_request_id", failure.RequestID)
			}
			h.logger.WarnContext(ctx, "customer portal could not be opened", fields...)
			response.Error(w, 503, "portal_unavailable", "customer portal could not be opened; no subscription change was requested")
		}
		return
	}
	response.JSON(w, 200, struct {
		ShopID    uuid.UUID `json:"shop_id"`
		URL       string    `json:"url"`
		OpenUntil time.Time `json:"open_until"`
		LiveMode  bool      `json:"livemode"`
	}{result.ShopID, result.URL, result.OpenUntil, false})
}
