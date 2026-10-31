package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type LifecycleHandler struct {
	manage  *application.ManageAppointmentStatus
	timeout time.Duration
	logger  *slog.Logger
}

func NewLifecycleHandler(manage *application.ManageAppointmentStatus, timeout time.Duration, logger *slog.Logger) *LifecycleHandler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LifecycleHandler{manage, timeout, logger}
}

type statusEventResponse struct {
	Version        int64               `json:"version"`
	PreviousStatus domain.Status       `json:"previous_status"`
	Status         domain.Status       `json:"status"`
	Source         domain.StatusSource `json:"source"`
	ActorID        *uuid.UUID          `json:"actor_id"`
	Reason         string              `json:"reason"`
	OccurredAt     time.Time           `json:"occurred_at"`
}
type lifecycleResponse struct {
	Appointment      adminAppointmentResponse `json:"appointment"`
	History          []statusEventResponse    `json:"history"`
	HistoryTruncated bool                     `json:"history_truncated"`
	Replayed         bool                     `json:"replayed"`
	ResultVersion    int64                    `json:"result_version"`
}

func lifecycleReceipt(out application.AppointmentLifecycleOutput) lifecycleResponse {
	history := make([]statusEventResponse, 0, len(out.History))
	for _, e := range out.History {
		var actor *uuid.UUID
		if e.ActorID != uuid.Nil {
			id := e.ActorID
			actor = &id
		}
		history = append(history, statusEventResponse{e.Version, e.PreviousStatus, e.Status, e.Source, actor, e.Reason, e.OccurredAt})
	}
	return lifecycleResponse{adminReceipt(out.Appointment), history, out.HistoryTruncated, out.Replayed, out.ResultVersion}
}
func lifecycleScope(w http.ResponseWriter, r *http.Request) (application.LifecycleScope, uuid.UUID, bool) {
	a, ok := requestctx.StaffIdentityFromContext(r.Context())
	s, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !ok || !authorized || a.UserID == uuid.Nil || s.ShopID == uuid.Nil || s.UserID != a.UserID {
		response.Error(w, 403, "access_denied", "shop access required")
		return application.LifecycleScope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "appointmentID"))
	if err != nil || id == uuid.Nil {
		response.Error(w, 400, "invalid_appointment_id", "appointment UUID required")
		return application.LifecycleScope{}, uuid.Nil, false
	}
	if r.URL.RawQuery != "" {
		response.Error(w, 400, "invalid_appointment_query", "query parameters are not supported")
		return application.LifecycleScope{}, uuid.Nil, false
	}
	return application.LifecycleScope{ActorID: a.UserID, ShopID: s.ShopID}, id, true
}
func (h *LifecycleHandler) Get(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := lifecycleScope(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.manage.Detail(ctx, scope, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, 200, lifecycleReceipt(out))
}
func (h *LifecycleHandler) Change(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := lifecycleScope(w, r)
	if !ok {
		return
	}
	var body *struct {
		Status          domain.Status `json:"status"`
		ExpectedVersion *int64        `json:"expected_version"`
		Reason          string        `json:"reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body == nil || body.ExpectedVersion == nil {
		response.Error(w, 400, "invalid_body", "status, expected_version and reason required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.manage.Change(ctx, application.ChangeAppointmentStatusInput{Scope: scope, AppointmentID: id, Status: body.Status, ExpectedVersion: *body.ExpectedVersion, Reason: body.Reason, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, 200, lifecycleReceipt(out))
}
func (h *LifecycleHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrAppointmentAccessDenied):
		response.Error(w, 403, "access_denied", "appointment access denied")
	case errors.Is(err, application.ErrAppointmentNotFound):
		response.Error(w, 404, "appointment_not_found", "appointment not found in this shop")
	case errors.Is(err, application.ErrInvalidStatusCommandKey):
		response.Error(w, 400, "invalid_idempotency_key", "64 lowercase hex characters required")
	case errors.Is(err, domain.ErrInvalidStatusChange):
		response.Error(w, 422, "invalid_appointment_status", "valid status, version and printable reason required")
	case errors.Is(err, domain.ErrAppointmentVersionConflict):
		response.Error(w, 409, "appointment_version_changed", "reload current appointment before correcting")
	case errors.Is(err, domain.ErrAppointmentAlreadyInStatus):
		response.Error(w, 409, "appointment_status_unchanged", "appointment already has this status")
	case errors.Is(err, domain.ErrAppointmentTransitionTooEarly):
		response.Error(w, 409, "appointment_transition_too_early", "completion requires reserved end; absence requires start")
	case errors.Is(err, application.ErrStatusCommandConflict):
		response.Error(w, 409, "idempotency_conflict", "status key used with different input")
	case errors.Is(err, application.ErrStatusCommandPending):
		response.Error(w, 409, "appointment_command_pending", "recover original status command")
	case errors.Is(err, application.ErrSlotUnavailable):
		response.Error(w, 409, "appointment_interval_conflict", "another appointment already occupies this interval")
	default:
		h.logger.ErrorContext(r.Context(), "appointment status request unavailable", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 503, "appointment_status_unavailable", "result may have been saved; preserve original key and input")
	}
}
