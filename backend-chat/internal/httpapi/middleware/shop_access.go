package middleware

import (
	"log/slog"
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ShopAccess requires Authentication first and resolves the requested path slug.
func ShopAccess(authorize *identityapp.AuthorizeShopAction, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := requestctx.StaffIdentityFromContext(r.Context())
			if !ok || identity.UserID == uuid.Nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
				response.Error(w, http.StatusUnauthorized, "authentication_required", "authentication is required")
				return
			}
			if authorize == nil {
				response.Error(w, http.StatusServiceUnavailable, "temporarily_unavailable", "authorization is unavailable")
				return
			}

			scope, err := authorize.Execute(r.Context(), identityapp.AuthorizeShopActionInput{
				UserID: identity.UserID, ShopSlug: chi.URLParam(r, "slug"),
			})
			if err != nil {
				identityhttp.WriteAccessError(w, r, logger, "authorize_shop", err)
				return
			}

			ctx := requestctx.WithShopScope(r.Context(), scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
