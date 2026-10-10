package http

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type AgendaHandler struct {
	list    *application.ListAgenda
	timeout time.Duration
	logger  *slog.Logger
}

func NewAgendaHandler(list *application.ListAgenda, timeout time.Duration, logger *slog.Logger) *AgendaHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AgendaHandler{list: list, timeout: timeout, logger: logger}
}

type adminAppointmentResponse struct {
	receiptResponse
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`
}
type agendaResponse struct {
	Month        string                     `json:"month"`
	Today        string                     `json:"today"`
	FromDate     string                     `json:"from_date"`
	UntilDate    string                     `json:"until_date"`
	ShopTimezone string                     `json:"shop_timezone"`
	Appointments []adminAppointmentResponse `json:"appointments"`
	NextCursor   *string                    `json:"next_cursor"`
}

func (h *AgendaHandler) List(w http.ResponseWriter, r *http.Request) {
	identity, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || identity.UserID == uuid.Nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
		response.Error(w, 401, "authentication_required", "authentication is required")
		return
	}
	if !authorized || scope.ShopID == uuid.Nil || scope.UserID != identity.UserID || !scope.Role.Valid() ||
		(scope.Role == identitydomain.RoleBarber && scope.ProfessionalID == uuid.Nil) {
		response.Error(w, 403, "access_denied", "shop access is required")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err == nil {
		for key, values := range query {
			if (key != "month" && key != "after") || len(values) != 1 || values[0] == "" {
				err = application.ErrInvalidAgenda
				break
			}
		}
	}
	var cursor *application.AgendaCursor
	if err == nil && query.Has("after") {
		cursor, err = decodeAgendaCursor(query.Get("after"))
	}
	if err != nil {
		response.Error(w, 422, "invalid_agenda", "check month and pagination cursor")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	professionalID := uuid.Nil
	if scope.Role == identitydomain.RoleBarber {
		professionalID = scope.ProfessionalID
	}
	output, err := h.list.Execute(ctx, application.ListAgendaInput{ShopID: scope.ShopID, ProfessionalID: professionalID, Month: query.Get("month"), After: cursor})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidAgenda):
			response.Error(w, 422, "invalid_agenda", "check month and pagination cursor")
		case errors.Is(err, application.ErrShopNotFound):
			response.Error(w, 404, "shop_not_found", "active shop not found")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			h.logger.WarnContext(ctx, "agenda operation unavailable", "request_id", chimiddleware.GetReqID(ctx))
			response.Error(w, 503, "temporarily_unavailable", "could not read agenda")
		default:
			h.logger.ErrorContext(ctx, "agenda operation failed", "request_id", chimiddleware.GetReqID(ctx))
			response.Error(w, 500, "internal_error", "could not read agenda")
		}
		return
	}
	result := agendaResponse{Month: output.Month, Today: output.Today, FromDate: output.FromDate,
		UntilDate: output.UntilDate, ShopTimezone: output.ShopTimezone, Appointments: make([]adminAppointmentResponse, 0, len(output.Appointments))}
	for _, item := range output.Appointments {
		result.Appointments = append(result.Appointments, adminAppointmentResponse{receipt(item.Appointment), item.CustomerName, item.CustomerPhone})
	}
	if output.Next != nil {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(output.Next.StartsAt.UTC().Format(time.RFC3339Nano) + "|" + output.Next.ID.String()))
		result.NextCursor = &encoded
	}
	response.JSON(w, 200, result)
}

func decodeAgendaCursor(value string) (*application.AgendaCursor, error) {
	if len(value) > 160 {
		return nil, application.ErrInvalidAgenda
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	parts := strings.Split(string(raw), "|")
	if err != nil || len(parts) != 2 {
		return nil, application.ErrInvalidAgenda
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, application.ErrInvalidAgenda
	}
	id, err := uuid.Parse(parts[1])
	if err != nil || id == uuid.Nil {
		return nil, application.ErrInvalidAgenda
	}
	return &application.AgendaCursor{StartsAt: at.UTC(), ID: id}, nil
}
