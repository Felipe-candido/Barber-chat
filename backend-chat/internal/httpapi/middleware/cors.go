package middleware

import (
	"net/http"
	"strings"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
)

// CORS permits one explicitly configured browser origin. Bearer authentication
// remains required on administrative routes, including requests without Origin.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			origins := r.Header.Values("Origin")
			if len(origins) > 0 {
				if len(origins) != 1 || allowedOrigin == "" || origins[0] != allowedOrigin {
					response.Error(w, http.StatusForbidden, "origin_not_allowed", "browser origin is not allowed")
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Preflight is registered per route, outside the authentication chain.
func Preflight(methods ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")
		allowedMethod := false
		for _, method := range methods {
			if r.Header.Get("Access-Control-Request-Method") == method {
				allowedMethod = true
				break
			}
		}
		if r.Header.Get("Origin") == "" || !allowedMethod {
			response.Error(w, http.StatusForbidden, "origin_not_allowed", "preflight method is not allowed")
			return
		}
		for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
			header = strings.TrimSpace(header)
			if header != "" && !strings.EqualFold(header, "Authorization") && !strings.EqualFold(header, "Content-Type") {
				response.Error(w, http.StatusForbidden, "origin_not_allowed", "preflight header is not allowed")
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	}
}
