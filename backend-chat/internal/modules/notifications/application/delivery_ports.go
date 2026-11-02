package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

var (
	ErrDeliveryUnavailable    = errors.New("notification delivery unavailable")
	ErrDeliveryNotConfigured  = errors.New("notification sender not configured")
	ErrInvalidDelivery        = errors.New("invalid notification delivery reference")
	ErrDeliveryLeaseLost      = errors.New("notification delivery lease lost")
	ErrDeliveryAccessDenied   = errors.New("notification delivery access denied")
	ErrNotificationSenderBusy = errors.New("notification sender busy")
)

// Internal references come from a committed booking or a trusted dispatcher.
// A future administrative handler must authorize the actor before invoking this.
type DeliveryKey struct {
	ShopID, JobID, ActorID uuid.UUID
	DispatchSequence       int64
	recoveryOnly           bool
}
type DeliveryResult struct {
	State  string
	Reason domain.DeliveryReason
}
type DeliveryClaim struct {
	Key   DeliveryKey
	Token uuid.UUID
}

type DeliveryRecord struct {
	DispatchSequence  int64
	Key               DeliveryKey
	AppointmentID     uuid.UUID
	Kind              string
	Scope             PayloadScope
	Payload           ProtectedPayload
	Window            domain.Window
	IntentInvalidated bool
	State             domain.DeliveryState
	Token             uuid.UUID
	LeaseUntil        time.Time
	Reason            domain.DeliveryReason
}

type DeliveryPermission struct {
	CustomerID uuid.UUID
	Allowed    bool
	Phone      string
}

func (DeliveryPermission) String() string         { return "[redacted delivery permission]" }
func (p DeliveryPermission) GoString() string     { return p.String() }
func (p DeliveryPermission) LogValue() slog.Value { return slog.StringValue(p.String()) }

type DeliveryBooking struct {
	CustomerID                                        uuid.UUID
	ProfessionalID                                    uuid.UUID
	Active                                            bool
	StartsAt, EndsAt                                  time.Time
	ServiceName, ProfessionalName, Currency, Timezone string
	PriceCents                                        int64
}

type DeliveryStore interface {
	Reference(context.Context, DeliveryKey) (uuid.UUID, bool, error)
	PolicyEnabled(context.Context, uuid.UUID) (bool, error)
	Connection(context.Context, uuid.UUID) (domain.Connection, error)
	Lock(context.Context, DeliveryKey) (DeliveryRecord, bool, error)
	Now(context.Context) (time.Time, error)
	Claim(context.Context, DeliveryClaim) error
	Stop(context.Context, DeliveryKey, domain.DeliveryState, domain.DeliveryReason) error
	Start(context.Context, DeliveryClaim, string, int) error
	Complete(context.Context, DeliveryClaim, domain.SendOutcome) error
	ListBooking(context.Context, uuid.UUID, uuid.UUID) ([]BookingMessageStatus, error)
}

type DeliveryShops interface {
	LockDeliveryShop(context.Context, uuid.UUID) (bool, error)
}
type DeliveryPermissions interface {
	LockDeliveryPermission(context.Context, uuid.UUID, uuid.UUID) (DeliveryPermission, error)
}
type DeliveryBookings interface {
	LockDeliveryBooking(context.Context, uuid.UUID, uuid.UUID) (DeliveryBooking, error)
}

type DeliveryTx struct {
	Store       DeliveryStore
	Shops       DeliveryShops
	Permissions DeliveryPermissions
	Bookings    DeliveryBookings
	Access      DeliveryStaffAccess
}
type DeliveryStaffAccess interface {
	RequireDeliveryStaff(context.Context, ConnectionScope, uuid.UUID) error
}
type DeliveryTransactor interface {
	WithinDeliveryTx(context.Context, func(context.Context, DeliveryTx) error) error
}
type PayloadOpener interface {
	Open(context.Context, time.Time, PayloadScope, ProtectedPayload) (BookingSnapshot, error)
}
type NotificationSender interface {
	// An error means uncertain external effect unless the adapter returns a valid
	// explicit SendNotSent outcome with no error. Never return raw provider bodies.
	Send(context.Context, OutboundNotification) (domain.SendOutcome, error)
}
type OutboundNotification struct {
	AttemptID          uuid.UUID
	TemplateVersion    int
	InstanceName, Kind string
	Snapshot           BookingSnapshot
}

func (OutboundNotification) String() string         { return "[redacted outbound notification]" }
func (m OutboundNotification) GoString() string     { return m.String() }
func (m OutboundNotification) LogValue() slog.Value { return slog.StringValue(m.String()) }
