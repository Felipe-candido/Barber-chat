package application

import (
	"context"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

// PayloadSealer is consumed by PrepareBookingNotifications. Its adapter must
// authenticate the scope and return sanitized errors without plaintext or keys.
// RabbitMQ and provider ports will be added with their consumers.
type PayloadSealer interface {
	Seal(context.Context, PayloadScope, BookingSnapshot) (ProtectedPayload, error)
}

type PlanningStore interface {
	Policy(context.Context, uuid.UUID) (domain.Policy, error)
	Save(context.Context, PrepareBookingNotificationsInput, PreparedBookingNotifications) error
}

type BookingPermissionRecorder interface {
	RecordBookingPermission(context.Context, BookingPermission) error
}

type MessageShopReader interface {
	FindMessageShop(context.Context, uuid.UUID) (MessageShop, bool, error)
}
