package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
)

func RegistrationAuthentication(verify *identity.VerifyRegistrationAccount, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
				response.Error(w, 401, "authentication_required", "a Bearer access token is required")
				return
			}
			if verify == nil {
				response.Error(w, 503, "registration_not_configured", "registration is unavailable")
				return
			}
			account, err := verify.Execute(r.Context(), token)
			if err != nil {
				switch {
				case errors.Is(err, identity.ErrEmailNotConfirmed):
					response.Error(w, 403, "email_confirmation_required", "confirm your email before registering a shop")
				case errors.Is(err, identity.ErrRegistrationNotConfigured):
					response.Error(w, 503, "registration_not_configured", "registration is unavailable")
				case errors.Is(err, identity.ErrRegistrationUnavailable):
					response.Error(w, 503, "temporarily_unavailable", "registration could not be verified")
				default:
					identityhttp.WriteAccessError(w, r, logger, "verify registration", err)
				}
				return
			}
			next.ServeHTTP(w, r.WithContext(requestctx.WithRegistrationIdentity(r.Context(), account)))
		})
	}
}
