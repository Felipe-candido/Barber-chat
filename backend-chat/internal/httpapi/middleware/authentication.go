package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
)

const maxAuthorizationBytes = 32*1024 + 32

// Authentication resolves an active local user from a Bearer access token.
// Reuse the use case and its verifier across requests; identity is request-local.
func Authentication(authenticate *identityapp.Authenticate, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
				response.Error(w, http.StatusUnauthorized, "authentication_required", "a Bearer access token is required")
				return
			}
			if authenticate == nil {
				response.Error(w, http.StatusServiceUnavailable, "temporarily_unavailable", "authentication is unavailable")
				return
			}

			identity, err := authenticate.Execute(r.Context(), identityapp.ResolveStaffIdentityInput{AccessToken: token})
			if err != nil {
				identityhttp.WriteAccessError(w, r, logger, "authenticate", err)
				return
			}

			ctx := requestctx.WithStaffIdentity(r.Context(), identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > maxAuthorizationBytes {
		return "", false
	}
	scheme, token, found := strings.Cut(values[0], " ")
	token = strings.TrimLeft(token, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n,") {
		return "", false
	}
	return token, true
}
