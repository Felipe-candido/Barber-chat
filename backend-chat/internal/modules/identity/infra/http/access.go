package http

import (
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

func Access(w http.ResponseWriter, r *http.Request) {
	scope, ok := requestctx.ShopScopeFromContext(r.Context())
	identity, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	if !authenticated || !ok || identity.UserID == uuid.Nil || scope.UserID != identity.UserID ||
		scope.ShopID == uuid.Nil || !scope.Role.Valid() || (scope.Role == domain.RoleBarber && scope.ProfessionalID == uuid.Nil) {
		response.Error(w, http.StatusForbidden, "access_denied", "shop access is required")
		return
	}
	var professionalID *uuid.UUID
	if scope.ProfessionalID != uuid.Nil {
		professionalID = &scope.ProfessionalID
	}
	response.JSON(w, 200, struct {
		Role           string     `json:"role"`
		ProfessionalID *uuid.UUID `json:"professional_id"`
	}{string(scope.Role), professionalID})
}
