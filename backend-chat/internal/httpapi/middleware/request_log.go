package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// RequestLog records only routing metadata, never raw URLs, bodies or headers.
// Generate our own correlation ID: an arbitrary caller header is not safe to log.
// Install before recovery/CORS so rejected and recovered requests are recorded too.
func RequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			id := uuid.NewString()
			r = r.WithContext(context.WithValue(r.Context(), chimiddleware.RequestIDKey, id))
			w.Header().Set("X-Request-ID", id)
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
				}
				route := "unmatched"
				if ctx := chi.RouteContext(r.Context()); ctx != nil && ctx.RoutePattern() != "" {
					route = ctx.RoutePattern()
				}
				// Custom HTTP methods can contain arbitrary caller-supplied text.
				method := r.Method
				switch method {
				case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
				default:
					method = "OTHER"
				}
				level := slog.LevelInfo
				if status >= 500 {
					level = slog.LevelError
				} else if status >= 400 {
					level = slog.LevelWarn
				}
				logger.Log(r.Context(), level, "HTTP request completed",
					"request_id", id, "method", method, "route", route,
					"status", status, "duration_ms", time.Since(started).Milliseconds())
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}
