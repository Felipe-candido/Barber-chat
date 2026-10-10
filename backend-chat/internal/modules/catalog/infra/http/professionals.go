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

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type ProfessionalHandler struct {
	create  *application.CreateProfessional
	list    *application.ListProfessionals
	timeout time.Duration
	logger  *slog.Logger
}

func NewProfessionalHandler(create *application.CreateProfessional, list *application.ListProfessionals, timeout time.Duration, logger *slog.Logger) *ProfessionalHandler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ProfessionalHandler{create, list, timeout, logger}
}

type professionalResponse struct {
	ID         uuid.UUID   `json:"id"`
	Name       string      `json:"name"`
	Active     bool        `json:"active"`
	ServiceIDs []uuid.UUID `json:"service_ids"`
}

func professionalResult(p application.ProfessionalOutput) professionalResponse {
	return professionalResponse{p.ID, p.Name, p.Active, p.ServiceIDs}
}

type professionalRequest struct {
	Name       string      `json:"name"`
	ServiceIDs []uuid.UUID `json:"service_ids"`
	Schedule   struct {
		IntervalMinutes int   `json:"interval_minutes"`
		BufferMinutes   int   `json:"buffer_minutes"`
		Weekdays        []int `json:"weekdays"`
		OpensAtMinute   int   `json:"opens_at_minute"`
		ClosesAtMinute  int   `json:"closes_at_minute"`
	} `json:"schedule"`
}

func (h *ProfessionalHandler) Create(w http.ResponseWriter, r *http.Request) {
	shopID, ok := authorizedShopID(w, r)
	if !ok {
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, 415, "unsupported_media_type", "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request *professionalRequest
	if err := decoder.Decode(&request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if request == nil {
		writeError(w, 400, "invalid_body", "JSON object required")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	s := request.Schedule
	result, err := h.create.Execute(ctx, application.CreateProfessionalInput{ShopID: shopID, Name: request.Name, ServiceIDs: request.ServiceIDs,
		Schedule: application.InitialSchedule{IntervalMinutes: s.IntervalMinutes, BufferMinutes: s.BufferMinutes, Weekdays: s.Weekdays, OpensAtMinute: s.OpensAtMinute, ClosesAtMinute: s.ClosesAtMinute}})
	if err != nil {
		h.failure(w, r, err, "create_professional")
		return
	}
	writeJSON(w, 201, professionalResult(result))
}
func (h *ProfessionalHandler) List(w http.ResponseWriter, r *http.Request) {
	shopID, ok := authorizedShopID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	items, err := h.list.Execute(ctx, shopID)
	if err != nil {
		h.failure(w, r, err, "list_professionals")
		return
	}
	result := make([]professionalResponse, 0, len(items))
	for _, item := range items {
		result = append(result, professionalResult(item))
	}
	writeJSON(w, 200, result)
}
func (h *ProfessionalHandler) failure(w http.ResponseWriter, r *http.Request, err error, operation string) {
	switch {
	case errors.Is(err, application.ErrShopNotFound):
		writeError(w, 404, "shop_not_found", "active shop not found")
	case errors.Is(err, domain.ErrInvalidProfessional):
		writeError(w, 422, "invalid_professional", "check the professional name")
	case errors.Is(err, application.ErrInvalidProfessionalServices):
		writeError(w, 422, "invalid_professional_services", "choose active services from this shop")
	case errors.Is(err, application.ErrInvalidProfessionalSchedule):
		writeError(w, 422, "invalid_professional_schedule", "check interval, buffer, weekdays and working hours")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		reason := "canceled"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "deadline_exceeded"
		}
		h.logger.WarnContext(r.Context(), "professional operation unavailable", "operation", operation,
			"reason", reason, "operation_timeout_ms", h.timeout.Milliseconds(), "request_id", chimiddleware.GetReqID(r.Context()))
		writeError(w, 503, "temporarily_unavailable", "professional request could not be completed")
	default:
		h.logger.ErrorContext(r.Context(), "professional operation failed", "operation", operation,
			"request_id", chimiddleware.GetReqID(r.Context()))
		writeError(w, 500, "internal_error", "could not complete professional operation")
	}
}
