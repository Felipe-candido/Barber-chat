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
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxBodyBytes = 64 << 10

type Handler struct {
	createService          *application.CreateService
	listServices           *application.ListServices
	listAuthorizedServices *application.ListAuthorizedServices
	timeout                time.Duration
	logger                 *slog.Logger
}

func NewHandler(create *application.CreateService, list *application.ListServices, authorizedList *application.ListAuthorizedServices, timeout time.Duration, logger *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{createService: create, listServices: list, listAuthorizedServices: authorizedList, timeout: timeout, logger: logger}
}

type createServiceRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	DurationMinutes int    `json:"duration_minutes"`
	PriceCents      *int64 `json:"price_cents"`
}
type serviceResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DurationMinutes int    `json:"duration_minutes"`
	PriceCents      int64  `json:"price_cents"`
	Currency        string `json:"currency"`
	Active          bool   `json:"active"`
}

func responseOf(s application.ServiceOutput) serviceResponse {
	return serviceResponse{ID: s.ID.String(), Name: s.Name, Description: s.Description,
		DurationMinutes: s.DurationMinutes, PriceCents: s.PriceCents, Currency: s.Currency, Active: s.Active}
}

func (h *Handler) CreateService(w http.ResponseWriter, r *http.Request) {
	shopID, ok := authorizedShopID(w, r)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request *createServiceRequest
	if err := decoder.Decode(&request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if request == nil {
		writeError(w, 400, "invalid_body", "body must be a JSON object")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if request.PriceCents == nil {
		writeError(w, 422, "invalid_service", "price_cents is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	service, err := h.createService.Execute(
		ctx, application.CreateServiceInput{
			ShopID:          shopID,
			Name:            request.Name,
			Description:     request.Description,
			DurationMinutes: request.DurationMinutes,
			PriceCents:      *request.PriceCents,
		})

	if err != nil {
		h.writeOperationError(w, err, "create")
		return
	}
	writeJSON(w, http.StatusCreated, responseOf(service))
}

func authorizedShopID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	identity, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || identity.UserID == uuid.Nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
		writeError(w, http.StatusUnauthorized, "authentication_required", "authentication is required")
		return uuid.Nil, false
	}
	if !authorized || scope.ShopID == uuid.Nil || scope.UserID != identity.UserID {
		writeError(w, http.StatusForbidden, "access_denied", "shop access is required")
		return uuid.Nil, false
	}
	return scope.ShopID, true
}

func (h *Handler) ListShopServices(w http.ResponseWriter, r *http.Request) {
	shopID, ok := authorizedShopID(w, r)
	if !ok {
		return
	}
	if h.listAuthorizedServices == nil {
		writeError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "catalog is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	services, err := h.listAuthorizedServices.Execute(ctx, shopID)
	if err != nil {
		h.writeOperationError(w, err, "list_authorized")
		return
	}
	items := make([]serviceResponse, 0, len(services))
	for _, service := range services {
		items = append(items, responseOf(service))
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	services, err := h.listServices.Execute(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		h.writeOperationError(w, err, "list")
		return
	}
	response := make([]serviceResponse, 0, len(services))
	for _, s := range services {
		response = append(response, responseOf(s))
	}
	writeJSON(w, http.StatusOK, response)
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, 413, "body_too_large", "request body exceeds 64 KiB")
		return
	}
	writeError(w, 400, "invalid_body", "body must contain exactly one JSON object with known fields")
}
func (h *Handler) writeOperationError(w http.ResponseWriter, err error, operation string) {
	switch {
	case errors.Is(err, application.ErrShopNotFound):
		writeError(w, 404, "shop_not_found", "active shop not found")
	case errors.Is(err, domain.ErrInvalidShopID), errors.Is(err, domain.ErrInvalidName),
		errors.Is(err, domain.ErrInvalidDuration), errors.Is(err, domain.ErrInvalidPrice):
		writeError(w, 422, "invalid_service", err.Error())
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeError(w, 503, "temporarily_unavailable", "catalog request could not be completed")
	default:
		h.logger.Error("catalog operation failed", "operation", operation)
		writeError(w, 500, "internal_error", "could not complete catalog operation")
	}
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	response.Error(w, status, code, message)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	response.JSON(w, status, value)
}
