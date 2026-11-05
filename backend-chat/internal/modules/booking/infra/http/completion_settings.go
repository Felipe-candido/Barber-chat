package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

type CompletionSettingsHandler struct {
	manage  *app.ManageCompletionSettings
	timeout time.Duration
	logger  *slog.Logger
}

func NewCompletionSettingsHandler(manage *app.ManageCompletionSettings, timeout time.Duration, logger *slog.Logger) *CompletionSettingsHandler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CompletionSettingsHandler{manage, timeout, logger}
}

type completionSettingsResponse struct {
	ShopID         uuid.UUID             `json:"shop_id"`
	Mode           domain.CompletionMode `json:"mode"`
	Version        int64                 `json:"version"`
	AutomaticSince *time.Time            `json:"automatic_since"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

func (h *CompletionSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := requestctx.StaffIdentityFromContext(r.Context())
	shop, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !ok || !authorized || actor.UserID == uuid.Nil || shop.ShopID == uuid.Nil || shop.UserID != actor.UserID {
		response.Error(w, 403, "access_denied", "shop access required")
		return
	}
	if r.URL.RawQuery != "" {
		response.Error(w, 400, "invalid_query", "query parameters are not supported")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	scope := app.CompletionSettingsScope{ActorID: actor.UserID, ShopID: shop.ShopID}
	var out app.CompletionSettings
	var err error
	switch r.Method {
	case http.MethodGet:
		out, err = h.manage.Read(ctx, scope)
	case http.MethodPut:
		var body *struct {
			Mode    domain.CompletionMode `json:"mode"`
			Version *int64                `json:"version"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body == nil || body.Version == nil {
			response.Error(w, 400, "invalid_body", "mode and version required")
			return
		}
		out, err = h.manage.Save(ctx, scope, body.Mode, *body.Version)
	default:
		w.Header().Set("Allow", "GET, PUT")
		response.Error(w, 405, "method_not_allowed", "method not allowed")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, app.ErrCompletionSettingsAccessDenied):
			response.Error(w, 403, "access_denied", "completion settings access denied")
		case errors.Is(err, app.ErrInvalidCompletionSettings):
			response.Error(w, 422, "invalid_completion_settings", "valid mode and version required")
		case errors.Is(err, app.ErrCompletionSettingsConflict):
			response.Error(w, 409, "completion_settings_changed", "reload current settings before saving")
		default:
			h.logger.ErrorContext(ctx, "completion settings request failed", "method", r.Method)
			response.Error(w, 503, "temporarily_unavailable", "completion settings could not be confirmed")
		}
		return
	}
	var since *time.Time
	if !out.AutomaticSince.IsZero() {
		at := out.AutomaticSince
		since = &at
	}
	response.JSON(w, 200, completionSettingsResponse{ShopID: out.ShopID, Mode: out.Mode, Version: out.Version, AutomaticSince: since, UpdatedAt: out.UpdatedAt})
}
