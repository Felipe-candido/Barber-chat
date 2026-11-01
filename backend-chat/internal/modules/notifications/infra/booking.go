package infra

import (
	"context"
	"strings"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
)

// BookingPlanner adapts Booking's consumer-owned port without coupling domains.
type BookingPlanner struct{ planner *app.PlanBookingNotifications }

func NewBookingPlanner(planner *app.PlanBookingNotifications) *BookingPlanner {
	return &BookingPlanner{planner}
}

func (p *BookingPlanner) Plan(ctx context.Context, input booking.BookingNotificationInput) error {
	a := input.Appointment
	return p.planner.Execute(ctx, app.PlanBookingInput{
		CustomerID: a.CustomerID, ActorID: input.ActorID,
		Preparation: app.PrepareBookingNotificationsInput{
			ShopID: a.ShopID, AppointmentID: a.ID, ScheduleGeneration: 1, CreatedAt: a.CreatedAt,
			PermissionGranted: input.WhatsAppNotifications,
			Snapshot: app.BookingSnapshot{
				ShopName: input.ShopName, ShopSlug: input.ShopSlug,
				CustomerName:   strings.TrimSpace(input.Customer.FirstName + " " + input.Customer.LastName),
				RecipientPhone: input.Customer.Phone, AppointmentCode: input.Code,
				ProfessionalName: a.ProfessionalName, ServiceName: a.Service.Name,
				PriceCents: a.Service.PriceCents, Currency: a.Service.Currency,
				StartsAt: a.StartsAt, EndsAt: a.EndsAt, ShopTimezone: a.ShopTimezone,
			},
		},
	})
}
