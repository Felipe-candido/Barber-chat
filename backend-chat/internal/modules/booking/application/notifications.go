package application

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

var ErrNotificationPlanningUnavailable = errors.New("booking notification planner unavailable")

// Called after a new insert, never on replay. Implementations must share the
// booking transaction and must not publish or send externally.
type BookingNotificationPlanner interface {
	Plan(context.Context, BookingNotificationInput) error
}

type BookingNotificationInput struct {
	Appointment           domain.Appointment
	ShopName, ShopSlug    string
	Customer              CustomerInput
	Code                  string
	WhatsAppNotifications bool
	ActorID               uuid.UUID
}

func (BookingNotificationInput) String() string         { return "[redacted booking notification input]" }
func (i BookingNotificationInput) GoString() string     { return i.String() }
func (i BookingNotificationInput) LogValue() slog.Value { return slog.StringValue(i.String()) }
