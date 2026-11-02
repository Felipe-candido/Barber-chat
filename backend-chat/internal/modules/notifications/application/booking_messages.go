package application

import (
	"context"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

type BookingMessageScope struct{ ActorID, ShopID, AppointmentID uuid.UUID }
type BookingMessageStatus struct {
	Receipt                      string
	JobID                        uuid.UUID
	Kind, State, Mode            string
	Reason                       domain.DeliveryReason
	DueAt, ExpiresAt, LeaseUntil time.Time
	CanSend, IntentInvalidated   bool
}
type BookingMessages struct {
	Messages          []BookingMessageStatus
	DeliveryAvailable bool
}
type ManageBookingMessages struct {
	tx      DeliveryTransactor
	deliver *DeliverNotification
	enabled bool
}

func NewManageBookingMessages(tx DeliveryTransactor, deliver *DeliverNotification, enabled bool) *ManageBookingMessages {
	return &ManageBookingMessages{tx, deliver, enabled}
}
func (u *ManageBookingMessages) Read(ctx context.Context, s BookingMessageScope) (BookingMessages, error) {
	if s.ActorID == uuid.Nil {
		return BookingMessages{}, ErrDeliveryAccessDenied
	}
	return u.read(ctx, s, false)
}
func (u *ManageBookingMessages) read(ctx context.Context, s BookingMessageScope, internal bool) (BookingMessages, error) {
	result := BookingMessages{Messages: []BookingMessageStatus{}, DeliveryAvailable: u.enabled}
	err := u.tx.WithinDeliveryTx(ctx, func(ctx context.Context, tx DeliveryTx) error {
		active, err := tx.Shops.LockDeliveryShop(ctx, s.ShopID)
		if err != nil {
			return err
		}
		b, err := tx.Bookings.LockDeliveryBooking(ctx, s.ShopID, s.AppointmentID)
		if err != nil {
			return err
		}
		if !active || b.CustomerID == uuid.Nil {
			return ErrDeliveryAccessDenied
		}
		if !internal {
			if tx.Access == nil {
				return ErrDeliveryAccessDenied
			}
			if err := tx.Access.RequireDeliveryStaff(ctx, ConnectionScope{ActorID: s.ActorID, ShopID: s.ShopID}, b.ProfessionalID); err != nil {
				return err
			}
		}
		permission, err := tx.Permissions.LockDeliveryPermission(ctx, s.ShopID, s.AppointmentID)
		if err != nil {
			return err
		}
		policy, err := tx.Store.PolicyEnabled(ctx, s.ShopID)
		if err != nil {
			return err
		}
		connection, err := tx.Store.Connection(ctx, s.ShopID)
		if err != nil {
			return err
		}
		now, err := tx.Store.Now(ctx)
		if err != nil {
			return err
		}
		messages, err := tx.Store.ListBooking(ctx, s.ShopID, s.AppointmentID)
		if err != nil {
			return err
		}
		for i := range messages {
			m := &messages[i]
			if m.State == "leased" && !now.Before(m.LeaseUntil) {
				m.State = "ready"
			}
			if m.State == "sending" && !now.Before(m.LeaseUntil) {
				m.State = "unknown"
			}
			if (m.State == "ready" || m.State == "leased") && (m.IntentInvalidated || !permission.Allowed || !policy || !b.Active) {
				m.State = "invalidated"
			}
			if m.State == "ready" && !now.Before(m.ExpiresAt) {
				m.State = "expired"
			}
			m.CanSend = u.enabled && m.State == "ready" && !now.Before(m.DueAt) && now.Before(m.ExpiresAt) && b.Active && permission.Allowed && policy && connection.State == domain.ConnectionConnected && connection.DesiredConnected && connection.ErrorCode == "" && connection.FreshAt(now) && !now.Before(connection.SendLeaseUntil)
		}
		result.Messages = messages
		return nil
	})
	if err != nil {
		return BookingMessages{}, deliveryError(err)
	}
	return result, nil
}
func (u *ManageBookingMessages) Send(ctx context.Context, s BookingMessageScope, kind string) (DeliveryResult, error) {
	if s.ActorID == uuid.Nil {
		return DeliveryResult{}, ErrDeliveryAccessDenied
	}
	messages, err := u.read(ctx, s, false)
	if err != nil {
		return DeliveryResult{}, err
	}
	if !u.enabled {
		return DeliveryResult{}, ErrDeliveryNotConfigured
	}
	if kind != "booking_confirmation" && kind != "booking_reminder" {
		return DeliveryResult{}, ErrInvalidDelivery
	}
	for _, m := range messages.Messages {
		if m.Kind == kind {
			return u.deliver.Execute(ctx, DeliveryKey{ShopID: s.ShopID, JobID: m.JobID, ActorID: s.ActorID})
		}
	}
	return DeliveryResult{State: "not_requested"}, nil
}

// DispatchConfirmation is used only with IDs returned by a committed booking.
// Replay reads the existing result without initiating another attempt.
func (u *ManageBookingMessages) DispatchConfirmation(ctx context.Context, shop, appointment uuid.UUID, fresh bool) (DeliveryResult, error) {
	if !u.enabled {
		return DeliveryResult{State: "unavailable"}, nil
	}
	messages, err := u.read(ctx, BookingMessageScope{ShopID: shop, AppointmentID: appointment}, true)
	if err != nil {
		return DeliveryResult{}, err
	}
	for _, m := range messages.Messages {
		if m.Kind == "booking_confirmation" {
			if !fresh {
				return DeliveryResult{State: m.State, Reason: m.Reason}, nil
			}
			return u.deliver.Execute(ctx, DeliveryKey{ShopID: shop, JobID: m.JobID})
		}
	}
	return DeliveryResult{State: "not_requested"}, nil
}
