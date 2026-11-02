package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidNotificationEnvelope = errors.New("invalid notification dispatch envelope")

// Broker payloads contain routing references only, never contact or content.
type DispatchEnvelope struct {
	Version  int       `json:"version"`
	OutboxID uuid.UUID `json:"outbox_id"`
	ShopID   uuid.UUID `json:"shop_id"`
	JobID    uuid.UUID `json:"job_id"`
	Sequence int64     `json:"sequence"`
}

func (e DispatchEnvelope) Valid() bool {
	return e.Version == 1 && e.OutboxID != uuid.Nil && e.ShopID != uuid.Nil && e.JobID != uuid.Nil && e.Sequence > 0
}

type DispatchLease struct {
	Envelope DispatchEnvelope
	Token    uuid.UUID
}
type DispatchStore interface {
	Metrics(context.Context) (DispatchMetrics, error)
	Maintenance(context.Context) error
	Materialize(context.Context) (int, error)
	Claim(context.Context) (DispatchLease, bool, error)
	Published(context.Context, DispatchLease) error
	PublishDeferred(context.Context, DispatchLease) error
	Validate(context.Context, DispatchEnvelope) (bool, error)
	Finish(context.Context, DispatchEnvelope) error
	RecoveryKeys(context.Context) ([]DeliveryKey, error)
	Heartbeat(context.Context, uuid.UUID, bool) error
}

type DispatchMetrics struct{ OutboxPending, Uncertain, Sending, OldestDueSeconds int64 }
type NotificationPublisher interface {
	Publish(context.Context, DispatchEnvelope) error
}
type DispatchNotifications struct {
	store   DispatchStore
	deliver *DeliverNotification
}

func NewDispatchNotifications(store DispatchStore, deliver *DeliverNotification) *DispatchNotifications {
	return &DispatchNotifications{store, deliver}
}

func (u *DispatchNotifications) Maintain(ctx context.Context) error { return u.store.Maintenance(ctx) }

func (u *DispatchNotifications) Schedule(ctx context.Context) (int, error) {
	keys, err := u.store.RecoveryKeys(ctx)
	if err != nil {
		return 0, err
	}
	for _, key := range keys {
		if _, err := u.deliver.Recover(ctx, key); err != nil {
			return 0, err
		}
	}
	return u.store.Materialize(ctx)
}
func (u *DispatchNotifications) RelayOne(ctx context.Context, publisher NotificationPublisher) (bool, error) {
	lease, found, err := u.store.Claim(ctx)
	if err != nil || !found {
		return false, err
	}
	if err = publisher.Publish(ctx, lease.Envelope); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if saveErr := u.store.PublishDeferred(cleanup, lease); saveErr != nil {
			return true, saveErr
		}
		return true, err
	}
	return true, u.store.Published(ctx, lease)
}
func (u *DispatchNotifications) Consume(ctx context.Context, envelope DispatchEnvelope) error {
	if !envelope.Valid() {
		return ErrInvalidNotificationEnvelope
	}
	valid, err := u.store.Validate(ctx, envelope)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidNotificationEnvelope
	}
	_, err = u.deliver.Execute(ctx, DeliveryKey{ShopID: envelope.ShopID, JobID: envelope.JobID, DispatchSequence: envelope.Sequence})
	if err != nil {
		return err
	}
	// Terminal results and durable deferral both precede broker ACK. A new
	// dispatch after a deferral gets a new sequence; unknown never becomes ready.
	return u.store.Finish(ctx, envelope)
}
