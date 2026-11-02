package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type MessageHandler struct {
	settings *app.ManageNotificationSettings
	messages *app.ManageBookingMessages
	options  app.PublicNotificationOptions
	enabled  bool
	timeout  time.Duration
}

func NewMessageHandler(settings *app.ManageNotificationSettings, messages *app.ManageBookingMessages, options app.PublicNotificationOptions, enabled bool, timeout time.Duration) *MessageHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &MessageHandler{settings, messages, options, enabled, timeout}
}

type settingsJSON struct {
	Enabled             bool   `json:"enabled"`
	DeliveryMode        string `json:"delivery_mode"`
	ReminderLeadMinutes int    `json:"reminder_lead_minutes"`
	Version             int64  `json:"version"`
	DeliveryAvailable   bool   `json:"delivery_available"`
	AutomaticAvailable  bool   `json:"automatic_available"`
}

func settingsBody(s app.NotificationSettings) settingsJSON {
	return settingsJSON{s.Enabled, s.DeliveryMode, s.ReminderLeadMinutes, s.Version, s.DeliveryAvailable, s.AutomaticAvailable}
}
func (h *MessageHandler) Settings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := connectionScope(r)
	if !ok {
		messageFailure(w, app.ErrDeliveryAccessDenied)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	var result app.NotificationSettings
	var err error
	if r.Method == http.MethodGet {
		result, err = h.settings.Read(ctx, scope)
	} else {
		var body *struct {
			Enabled             *bool  `json:"enabled"`
			DeliveryMode        string `json:"delivery_mode"`
			ReminderLeadMinutes int    `json:"reminder_lead_minutes"`
			Version             *int64 `json:"version"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&body) != nil || body == nil || body.Enabled == nil || body.Version == nil || d.Decode(new(any)) != io.EOF {
			response.Error(w, 400, "invalid_body", "complete notification settings required")
			return
		}
		result, err = h.settings.Save(ctx, scope, app.NotificationSettings{Enabled: *body.Enabled, DeliveryMode: body.DeliveryMode, ReminderLeadMinutes: body.ReminderLeadMinutes, Version: *body.Version})
	}
	if err != nil {
		messageFailure(w, err)
		return
	}
	response.JSON(w, 200, settingsBody(result))
}
func (h *MessageHandler) PublicOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	enabled := false
	if h.enabled {
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		defer cancel()
		var err error
		enabled, err = h.options.WhatsAppEnabled(ctx, chi.URLParam(r, "slug"))
		if err != nil {
			messageFailure(w, err)
			return
		}
	}
	response.JSON(w, 200, struct {
		Enabled bool `json:"whatsapp_enabled"`
	}{enabled})
}
func messageScope(r *http.Request) (app.BookingMessageScope, bool) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	shop, authorized := requestctx.ShopScopeFromContext(r.Context())
	appointment, err := uuid.Parse(chi.URLParam(r, "appointmentID"))
	return app.BookingMessageScope{ActorID: actor.UserID, ShopID: shop.ShopID, AppointmentID: appointment}, authenticated && authorized && err == nil && actor.UserID != uuid.Nil && shop.UserID == actor.UserID && shop.ShopID != uuid.Nil && appointment != uuid.Nil
}
func (h *MessageHandler) BookingStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := messageScope(r)
	if !ok {
		messageFailure(w, app.ErrDeliveryAccessDenied)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.messages.Read(ctx, scope)
	if err != nil {
		messageFailure(w, err)
		return
	}
	type item struct {
		Receipt   string    `json:"receipt"`
		Kind      string    `json:"kind"`
		State     string    `json:"state"`
		Reason    string    `json:"reason"`
		Mode      string    `json:"mode"`
		DueAt     time.Time `json:"due_at"`
		ExpiresAt time.Time `json:"expires_at"`
		CanSend   bool      `json:"can_send"`
	}
	messages := make([]item, 0, len(result.Messages))
	for _, m := range result.Messages {
		messages = append(messages, item{m.Receipt, m.Kind, m.State, string(m.Reason), m.Mode, m.DueAt, m.ExpiresAt, m.CanSend})
	}
	response.JSON(w, 200, struct {
		Messages  []item `json:"messages"`
		Available bool   `json:"delivery_available"`
	}{messages, result.DeliveryAvailable})
}
func (h *MessageHandler) SendConfirmation(w http.ResponseWriter, r *http.Request) {
	h.send(w, r, "booking_confirmation")
}
func (h *MessageHandler) SendReminder(w http.ResponseWriter, r *http.Request) {
	h.send(w, r, "booking_reminder")
}
func (h *MessageHandler) send(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := messageScope(r)
	if !ok {
		messageFailure(w, app.ErrDeliveryAccessDenied)
		return
	}
	if r.ContentLength != 0 {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var body *struct{}
		if d.Decode(&body) != nil || body == nil || d.Decode(new(any)) != io.EOF {
			response.Error(w, 400, "invalid_body", "send an empty JSON object or no body")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.messages.Send(ctx, scope, kind)
	if err != nil {
		messageFailure(w, err)
		return
	}
	response.JSON(w, 200, struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}{result.State, string(result.Reason)})
}
func messageFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrDeliveryAccessDenied) || errors.Is(err, app.ErrConnectionAccessDenied):
		response.Error(w, 403, "access_denied", "notification access denied")
	case errors.Is(err, app.ErrNotificationSettingsConflict):
		response.Error(w, 409, "settings_conflict", "notification settings changed; reload before saving")
	case errors.Is(err, app.ErrInvalidNotificationSettings) || errors.Is(err, app.ErrInvalidDelivery):
		response.Error(w, 400, "invalid_settings", "invalid notification settings or action")
	default:
		response.Error(w, 503, "temporarily_unavailable", "notification request could not be completed; check current status before retrying")
	}
}
