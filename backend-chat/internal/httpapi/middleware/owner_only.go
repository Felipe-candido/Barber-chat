package middleware

import (
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

// OwnerOnly runs after authentication and shop access. Reading the role from
// the verified scope prevents headers, token metadata or request bodies from
// promoting a barber to owner.
func OwnerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, authenticated := requestctx.StaffIdentityFromContext(r.Context())
		scope, authorized := requestctx.ShopScopeFromContext(r.Context())
		if !authenticated || !authorized || identity.UserID == uuid.Nil ||
			scope.UserID != identity.UserID || scope.ShopID == uuid.Nil || scope.Role != domain.RoleOwner {
			response.Error(w, http.StatusForbidden, "access_denied", "owner access is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
