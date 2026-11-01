package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type NotificationDeliveryBookings struct{ q *db.Queries }

func NewNotificationDeliveryBookings(q *db.Queries) *NotificationDeliveryBookings {
	return &NotificationDeliveryBookings{q}
}
func (b *NotificationDeliveryBookings) LockDeliveryBooking(ctx context.Context, shop, appointment uuid.UUID) (notifications.DeliveryBooking, error) {
	professional, err := b.q.FindNotificationDeliveryProfessional(ctx, db.FindNotificationDeliveryProfessionalParams{ShopID: shop, ID: appointment})
	if errors.Is(err, pgx.ErrNoRows) {
		return notifications.DeliveryBooking{}, nil
	}
	if err != nil {
		return notifications.DeliveryBooking{}, err
	}
	// Same lock protocol as booking creation, cancellation and schedule changes.
	if _, err = b.q.LockProfessionalCalendar(ctx, db.LockProfessionalCalendarParams{ShopID: shop, ID: professional}); err != nil {
		return notifications.DeliveryBooking{}, err
	}
	r, err := b.q.LockNotificationDeliveryBooking(ctx, db.LockNotificationDeliveryBookingParams{ShopID: shop, ID: appointment})
	if errors.Is(err, pgx.ErrNoRows) {
		return notifications.DeliveryBooking{}, nil
	}
	if err != nil {
		return notifications.DeliveryBooking{}, err
	}
	// A concurrent unsupported reassignment must not bypass the calendar lock.
	if r.ProfessionalID != professional {
		return notifications.DeliveryBooking{}, nil
	}
	return notifications.DeliveryBooking{CustomerID: r.CustomerID, Active: r.Status == "scheduled" || r.Status == "confirmed", StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), ServiceName: r.ServiceName, ProfessionalName: r.ProfessionalName, PriceCents: r.PriceCents, Currency: r.Currency, Timezone: r.ShopTimezone}, nil
}
