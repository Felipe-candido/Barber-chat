package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// WriteAccessError maps identity failures consistently for middleware and handlers.
// Raw errors and credentials must not be exposed in responses or logs.
func WriteAccessError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	switch {
	case errors.Is(err, identityapp.ErrInvalidAccessToken), errors.Is(err, identityapp.ErrInvalidStaffIdentity):
		w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat", error="invalid_token"`)
		response.Error(w, http.StatusUnauthorized, "invalid_access_token", "the access token is invalid or expired")
	case errors.Is(err, identityapp.ErrUserNotProvisioned), errors.Is(err, identityapp.ErrUserInactive), errors.Is(err, identityapp.ErrShopAccessDenied):
		response.Error(w, http.StatusForbidden, "access_denied", "access is not allowed")
	case errors.Is(err, supabase.ErrJWKSUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		logger.WarnContext(r.Context(), "access check unavailable", "operation", operation, "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, http.StatusServiceUnavailable, "temporarily_unavailable", "access could not be checked right now")
	default:
		// Repository and provider errors may contain credentials or request data.
		logger.ErrorContext(r.Context(), "access check failed", "operation", operation, "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, http.StatusInternalServerError, "internal_error", "access could not be checked")
	}
}
