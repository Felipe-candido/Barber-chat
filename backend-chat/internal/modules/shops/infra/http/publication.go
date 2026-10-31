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

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type PublicationHandler struct {
	publication *application.ShopPublication
	timeout     time.Duration
	logger      *slog.Logger
}

func NewPublicationHandler(publication *application.ShopPublication, timeout time.Duration, logger *slog.Logger) *PublicationHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PublicationHandler{publication: publication, timeout: timeout, logger: logger}
}

func (h *PublicationHandler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := settingsScope(r)
	if !ok {
		response.Error(w, 403, "access_denied", "owner access is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	status, err := h.publication.Get(ctx, scope)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, 200, status)
}

func (h *PublicationHandler) Set(w http.ResponseWriter, r *http.Request) {
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
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body *struct {
		Requested *bool `json:"requested"`
	}
	err = decoder.Decode(&body)
	if err == nil {
		if next := decoder.Decode(new(any)); next != io.EOF {
			err = next
			if err == nil {
				err = errors.New("extra JSON value")
			}
		}
	}
	if err != nil || body == nil || body.Requested == nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			response.Error(w, 413, "body_too_large", "body exceeds 1 KiB")
		} else {
			response.Error(w, 400, "invalid_body", "one object with requested boolean is required")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.publication.Set(ctx, scope, *body.Requested); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *PublicationHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrSettingsAccessDenied):
		response.Error(w, 403, "access_denied", "owner access is required")
	case errors.Is(err, application.ErrSettingsNotFound):
		response.Error(w, 404, "shop_not_found", "shop was not found")
	case errors.Is(err, application.ErrPublicationNotManaged):
		response.Error(w, 409, "publication_not_managed", "legacy shop publication is managed by the operator")
	case errors.Is(err, application.ErrPublicationBlocked):
		response.Error(w, 409, "publication_blocked", "read commercial status and complete publication prerequisites")
	default:
		h.logger.ErrorContext(r.Context(), "shop publication request failed", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 503, "temporarily_unavailable", "publication could not be confirmed; read current status before retrying a write")
	}
}
