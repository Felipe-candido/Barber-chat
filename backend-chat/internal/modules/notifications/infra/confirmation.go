package infra

import (
	"context"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/google/uuid"
)

type BookingConfirmation struct {
	messages *notifications.ManageBookingMessages
}

func NewBookingConfirmation(messages *notifications.ManageBookingMessages) *BookingConfirmation {
	return &BookingConfirmation{messages}
}
func (b *BookingConfirmation) DispatchConfirmation(ctx context.Context, shop, appointment uuid.UUID, fresh bool) (booking.BookingConfirmationStatus, error) {
	r, err := b.messages.DispatchConfirmation(ctx, shop, appointment, fresh)
	return booking.BookingConfirmationStatus{State: r.State}, err
}
