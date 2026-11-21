package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type TimeOffHandler struct {
	manage  *app.ManageTimeOff
	timeout time.Duration
	logger  *slog.Logger
}

type timeOffBlockResponse struct {
	ID               uuid.UUID `json:"id"`
	ProfessionalID   uuid.UUID `json:"professional_id"`
	ProfessionalName string    `json:"professional_name"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
}
type timeOffProfessionalResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
type timeOffResponse struct {
	Today         string                        `json:"today"`
	MaxDate       string                        `json:"max_date"`
	ShopTimezone  string                        `json:"shop_timezone"`
	Professionals []timeOffProfessionalResponse `json:"professionals"`
	Blocks        []timeOffBlockResponse        `json:"blocks"`
}

func timeOffBlock(b app.TimeOffBlock) timeOffBlockResponse {
	return timeOffBlockResponse{b.ID, b.ProfessionalID, b.ProfessionalName, b.StartsAt, b.EndsAt}
}

func NewTimeOffHandler(manage *app.ManageTimeOff, timeout time.Duration, logger *slog.Logger) *TimeOffHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TimeOffHandler{manage, timeout, logger}
}
func (h *TimeOffHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	scope, ok := staffBookingAuthority(r)
	if !ok {
		response.Error(w, 403, "access_denied", "staff access required")
		return
	}
	if r.URL.RawQuery != "" {
		response.Error(w, 400, "invalid_time_off", "query parameters are not supported")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if r.Method == http.MethodGet {
		out, err := h.manage.List(ctx, scope)
		if err != nil {
			h.failure(w, err)
			return
		}
		result := timeOffResponse{Today: out.Today, MaxDate: out.MaxDate, ShopTimezone: out.ShopTimezone, Professionals: make([]timeOffProfessionalResponse, 0, len(out.Professionals)), Blocks: make([]timeOffBlockResponse, 0, len(out.Blocks))}
		for _, p := range out.Professionals {
			result.Professionals = append(result.Professionals, timeOffProfessionalResponse{p.ID, p.Name})
		}
		for _, b := range out.Blocks {
			result.Blocks = append(result.Blocks, timeOffBlock(b))
		}
		response.JSON(w, 200, result)
		return
	}
	professional, e1 := uuid.Parse(chi.URLParam(r, "professionalID"))
	id, e2 := uuid.Parse(chi.URLParam(r, "blockID"))
	if e1 != nil || e2 != nil || id == uuid.Nil || professional == uuid.Nil {
		h.failure(w, domain.ErrInvalidTimeOff)
		return
	}
	if r.Method == http.MethodDelete {
		if err := h.manage.Delete(ctx, scope, professional, id); err != nil {
			h.failure(w, err)
			return
		}
		w.WriteHeader(204)
		return
	}
	var input struct {
		FromDate    string `json:"from_date"`
		ThroughDate string `json:"through_date"`
	}
	if !decode(w, r, &input) {
		return
	}
	out, err := h.manage.Save(ctx, scope, professional, id, input.FromDate, input.ThroughDate)
	if err != nil {
		h.failure(w, err)
		return
	}
	response.JSON(w, 200, timeOffBlock(out))
}
func (h *TimeOffHandler) failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrStaffBookingAccessDenied):
		response.Error(w, 403, "access_denied", "staff access required")
	case errors.Is(err, domain.ErrInvalidTimeOff):
		response.Error(w, 422, "invalid_time_off", "check dates and professional")
	case errors.Is(err, app.ErrTimeOffAppointments):
		response.Error(w, 409, "time_off_has_appointments", "period contains appointments")
	case errors.Is(err, app.ErrTimeOffOverlap):
		response.Error(w, 409, "time_off_overlap", "period already blocked")
	case errors.Is(err, app.ErrTimeOffConflict):
		response.Error(w, 409, "time_off_conflict", "resource already used")
	case errors.Is(err, app.ErrBookingOptionNotFound), errors.Is(err, app.ErrShopNotFound):
		response.Error(w, 404, "booking_option_not_found", "professional unavailable")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		response.Error(w, 503, "temporarily_unavailable", "could not confirm time off")
	default:
		h.logger.Error("time off operation failed")
		response.Error(w, 500, "internal_error", "could not complete operation")
	}
}
