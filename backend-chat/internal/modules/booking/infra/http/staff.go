package http

import (
	"errors"
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handler) ListProfessionalsForStaff(w http.ResponseWriter, r *http.Request) {
	h.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, ok := staffBookingAuthority(r)
		if !ok {
			response.Error(w, 403, "access_denied", "staff shop access is required")
			return
		}
		h.professionalsWithScope(w, r, &scope)
	})).ServeHTTP(w, r)
}

func (h *Handler) ListAvailabilityForStaff(w http.ResponseWriter, r *http.Request) {
	h.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, ok := staffBookingAuthority(r)
		if !ok {
			response.Error(w, 403, "access_denied", "staff shop access is required")
			return
		}
		h.availabilityWithScope(w, r, &scope)
	})).ServeHTTP(w, r)
}

func staffBookingAuthority(r *http.Request) (application.StaffBookingScope, bool) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || scope.UserID != actor.UserID || scope.ShopID == uuid.Nil || !scope.Role.Valid() || (scope.Role == identity.RoleBarber && scope.ProfessionalID == uuid.Nil) {
		return application.StaffBookingScope{}, false
	}
	allowed := uuid.Nil
	if scope.Role == identity.RoleBarber {
		allowed = scope.ProfessionalID
	}
	return application.StaffBookingScope{UserID: actor.UserID, ShopID: scope.ShopID, ProfessionalID: allowed}, true
}

func (h *Handler) CreateForStaff(w http.ResponseWriter, r *http.Request) {
	h.guard(http.HandlerFunc(h.createForStaff)).ServeHTTP(w, r)
}

func (h *Handler) createForStaff(w http.ResponseWriter, r *http.Request) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || scope.UserID != actor.UserID || scope.ShopID == uuid.Nil || !scope.Role.Valid() ||
		(scope.Role == identity.RoleBarber && scope.ProfessionalID == uuid.Nil) {
		response.Error(w, 403, "access_denied", "staff shop access is required")
		return
	}
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
		h.failure(w, application.ErrInvalidAppointment, "staff_create")
		return
	}
	allowedProfessionalID := uuid.Nil
	if scope.Role == identity.RoleBarber {
		allowedProfessionalID = scope.ProfessionalID
	}
	saved, err := h.create.ExecuteForStaff(r.Context(), application.CreateAppointmentInput{
		ShopSlug: chi.URLParam(r, "slug"), ServiceID: input.ServiceID, ProfessionalID: input.ProfessionalID, StartsAt: input.StartsAt, IdempotencyKey: keys[0],
		Customer: application.CustomerInput{FirstName: input.Customer.FirstName, LastName: input.Customer.LastName, Phone: input.Customer.Phone, PhoneCountry: input.Customer.PhoneCountry},
	}, actor.UserID, scope.ShopID, allowedProfessionalID)
	if err != nil {
		if errors.Is(err, application.ErrStaffBookingAccessDenied) {
			response.Error(w, 403, "access_denied", "professional booking access is required")
		} else if errors.Is(err, application.ErrBookingSubscriptionRequired) {
			if scope.Role == identity.RoleOwner {
				response.Error(w, http.StatusPaymentRequired, "shop_subscription_required", "an active shop subscription is required; manage billing in shop settings")
			} else {
				response.Error(w, http.StatusForbidden, "shop_subscription_required", "contact the shop owner to enable new appointments")
			}
		} else {
			h.failure(w, err, "staff_create")
		}
		return
	}
	status := http.StatusCreated
	if saved.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, status, struct {
		Appointment receiptResponse `json:"appointment"`
		Code        string          `json:"code"`
		Replayed    bool            `json:"replayed"`
	}{receipt(saved.Appointment), saved.Code, saved.Replayed})
}
