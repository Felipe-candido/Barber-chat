package middleware

import (
	"context"
	"net/http"
	"time"
)

// RequestTimeout applies one deadline across authentication, authorization and
// the handler. It leaves response selection to the operation that observes it.
func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
