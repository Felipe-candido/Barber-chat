// Package http serves administrative identity over HTTP after authentication.
package http

import (
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/google/uuid"
)

// Me returns the active local identity supplied by the authentication middleware.
func Me(w http.ResponseWriter, r *http.Request) {
	identity, ok := requestctx.StaffIdentityFromContext(r.Context())
	if !ok || identity.UserID == uuid.Nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
		response.Error(w, http.StatusUnauthorized, "authentication_required", "authentication is required")
		return
	}
	response.JSON(w, http.StatusOK, struct {
		UserID      uuid.UUID `json:"user_id"`
		DisplayName string    `json:"display_name"`
	}{UserID: identity.UserID, DisplayName: identity.DisplayName})
}
