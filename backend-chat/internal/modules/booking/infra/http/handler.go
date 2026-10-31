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

	access "github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	create        *application.CreateAppointment
	availability  *application.ListAvailability
	professionals *application.ListProfessionals
	manage        *application.ManageAppointment
	timeout       time.Duration
	logger        *slog.Logger
	limiter       *requestLimiter
}

func NewHandler(create *application.CreateAppointment, availability *application.ListAvailability, professionals *application.ListProfessionals, manage *application.ManageAppointment, timeout time.Duration, logger *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{create: create, availability: availability, professionals: professionals, manage: manage, timeout: timeout, logger: logger, limiter: newRequestLimiter()}
}

func (h *Handler) Register(router chi.Router, publicGates ...func(http.Handler) http.Handler) {
	base := "/api/v1/public/shops/{slug}"
	for _, route := range []struct {
		path, method string
		handler      http.HandlerFunc
	}{
		{"/professionals", http.MethodGet, h.listProfessionals}, {"/availability", http.MethodGet, h.listAvailability},
		{"/appointments", http.MethodPost, h.createAppointment}, {"/appointments/lookup", http.MethodPost, h.lookupAppointment},
		{"/appointments/cancel", http.MethodPost, h.cancelAppointment},
	} {
		router.Options(base+route.path, access.Preflight(route.method))
		gates := []func(http.Handler) http.Handler{h.guard}
		if route.path == "/professionals" || route.path == "/availability" {
			gates = append(gates, publicGates...)
		}
		router.With(gates...).MethodFunc(route.method, base+route.path, route.handler)
	}
}

func (h *Handler) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !h.limiter.allow(r.RemoteAddr, r.Method == http.MethodPost) {
			w.Header().Set("Retry-After", "60")
			response.Error(w, 429, "rate_limited", "too many booking requests")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type receiptResponse struct {
	ID               uuid.UUID     `json:"id"`
	ShopName         string        `json:"shop_name"`
	Status           domain.Status `json:"status"`
	ServiceName      string        `json:"service_name"`
	DurationMinutes  int           `json:"duration_minutes"`
	PriceCents       int64         `json:"price_cents"`
	Currency         string        `json:"currency"`
	ProfessionalID   uuid.UUID     `json:"professional_id"`
	ProfessionalName string        `json:"professional_name"`
	StartsAt         time.Time     `json:"starts_at"`
	EndsAt           time.Time     `json:"ends_at"`
	ShopTimezone     string        `json:"shop_timezone"`
}

func receipt(a application.AppointmentReceipt) receiptResponse {
	return receiptResponse{ID: a.AppointmentID, ShopName: a.ShopName, Status: a.Status, ServiceName: a.Service.Name, DurationMinutes: a.Service.DurationMinutes,
		PriceCents: a.Service.PriceCents, Currency: a.Service.Currency, ProfessionalID: a.ProfessionalID, ProfessionalName: a.ProfessionalName,
		StartsAt: a.StartsAt, EndsAt: a.EndsAt, ShopTimezone: a.ShopTimezone}
}

type detailsResponse struct {
	Appointment receiptResponse `json:"appointment"`
	CanCancel   bool            `json:"can_cancel"`
}

func (h *Handler) listProfessionals(w http.ResponseWriter, r *http.Request) {
	h.professionalsWithScope(w, r, nil)
}

func (h *Handler) professionalsWithScope(w http.ResponseWriter, r *http.Request, staff *application.StaffBookingScope) {
	id, err := uuid.Parse(r.URL.Query().Get("service_id"))
	if err != nil {
		h.failure(w, application.ErrInvalidAvailability, "professionals")
		return
	}
	var result application.ProfessionalsOutput
	if staff == nil {
		result, err = h.professionals.Execute(r.Context(), chi.URLParam(r, "slug"), id)
	} else {
		result, err = h.professionals.ExecuteForStaff(r.Context(), chi.URLParam(r, "slug"), id, *staff)
	}
	if err != nil {
		h.failure(w, err, "professionals")
		return
	}
	type professional struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	items := make([]professional, 0, len(result.Professionals))
	for _, p := range result.Professionals {
		items = append(items, professional{p.ID, p.Name})
	}
	response.JSON(w, 200, struct {
		ShopName        string         `json:"shop_name"`
		ShopTimezone    string         `json:"shop_timezone"`
		Today           string         `json:"today"`
		LastBookingDate string         `json:"last_booking_date"`
		Professionals   []professional `json:"professionals"`
	}{result.ShopName, result.ShopTimezone, result.Today, result.LastBookingDate, items})
}

func (h *Handler) listAvailability(w http.ResponseWriter, r *http.Request) {
	h.availabilityWithScope(w, r, nil)
}

func (h *Handler) availabilityWithScope(w http.ResponseWriter, r *http.Request, staff *application.StaffBookingScope) {
	serviceID, err := uuid.Parse(r.URL.Query().Get("service_id"))
	if err != nil {
		h.failure(w, application.ErrInvalidAvailability, "availability")
		return
	}
	professionalID, err := uuid.Parse(r.URL.Query().Get("professional_id"))
	if err != nil {
		h.failure(w, application.ErrInvalidAvailability, "availability")
		return
	}
	input := application.ListAvailabilityInput{ShopSlug: chi.URLParam(r, "slug"), ServiceID: serviceID, ProfessionalID: professionalID, LocalDate: r.URL.Query().Get("date")}
	var result application.AvailabilityOutput
	if staff == nil {
		result, err = h.availability.Execute(r.Context(), input)
	} else {
		result, err = h.availability.ExecuteForStaff(r.Context(), input, *staff)
	}
	if err != nil {
		h.failure(w, err, "availability")
		return
	}
	type slot struct {
		StartsAt time.Time `json:"starts_at"`
		EndsAt   time.Time `json:"ends_at"`
	}
	slots := make([]slot, 0, len(result.Slots))
	for _, s := range result.Slots {
		slots = append(slots, slot{s.StartsAt, s.EndsAt})
	}
	response.JSON(w, 200, struct {
		ShopTimezone string `json:"shop_timezone"`
		Slots        []slot `json:"slots"`
	}{result.ShopTimezone, slots})
}

type createRequest struct {
	ServiceID      uuid.UUID `json:"service_id"`
	ProfessionalID uuid.UUID `json:"professional_id"`
	StartsAt       time.Time `json:"starts_at"`
	Customer       struct {
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		Phone        string `json:"phone"`
		PhoneCountry string `json:"phone_country"`
	} `json:"customer"`
}

func (h *Handler) createAppointment(w http.ResponseWriter, r *http.Request) {
	var input *createRequest
	if !decode(w, r, &input) {
		return
	}
	if input == nil {
		response.Error(w, 400, "invalid_body", "JSON object required")
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		h.failure(w, application.ErrInvalidAppointment, "create")
		return
	}
	result, err := h.create.Execute(
		r.Context(),
		application.CreateAppointmentInput{
			ShopSlug:       chi.URLParam(r, "slug"),
			ServiceID:      input.ServiceID,
			ProfessionalID: input.ProfessionalID,
			StartsAt:       input.StartsAt,
			IdempotencyKey: keys[0],
			Customer: application.CustomerInput{
				FirstName:    input.Customer.FirstName,
				LastName:     input.Customer.LastName,
				Phone:        input.Customer.Phone,
				PhoneCountry: input.Customer.PhoneCountry}})

	if err != nil {
		h.failure(w, err, "create")
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, status, struct {
		Appointment receiptResponse `json:"appointment"`
		Code        string          `json:"code"`
		Replayed    bool            `json:"replayed"`
	}{receipt(result.Appointment), result.Code, result.Replayed})
}

func (h *Handler) lookupAppointment(w http.ResponseWriter, r *http.Request) {
	h.manageAppointment(w, r, false)
}
func (h *Handler) cancelAppointment(w http.ResponseWriter, r *http.Request) {
	h.manageAppointment(w, r, true)
}
func (h *Handler) manageAppointment(w http.ResponseWriter, r *http.Request, cancel bool) {
	var input *struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input == nil {
		response.Error(w, 400, "invalid_body", "JSON object required")
		return
	}
	request := application.AppointmentCodeInput{ShopSlug: chi.URLParam(r, "slug"), Code: input.Code}
	var result application.AppointmentDetails
	var err error
	if cancel {
		result, err = h.manage.Cancel(r.Context(), request)
	} else {
		result, err = h.manage.Lookup(r.Context(), request)
	}
	if err != nil {
		h.failure(w, err, "manage")
		return
	}
	response.JSON(w, 200, detailsResponse{receipt(result.Appointment), result.CanCancel})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(target)
	if err == nil {
		err = decoder.Decode(new(any))
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		response.Error(w, 413, "body_too_large", "booking body exceeds 8 KiB")
	} else {
		response.Error(w, 400, "invalid_body", "one JSON object with known fields is required")
	}
	return false
}

func (h *Handler) failure(w http.ResponseWriter, err error, operation string) {
	status, code, message := 500, "internal_error", "could not complete booking operation"
	switch {
	case errors.Is(err, application.ErrStaffBookingAccessDenied):
		response.Error(w, 403, "access_denied", "staff booking access is required")
	case errors.Is(err, application.ErrShopNotFound):
		status, code, message = 404, "shop_not_found", "active shop not found"
	case errors.Is(err, application.ErrBookingOptionNotFound):
		status, code, message = 404, "booking_option_not_found", "booking selection not found"
	case errors.Is(err, application.ErrAppointmentNotFound):
		status, code, message = 404, "appointment_not_found", "appointment code not found"
	case errors.Is(err, application.ErrInvalidCustomer):
		status, code, message = 422, "invalid_customer", "check the customer name and phone"
	case errors.Is(err, application.ErrInvalidAvailability):
		status, code, message = 422, "invalid_availability", "check service, professional and date"
	case errors.Is(err, application.ErrInvalidAppointment), errors.Is(err, domain.ErrInvalidAppointment):
		status, code, message = 422, "invalid_appointment", "check selection, start and 64-character random request key"
	case errors.Is(err, application.ErrSlotUnavailable):
		status, code, message = 409, "slot_unavailable", "slot is no longer available"
	case errors.Is(err, application.ErrIdempotencyConflict):
		status, code, message = 409, "idempotency_conflict", "request key already used with different data"
	case errors.Is(err, application.ErrIdempotencyExpired):
		status, code, message = 409, "idempotency_expired", "request key expired; check the original appointment"
	case errors.Is(err, domain.ErrCancellationClosed):
		status, code, message = 409, "cancellation_closed", "appointment can no longer be cancelled"
	case errors.Is(err, application.ErrBookingEligibilityUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code, message = 503, "temporarily_unavailable", "booking request could not be completed"
	default:
		h.logger.Error("booking operation failed", "operation", operation)
	}
	response.Error(w, status, code, message)
}
