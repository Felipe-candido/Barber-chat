package application

import (
	"context"
	"errors"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

type DeliverNotification struct {
	tx     DeliveryTransactor
	opener PayloadOpener
	sender NotificationSender
}

func NewDeliverNotification(tx DeliveryTransactor, opener PayloadOpener, sender NotificationSender) *DeliverNotification {
	return &DeliverNotification{tx, opener, sender}
}

// Recover inspects expired work without ever initiating a new external call.
func (uc *DeliverNotification) Recover(ctx context.Context, key DeliveryKey) (DeliveryResult, error) {
	key.recoveryOnly = true
	return uc.Execute(ctx, key)
}

// Execute performs at most one external attempt. Both HTTP and a future AMQP
// consumer can invoke it. No broker ACK, goroutine or business transaction here.
func (uc *DeliverNotification) Execute(ctx context.Context, key DeliveryKey) (DeliveryResult, error) {
	if key.ShopID == uuid.Nil || key.JobID == uuid.Nil {
		return DeliveryResult{}, ErrInvalidDelivery
	}
	if uc == nil || uc.tx == nil || uc.opener == nil || uc.sender == nil {
		return DeliveryResult{}, ErrDeliveryNotConfigured
	}
	token, err := uuid.NewRandom()
	if err != nil {
		return DeliveryResult{}, deliveryError(err)
	}
	claim := DeliveryClaim{Key: key, Token: token}
	result := DeliveryResult{State: "not_found"}
	var record DeliveryRecord
	var observed time.Time
	claimed := false
	err = uc.tx.WithinDeliveryTx(ctx, func(ctx context.Context, tx DeliveryTx) error {
		locked, err := lockDelivery(ctx, tx, key)
		if err != nil {
			return err
		}
		if !locked.found {
			return nil
		}
		if key.DispatchSequence > 0 && key.DispatchSequence != locked.record.DispatchSequence {
			result = DeliveryResult{State: "superseded"}
			return nil
		}
		result, err = inspectDelivery(ctx, tx, locked)
		if err != nil || result.State != "ready" || key.recoveryOnly {
			return err
		}
		if err := tx.Store.Claim(ctx, claim); err != nil {
			return err
		}
		record, observed, claimed = locked.record, locked.now, true
		return nil
	})
	if err != nil {
		return DeliveryResult{}, deliveryError(err)
	}
	if !claimed {
		return result, nil
	}

	snapshot, err := uc.opener.Open(ctx, observed, record.Scope, record.Payload)
	if err != nil {
		if ctx.Err() != nil {
			return uc.stopUnstarted(ctx, claim, "")
		}
		return uc.stopUnstarted(ctx, claim, domain.ReasonPayloadUnavailable)
	}
	var instance string
	started := false
	err = uc.tx.WithinDeliveryTx(ctx, func(ctx context.Context, tx DeliveryTx) error {
		locked, err := lockDelivery(ctx, tx, key)
		if err != nil {
			return err
		}
		if !locked.found {
			return ErrDeliveryLeaseLost
		}
		r := locked.record
		if r.State != domain.DeliveryLeased || r.Token != token || !locked.now.Before(r.LeaseUntil) {
			result = DeliveryResult{State: "busy"}
			if r.State != domain.DeliveryLeased && r.State != domain.DeliverySending {
				result = DeliveryResult{State: string(r.State), Reason: r.Reason}
			}
			return nil
		}
		// Inspect eligibility again under the common locks, immediately before the
		// attempt. Our own unexpired lease is not contention in this second phase.
		locked.record.State = domain.DeliveryReady
		result, err = inspectDelivery(ctx, tx, locked)
		if err != nil || result.State != "ready" {
			if err == nil && (result.State == "connection_unavailable" || result.State == "sender_busy") {
				return tx.Store.Stop(ctx, key, domain.DeliveryReady, "")
			}
			return err
		}
		if !snapshotMatches(snapshot, locked.booking, locked.permission) ||
			r.Scope.ScheduleGeneration != record.Scope.ScheduleGeneration || !r.Scope.ExpiresAt.Equal(record.Scope.ExpiresAt) {
			result = DeliveryResult{State: string(domain.DeliveryInvalidated), Reason: domain.ReasonIneligible}
			return tx.Store.Stop(ctx, key, domain.DeliveryInvalidated, domain.ReasonIneligible)
		}
		if err := tx.Store.Start(ctx, claim, locked.connection.InstanceName, BookingMessageTemplateVersion); err != nil {
			return err
		}
		instance, started = locked.connection.InstanceName, true
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotificationSenderBusy) {
			_, _ = uc.stopUnstarted(ctx, claim, "")
			return DeliveryResult{State: "sender_busy"}, nil
		}
		return DeliveryResult{}, deliveryError(err)
	}
	if !started {
		return result, nil
	}

	outcome, sendErr := uc.sender.Send(ctx, OutboundNotification{AttemptID: token, TemplateVersion: BookingMessageTemplateVersion, InstanceName: instance, Kind: record.Kind, Snapshot: snapshot})
	if sendErr != nil {
		outcome = domain.SendOutcome{Disposition: domain.SendUnknown, Reason: domain.ReasonTransportUnknown}
	}
	outcome = outcome.Normalize()
	// The browser may have gone away after the provider accepted the message.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	final := DeliveryResult{State: string(outcome.State()), Reason: outcome.Reason}
	err = uc.tx.WithinDeliveryTx(cleanup, func(ctx context.Context, tx DeliveryTx) error {
		if err := tx.Store.Complete(ctx, claim, outcome); err != nil {
			return err
		}
		current, found, err := tx.Store.Lock(ctx, key)
		if err != nil {
			return err
		}
		if found {
			final = DeliveryResult{State: string(current.State), Reason: current.Reason}
		}
		return nil
	})
	if err != nil {
		return DeliveryResult{State: string(domain.DeliveryUnknown), Reason: domain.ReasonTransportUnknown}, ErrDeliveryUnavailable
	}
	return final, nil
}

type lockedDelivery struct {
	record         DeliveryRecord
	permission     DeliveryPermission
	booking        DeliveryBooking
	connection     domain.Connection
	now            time.Time
	enabled, found bool
}

func lockDelivery(ctx context.Context, tx DeliveryTx, key DeliveryKey) (lockedDelivery, error) {
	var l lockedDelivery
	appointment, found, err := tx.Store.Reference(ctx, key)
	if err != nil || !found {
		return l, err
	}
	// Booking also locks shop/professional before contact and notification policy.
	// Revocation/disable triggers lock jobs last, so we must not hold one yet.
	active, err := tx.Shops.LockDeliveryShop(ctx, key.ShopID)
	if err != nil {
		return l, err
	}
	l.booking, err = tx.Bookings.LockDeliveryBooking(ctx, key.ShopID, appointment)
	if err != nil {
		return l, err
	}
	if key.ActorID != uuid.Nil {
		if tx.Access == nil {
			return l, ErrDeliveryAccessDenied
		}
		if err := tx.Access.RequireDeliveryStaff(ctx, ConnectionScope{ActorID: key.ActorID, ShopID: key.ShopID}, l.booking.ProfessionalID); err != nil {
			return l, err
		}
	}
	l.permission, err = tx.Permissions.LockDeliveryPermission(ctx, key.ShopID, appointment)
	if err != nil {
		return l, err
	}
	enabled, err := tx.Store.PolicyEnabled(ctx, key.ShopID)
	if err != nil {
		return l, err
	}
	l.enabled = active && enabled
	l.connection, err = tx.Store.Connection(ctx, key.ShopID)
	if err != nil {
		return l, err
	}
	l.record, l.found, err = tx.Store.Lock(ctx, key)
	if err != nil {
		return l, err
	}
	if l.found && l.record.AppointmentID != appointment {
		return l, ErrInvalidDelivery
	}
	l.now, err = tx.Store.Now(ctx)
	return l, err
}

func deliveryError(err error) error {
	if errors.Is(err, ErrDeliveryAccessDenied) {
		return ErrDeliveryAccessDenied
	}
	return ErrDeliveryUnavailable
}

func inspectDelivery(ctx context.Context, tx DeliveryTx, l lockedDelivery) (DeliveryResult, error) {
	r := l.record
	if r.State.StartedLeaseExpired(l.now, r.LeaseUntil) {
		o := domain.SendOutcome{Disposition: domain.SendUnknown, Reason: domain.ReasonTransportUnknown}
		err := tx.Store.Complete(ctx, DeliveryClaim{Key: r.Key, Token: r.Token}, o)
		return DeliveryResult{State: string(domain.DeliveryUnknown), Reason: o.Reason}, err
	}
	if !r.State.ReclaimableAt(l.now, r.LeaseUntil) {
		if r.State == domain.DeliveryLeased || r.State == domain.DeliverySending {
			return DeliveryResult{State: "busy"}, nil
		}
		return DeliveryResult{State: string(r.State), Reason: r.Reason}, nil
	}
	state, reason := domain.DeliveryInvalidated, domain.ReasonIneligible
	switch {
	case r.IntentInvalidated:
		reason = domain.ReasonIntentInvalidated
	case !l.enabled || !l.permission.Allowed || !l.booking.Active || l.booking.CustomerID != l.permission.CustomerID || !l.now.Before(l.booking.StartsAt):
	case !r.Window.Valid() || !l.now.Before(r.Window.ExpiresAt) || !l.now.Before(r.Scope.ExpiresAt):
		state, reason = domain.DeliveryExpired, domain.ReasonExpired
	default:
		if l.now.Before(r.Window.DueAt) {
			return DeliveryResult{State: "not_due"}, nil
		}
		c := l.connection
		if l.now.Before(c.SendLeaseUntil) {
			return DeliveryResult{State: "sender_busy"}, nil
		}
		if c.State != domain.ConnectionConnected || !c.DesiredConnected || c.ErrorCode != "" || !c.FreshAt(l.now) {
			return DeliveryResult{State: "connection_unavailable"}, nil
		}
		return DeliveryResult{State: "ready"}, nil
	}
	return DeliveryResult{State: string(state), Reason: reason}, tx.Store.Stop(ctx, r.Key, state, reason)
}

func (uc *DeliverNotification) stopUnstarted(ctx context.Context, claim DeliveryClaim, reason domain.DeliveryReason) (DeliveryResult, error) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	state := domain.DeliveryInvalidated
	if reason == "" {
		state = domain.DeliveryReady
	}
	err := uc.tx.WithinDeliveryTx(cleanup, func(ctx context.Context, tx DeliveryTx) error {
		r, found, err := tx.Store.Lock(ctx, claim.Key)
		if err != nil {
			return err
		}
		if found && r.Token == claim.Token && r.State == domain.DeliveryLeased {
			return tx.Store.Stop(ctx, claim.Key, state, reason)
		}
		return nil
	})
	if err != nil {
		return DeliveryResult{}, ErrDeliveryUnavailable
	}
	return DeliveryResult{State: string(state), Reason: reason}, nil
}

func snapshotMatches(s BookingSnapshot, b DeliveryBooking, p DeliveryPermission) bool {
	return s.Valid() && s.RecipientPhone == p.Phone && s.StartsAt.Equal(b.StartsAt) && s.EndsAt.Equal(b.EndsAt) &&
		s.ServiceName == b.ServiceName && s.ProfessionalName == b.ProfessionalName && s.PriceCents == b.PriceCents && s.Currency == b.Currency && s.ShopTimezone == b.Timezone
}
