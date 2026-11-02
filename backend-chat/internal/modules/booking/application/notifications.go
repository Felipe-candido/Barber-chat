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

// Transport invokes this only after CreateAppointment has committed. Failure
// must never roll back/reclassify an already accepted booking.
type BookingConfirmationDispatcher interface {
	DispatchConfirmation(context.Context, uuid.UUID, uuid.UUID, bool) (BookingConfirmationStatus, error)
}
type BookingConfirmationStatus struct{ State string }

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
