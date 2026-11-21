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
	app "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ProfessionalManagementHandler struct {
	manage  *app.ManageProfessionals
	timeout time.Duration
	logger  *slog.Logger
}

func NewProfessionalManagementHandler(manage *app.ManageProfessionals, timeout time.Duration, logger *slog.Logger) *ProfessionalManagementHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ProfessionalManagementHandler{manage, timeout, logger}
}

type workingPeriodJSON struct {
	Weekday        int `json:"weekday"`
	OpensAtMinute  int `json:"opens_at_minute"`
	ClosesAtMinute int `json:"closes_at_minute"`
}
type professionalScheduleJSON struct {
	IntervalMinutes int                 `json:"interval_minutes"`
	BufferMinutes   int                 `json:"buffer_minutes"`
	WorkingHours    []workingPeriodJSON `json:"working_hours"`
}
type professionalDetailJSON struct {
	professionalResponse
	Schedule professionalScheduleJSON `json:"schedule"`
	Revision string                   `json:"revision"`
}

func professionalDetailResult(p app.ProfessionalDetail) professionalDetailJSON {
	s := professionalScheduleJSON{IntervalMinutes: p.Schedule.IntervalMinutes, BufferMinutes: p.Schedule.BufferMinutes, WorkingHours: make([]workingPeriodJSON, 0, len(p.Schedule.WorkingHours))}
	for _, w := range p.Schedule.WorkingHours {
		s.WorkingHours = append(s.WorkingHours, workingPeriodJSON{w.Weekday, w.OpensAtMinute, w.ClosesAtMinute})
	}
	return professionalDetailJSON{professionalResult(p.ProfessionalOutput), s, p.Revision}
}
func (h *ProfessionalManagementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	shop, ok := authorizedShopID(w, r)
	if !ok {
		return
	}
	actor, _ := requestctx.StaffIdentityFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "professionalID"))
	if err != nil || id == uuid.Nil {
		h.failure(w, app.ErrInvalidProfessionalEdit)
		return
	}
	if r.URL.RawQuery != "" {
		writeError(w, 400, "invalid_body", "query parameters are not supported")
		return
	}
	scope := app.ProfessionalEditScope{ActorID: actor.UserID, ShopID: shop}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if r.Method == http.MethodGet {
		p, err := h.manage.Get(ctx, scope, id)
		if err != nil {
			h.failure(w, err)
			return
		}
		writeJSON(w, 200, professionalDetailResult(p))
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, 415, "unsupported_media_type", "JSON required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input *struct {
		Name       string                    `json:"name"`
		Active     *bool                     `json:"active"`
		ServiceIDs []uuid.UUID               `json:"service_ids"`
		Schedule   *professionalScheduleJSON `json:"schedule"`
		Revision   string                    `json:"revision"`
	}
	if err := decoder.Decode(&input); err != nil {
		writeDecodeError(w, err)
		return
	}
	if input == nil {
		writeError(w, 400, "invalid_body", "JSON object required")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if input.Active == nil || input.Schedule == nil || input.ServiceIDs == nil || input.Schedule.WorkingHours == nil {
		h.failure(w, app.ErrInvalidProfessionalEdit)
		return
	}
	s := input.Schedule
	detail := app.ProfessionalDetail{ProfessionalOutput: app.ProfessionalOutput{ID: id, Name: input.Name, Active: *input.Active, ServiceIDs: input.ServiceIDs}, Revision: input.Revision, Schedule: app.ProfessionalSchedule{IntervalMinutes: s.IntervalMinutes, BufferMinutes: s.BufferMinutes, WorkingHours: make([]app.WorkingPeriod, 0, len(s.WorkingHours))}}
	for _, w := range s.WorkingHours {
		detail.Schedule.WorkingHours = append(detail.Schedule.WorkingHours, app.WorkingPeriod{Weekday: w.Weekday, OpensAtMinute: w.OpensAtMinute, ClosesAtMinute: w.ClosesAtMinute})
	}
	saved, err := h.manage.Update(ctx, scope, detail)
	if err != nil {
		h.failure(w, err)
		return
	}
	writeJSON(w, 200, professionalDetailResult(saved))
}
func (h *ProfessionalManagementHandler) failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrProfessionalNotFound), errors.Is(err, app.ErrShopNotFound):
		writeError(w, 404, "professional_not_found", "professional unavailable")
	case errors.Is(err, app.ErrProfessionalAccessDenied):
		writeError(w, 403, "access_denied", "owner required")
	case errors.Is(err, app.ErrProfessionalChanged):
		writeError(w, 409, "professional_changed", "read current settings before editing")
	case errors.Is(err, app.ErrInvalidProfessionalEdit), errors.Is(err, domain.ErrInvalidProfessional):
		writeError(w, 422, "invalid_professional_edit", "check profile and revision")
	case errors.Is(err, app.ErrInvalidProfessionalServices):
		writeError(w, 422, "invalid_professional_services", "choose services from this shop")
	case errors.Is(err, app.ErrInvalidProfessionalSchedule):
		writeError(w, 422, "invalid_professional_schedule", "check weekly periods and policy")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, 503, "temporarily_unavailable", "could not confirm changes")
	default:
		h.logger.Error("professional editing failed")
		writeError(w, 500, "internal_error", "could not complete operation")
	}
}
