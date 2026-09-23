package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 64 << 10

type Handler struct {
	createService     *application.CreateService
	listServices      *application.ListServices
	devShopSlug       string
	devFrontendOrigin string
	timeout           time.Duration
	logger            *slog.Logger
}

func NewHandler(create *application.CreateService, list *application.ListServices, devShopSlug string, timeout time.Duration, logger *slog.Logger, frontendOrigin ...string) *Handler {
	h := &Handler{createService: create, listServices: list, devShopSlug: devShopSlug, timeout: timeout, logger: logger}
	if len(frontendOrigin) > 0 {
		h.devFrontendOrigin = frontendOrigin[0]
	}
	return h
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
	if h.devShopSlug == "" || !localRequest(r, h.devFrontendOrigin) {
		writeError(w, http.StatusForbidden, "admin_access_unavailable", "local catalog writes are disabled or this request is not local")
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
			ShopSlug:        h.devShopSlug,
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

// Local development access is not authentication. Never expose it through a proxy.
func localRequest(r *http.Request, allowedOrigin string) bool {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(remote).IsLoopback() {
		return false
	}
	host := r.Host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return allowedOrigin != "" && origin == allowedOrigin
	}
	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
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
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
