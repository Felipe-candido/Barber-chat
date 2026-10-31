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
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type SettingsHandler struct {
	get     *application.GetShopSettings
	update  *application.UpdateShopSettings
	timeout time.Duration
	logger  *slog.Logger
}

func NewSettingsHandler(get *application.GetShopSettings, update *application.UpdateShopSettings, timeout time.Duration, logger *slog.Logger) *SettingsHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SettingsHandler{get: get, update: update, timeout: timeout, logger: logger}
}

func settingsScope(r *http.Request) (application.SettingsScope, bool) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || actor.UserID != scope.UserID ||
		scope.ShopID == uuid.Nil || scope.Role != identity.RoleOwner {
		return application.SettingsScope{}, false
	}
	return application.SettingsScope{ActorID: actor.UserID, ShopID: scope.ShopID}, true
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := settingsScope(r)
	if !ok {
		response.Error(w, 403, "access_denied", "owner access is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	settings, err := h.get.Execute(ctx, scope)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeSettings(w, settings)
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	scope, ok := settingsScope(r)
	if !ok {
		response.Error(w, 403, "access_denied", "owner access is required")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body *struct {
		Version     *int64       `json:"version"`
		Name        *string      `json:"name"`
		Phone       *string      `json:"phone"`
		Description *string      `json:"description"`
		Address     *addressJSON `json:"address"`
	}
	err = decoder.Decode(&body)
	if err == nil && body != nil {
		if next := decoder.Decode(new(any)); next != io.EOF {
			err = next
			if err == nil {
				err = errors.New("extra JSON value")
			}
		}
	}
	if err != nil || body == nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			response.Error(w, 413, "body_too_large", "body exceeds 8 KiB")
		} else {
			response.Error(w, 400, "invalid_body", "one valid JSON object is required")
		}
		return
	}
	if body.Version == nil || body.Name == nil || body.Phone == nil || body.Description == nil {
		response.Error(w, 422, "invalid_shop_settings", "version, name, phone and description are required; empty contact fields are allowed")
		return
	}
	if body.Address == nil {
		response.Error(w, 422, "invalid_shop_address", "a complete address is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	settings, err := h.update.Execute(ctx, application.UpdateShopSettingsInput{Scope: scope, ExpectedVersion: *body.Version,
		Settings: domain.BasicSettings{Name: *body.Name, Phone: *body.Phone, Description: *body.Description, Address: body.Address.domainAddress()}})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeSettings(w, settings)
}

func writeSettings(w http.ResponseWriter, settings application.ShopSettings) {
	response.JSON(w, 200, struct {
		ShopID      uuid.UUID    `json:"shop_id"`
		Name        string       `json:"name"`
		Slug        string       `json:"slug"`
		Timezone    string       `json:"timezone"`
		Phone       string       `json:"phone"`
		Description string       `json:"description"`
		Address     *addressJSON `json:"address"`
		Version     int64        `json:"version"`
	}{settings.ShopID, settings.Name, settings.Slug, settings.Timezone, settings.Phone,
		settings.Description, addressResponse(settings.Address), settings.Version})
}

func (h *SettingsHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrSettingsAccessDenied):
		response.Error(w, 403, "access_denied", "owner access is required")
	case errors.Is(err, application.ErrSettingsNotFound):
		response.Error(w, 404, "shop_not_found", "shop settings were not found")
	case errors.Is(err, application.ErrSettingsConflict):
		response.Error(w, 409, "shop_settings_conflict", "settings changed; reload and review before saving again")
	case errors.Is(err, domain.ErrInvalidAddress):
		response.Error(w, 422, "invalid_shop_address", "a complete Brazilian address is required")
	case errors.Is(err, domain.ErrInvalidSettings), errors.Is(err, application.ErrInvalidSettingsVersion):
		response.Error(w, 422, "invalid_shop_settings", "check name, Brazilian phone, description and version")
	default:
		// Never log address, phone, body or raw database/provider errors.
		h.logger.ErrorContext(r.Context(), "shop settings request failed", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 503, "temporarily_unavailable", "settings could not be confirmed; read current settings before retrying a write")
	}
}
