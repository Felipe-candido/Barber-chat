package application

import (
	"context"
	"errors"
	"math"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
)

var ErrInvalidNotificationSettings = errors.New("invalid notification settings")
var ErrNotificationSettingsConflict = errors.New("notification settings changed")

type PublicNotificationOptions interface {
	WhatsAppEnabled(context.Context, string) (bool, error)
}

type NotificationSettings struct {
	Enabled                               bool
	DeliveryMode                          string
	ReminderLeadMinutes                   int
	Version                               int64
	DeliveryAvailable, AutomaticAvailable bool
}
type NotificationSettingsStore interface {
	Read(context.Context, ConnectionScope) (NotificationSettings, error)
	Save(context.Context, ConnectionScope, NotificationSettings) (NotificationSettings, error)
	WorkerAvailable(context.Context) (bool, error)
}
type ManageNotificationSettings struct {
	store                NotificationSettingsStore
	available, automatic bool
}

func NewManageNotificationSettings(store NotificationSettingsStore, available, automatic bool) *ManageNotificationSettings {
	return &ManageNotificationSettings{store, available, automatic}
}
func (u *ManageNotificationSettings) Read(ctx context.Context, s ConnectionScope) (NotificationSettings, error) {
	r, err := u.store.Read(ctx, s)
	r.DeliveryAvailable = u.available
	r.AutomaticAvailable = u.automatic
	if err == nil && u.automatic {
		r.AutomaticAvailable, err = u.store.WorkerAvailable(ctx)
	}
	return r, err
}
func (u *ManageNotificationSettings) Save(ctx context.Context, s ConnectionScope, input NotificationSettings) (NotificationSettings, error) {
	// Revalidate owner before revealing runtime capabilities, including writes.
	current, err := u.Read(ctx, s)
	if err != nil {
		return NotificationSettings{}, err
	}
	if input.Version < 0 || input.Version == math.MaxInt64 || (input.DeliveryMode != "request" && input.DeliveryMode != "automatic") || !(domain.Policy{Version: 1, ReminderLeadMinutes: input.ReminderLeadMinutes, DeliveryMode: input.DeliveryMode}).Valid() {
		return NotificationSettings{}, ErrInvalidNotificationSettings
	}
	if input.Enabled && !u.available {
		return NotificationSettings{}, ErrDeliveryNotConfigured
	}
	if input.Enabled && input.DeliveryMode == "automatic" && !current.AutomaticAvailable {
		return NotificationSettings{}, ErrInvalidNotificationSettings
	}
	r, err := u.store.Save(ctx, s, input)
	r.DeliveryAvailable = u.available
	r.AutomaticAvailable = current.AutomaticAvailable
	return r, err
}
