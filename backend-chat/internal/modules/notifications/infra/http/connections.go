package http

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
	"github.com/google/uuid"
)

type ConnectionHandler struct {
	usecase *app.ManageConnections
	secret  string
	timeout time.Duration
}

func NewConnectionHandler(uc *app.ManageConnections, secret string, timeout time.Duration) *ConnectionHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &ConnectionHandler{uc, secret, timeout}
}
func connectionScope(r *http.Request) (app.ConnectionScope, bool) {
	i, ok := requestctx.StaffIdentityFromContext(r.Context())
	s, found := requestctx.ShopScopeFromContext(r.Context())
	return app.ConnectionScope{ActorID: i.UserID, ShopID: s.ShopID}, ok && found && i.UserID != uuid.Nil && s.ShopID != uuid.Nil && i.UserID == s.UserID && s.Role == "owner"
}

type connectionJSON struct {
	State              string     `json:"state"`
	DesiredConnected   bool       `json:"desired_connected"`
	Stale              bool       `json:"stale"`
	ProviderConfigured bool       `json:"provider_configured"`
	VerifiedAt         *time.Time `json:"verified_at"`
	LastConnectedAt    *time.Time `json:"last_connected_at"`
	ErrorCode          string     `json:"error_code"`
}

func connectionBody(v app.ConnectionView) connectionJSON {
	b := connectionJSON{State: string(v.State), DesiredConnected: v.DesiredConnected, Stale: v.Stale, ProviderConfigured: v.ProviderConfigured, ErrorCode: v.ErrorCode}
	if !v.VerifiedAt.IsZero() {
		at := v.VerifiedAt
		b.VerifiedAt = &at
	}
	if !v.LastConnectedAt.IsZero() {
		at := v.LastConnectedAt
		b.LastConnectedAt = &at
	}
	return b
}
func (h *ConnectionHandler) Status(w http.ResponseWriter, r *http.Request) { h.execute(w, r, "status") }
func (h *ConnectionHandler) Connect(w http.ResponseWriter, r *http.Request) {
	h.execute(w, r, "connect")
}
func (h *ConnectionHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	h.execute(w, r, "disconnect")
}
func (h *ConnectionHandler) execute(w http.ResponseWriter, r *http.Request, action string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	scope, ok := connectionScope(r)
	if !ok {
		response.Error(w, 403, "access_denied", "owner access is required")
		return
	}
	if action != "status" && r.ContentLength != 0 {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var body *struct{}
		if err := d.Decode(&body); err != nil || body == nil || d.Decode(new(any)) != io.EOF {
			response.Error(w, 400, "invalid_body", "send an empty JSON object or no body")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	var view app.ConnectionView
	var result app.ConnectResult
	var err error
	switch action {
	case "status":
		view, err = h.usecase.Status(ctx, scope)
	case "connect":
		result, err = h.usecase.Connect(ctx, scope)
		view = result.Connection
	case "disconnect":
		view, err = h.usecase.Disconnect(ctx, scope)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if action == "connect" {
		type challenge struct {
			PNG          string    `json:"png"`
			DiscardAfter time.Time `json:"discard_after"`
		}
		var qr *challenge
		if result.Challenge.QRBase64 != "" {
			qr = &challenge{result.Challenge.QRBase64, result.Challenge.DiscardAfter}
		}
		response.JSON(w, 200, struct {
			Connection connectionJSON `json:"connection"`
			QR         *challenge     `json:"qr"`
		}{connectionBody(view), qr})
		return
	}
	response.JSON(w, 200, connectionBody(view))
}
func (h *ConnectionHandler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrConnectionAccessDenied):
		response.Error(w, 403, "access_denied", "owner access is required")
	case errors.Is(err, app.ErrConnectionBusy):
		response.Error(w, 409, "connection_busy", "wait for the current operation and check connection status")
	case errors.Is(err, app.ErrProviderActivationRequired):
		response.Error(w, 503, "evolution_activation_required", "the operator must activate Evolution in its manager")
	case errors.Is(err, app.ErrProviderNotConfigured):
		response.Error(w, 503, "whatsapp_not_configured", "WhatsApp provider is not configured")
	default:
		response.Error(w, 503, "whatsapp_unavailable", "check connection status before repeating the action")
	}
}

func (h *ConnectionHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body struct {
		Event    string `json:"event"`
		Instance string `json:"instance"`
	}
	decoder := json.NewDecoder(r.Body)
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !evolution.ValidInstanceName(body.Instance) {
		response.Error(w, 400, "invalid_event", "invalid provider event")
		return
	}
	provided := r.Header.Values(evolution.WebhookHeader)
	if len(h.secret) < 32 || len(provided) != 1 || !hmac.Equal([]byte(provided[0]), []byte(evolution.WebhookToken(h.secret, body.Instance))) {
		response.Error(w, 401, "invalid_webhook", "invalid provider authentication")
		return
	}
	if body.Event != "connection.update" {
		w.WriteHeader(204)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.usecase.RequestRecheck(ctx, body.Instance); err != nil {
		response.Error(w, 503, "whatsapp_unavailable", "event could not be recorded")
		return
	}
	w.WriteHeader(204)
}
