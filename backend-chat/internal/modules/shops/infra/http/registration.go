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
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type RegistrationHandler struct {
	create  *application.CreateShop
	timeout time.Duration
	logger  *slog.Logger
}

func NewRegistrationHandler(create *application.CreateShop, timeout time.Duration, logger *slog.Logger) *RegistrationHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RegistrationHandler{create: create, timeout: timeout, logger: logger}
}

func (h *RegistrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity, ok := requestctx.RegistrationIdentityFromContext(r.Context())
	if !ok || identity.UserID == uuid.Nil {
		response.Error(w, 401, "authentication_required", "verified registration identity is required")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json is required")
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || len(keys[0]) != 64 {
		response.Error(w, 422, "invalid_idempotency_key", "a 32-byte lowercase hexadecimal Idempotency-Key is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var input *struct {
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Timezone    string `json:"timezone"`
		Address     struct {
			PostalCode string `json:"postal_code"`
			Street     string `json:"street"`
			Number     string `json:"number"`
			Complement string `json:"complement"`
			District   string `json:"district"`
			City       string `json:"city"`
			State      string `json:"state"`
		} `json:"address"`
	}
	err = d.Decode(&input)
	if err == nil && input != nil {
		if nextErr := d.Decode(new(any)); nextErr != io.EOF {
			err = nextErr
			if err == nil {
				err = errors.New("extra JSON value")
			}
		}
	}
	if err != nil || input == nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			response.Error(w, 413, "body_too_large", "body exceeds limit")
		} else {
			response.Error(w, 400, "invalid_body", "one valid JSON object is required")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	output, err := h.create.Execute(ctx, application.CreateShopInput{OwnerID: identity.UserID, DisplayName: input.DisplayName,
		Name: input.Name, Slug: input.Slug, Timezone: input.Timezone, IdempotencyKey: keys[0],
		Address: domain.Address{PostalCode: input.Address.PostalCode, Street: input.Address.Street, Number: input.Address.Number,
			Complement: input.Address.Complement, District: input.Address.District, City: input.Address.City, State: input.Address.State}})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidAddress):
			response.Error(w, 422, "invalid_shop_address", "a complete Brazilian business address is required")
		case errors.Is(err, domain.ErrInvalidShop), errors.Is(err, application.ErrInvalidRegistration):
			response.Error(w, 422, "invalid_shop_registration", "check display name, shop name, slug, timezone and request key")
		case errors.Is(err, application.ErrOwnerInactive), errors.Is(err, application.ErrOwnershipDenied):
			response.Error(w, 403, "access_denied", "registration is not allowed")
		case errors.Is(err, application.ErrSlugTaken):
			response.Error(w, 409, "shop_slug_taken", "choose another shop slug")
		case errors.Is(err, application.ErrCreationConflict):
			response.Error(w, 409, "idempotency_conflict", "this request key was used with different input")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			response.Error(w, 503, "temporarily_unavailable", "registration outcome may be unknown; retry the same key and input")
		default:
			h.logger.ErrorContext(r.Context(), "shop registration failed", "request_id", chimiddleware.GetReqID(r.Context()))
			response.Error(w, 503, "temporarily_unavailable", "registration outcome may be unknown; retry the same key and input")
		}
		return
	}
	status := http.StatusCreated
	if output.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, status, struct {
		ShopID   uuid.UUID `json:"shop_id"`
		Name     string    `json:"name"`
		Slug     string    `json:"slug"`
		Timezone string    `json:"timezone"`
		Replayed bool      `json:"replayed"`
	}{output.Shop.ID, output.Shop.Name, output.Shop.Slug, output.Shop.Timezone, output.Replayed})
}

// Applied to public catalog/availability. New writes enforce eligibility inside
// booking's transaction after idempotency replay; lookup/cancel remain available.
func PublicBookingGate(check *application.CheckPublicBooking, timeout time.Duration, logger *slog.Logger) func(http.Handler) http.Handler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			if err := check.Execute(ctx, chi.URLParam(r, "slug")); err != nil {
				if errors.Is(err, application.ErrPublicShopNotFound) {
					response.Error(w, 404, "shop_not_found", "shop is not available")
				} else {
					logger.WarnContext(r.Context(), "public shop check unavailable", "request_id", chimiddleware.GetReqID(r.Context()))
					response.Error(w, 503, "temporarily_unavailable", "shop could not be checked")
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
