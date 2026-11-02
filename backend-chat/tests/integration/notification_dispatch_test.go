//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/simulation"
	"github.com/google/uuid"
)

func dispatchFixture(t *testing.T, mode string) (*deliveryFixture, *notificationspg.DispatchStore) {
	t.Helper()
	f := newDeliveryFixtureMode(t, mode)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := f.pool.Exec(ctx, "UPDATE notification_jobs SET state='invalidated',invalidated_at=clock_timestamp() WHERE shop_id=$1 AND state='pending'", f.shop)
		if err != nil {
			t.Error("fixture invalidation failed", err)
		}
	})
	return f, notificationspg.NewDispatchStore(f.pool)
}
func envelopeFor(t *testing.T, f *deliveryFixture) app.DispatchEnvelope {
	t.Helper()
	e := app.DispatchEnvelope{Version: 1}
	if err := f.pool.QueryRow(f.ctx, "SELECT id,shop_id,job_id,dispatch_seq FROM notification_outbox WHERE shop_id=$1 ORDER BY dispatch_seq DESC LIMIT 1", f.shop).Scan(&e.OutboxID, &e.ShopID, &e.JobID, &e.Sequence); err != nil {
		t.Fatal(err)
	}
	return e
}
func dispatchPipeline(f *deliveryFixture, store app.DispatchStore, sender app.NotificationSender) *app.DispatchNotifications {
	return app.NewDispatchNotifications(store, app.NewDeliverNotification(f.tx, f.cipher, sender))
}

func TestNotificationDispatchStore(t *testing.T) {
	t.Run("concurrent schedulers and request mode exclusion", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		manual, _ := dispatchFixture(t, "request")
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			wg.Go(func() { _, err := store.Materialize(f.ctx); errs <- err })
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if f.count(t, "notification_outbox") != 1 || manual.count(t, "notification_outbox") != 0 {
			t.Fatal("duplicate dispatch or automatic send of manual intent")
		}
		if e := envelopeFor(t, f); e.Sequence != 1 {
			t.Fatal("dispatch generation not persisted")
		}
	})
	t.Run("outbox leases exclude stale publishers", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		if _, err := store.Materialize(f.ctx); err != nil {
			t.Fatal(err)
		}
		first, found, err := store.Claim(f.ctx)
		if err != nil || !found {
			t.Fatal("outbox not claimable", err)
		}
		if _, found, err = store.Claim(f.ctx); err != nil || found {
			t.Fatal("stole active outbox lease", err)
		}
		f.exec(t, "UPDATE notification_outbox SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", first.Envelope.OutboxID)
		second, found, err := store.Claim(f.ctx)
		if err != nil || !found || second.Token == first.Token {
			t.Fatal("publisher not recovered", err)
		}
		if err = store.Published(f.ctx, first); !errors.Is(err, app.ErrDeliveryLeaseLost) {
			t.Fatal("late publisher overwrote new lease", err)
		}
		if err = store.Published(f.ctx, second); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("old dispatch cannot start and unknown cannot retry", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		sender := simulation.NewSender(domain.SendUnknown)
		pipeline := dispatchPipeline(f, store, sender)
		if _, err := pipeline.Schedule(f.ctx); err != nil {
			t.Fatal(err)
		}
		old := envelopeFor(t, f)
		if err := store.Finish(f.ctx, old); err != nil {
			t.Fatal(err)
		}
		f.exec(t, "UPDATE notification_jobs SET dispatch_next_at=clock_timestamp()-interval '1 second' WHERE shop_id=$1", f.shop)
		if _, err := pipeline.Schedule(f.ctx); err != nil {
			t.Fatal(err)
		}
		next := envelopeFor(t, f)
		if next.Sequence != old.Sequence+1 {
			t.Fatal("deferral did not advance generation")
		}
		if err := pipeline.Consume(f.ctx, old); err != nil || sender.Calls() != 0 {
			t.Fatal("stale envelope caused send", err)
		}
		if err := pipeline.Consume(f.ctx, next); err != nil || sender.Calls() != 1 || f.state(t) != "unknown" {
			t.Fatal("current dispatch not recorded", err)
		}
		f.exec(t, "UPDATE notification_jobs SET dispatch_next_at=clock_timestamp()-interval '1 hour' WHERE shop_id=$1", f.shop)
		if _, err := pipeline.Schedule(f.ctx); err != nil || f.count(t, "notification_outbox") != 2 {
			t.Fatal("uncertain attempt was automatically retried", err)
		}
	})
	t.Run("only proven not sent can retry with backoff and limit", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		calls := 0
		sender := senderFunc(func(context.Context, app.OutboundNotification) (domain.SendOutcome, error) {
			calls++
			return domain.SendOutcome{Disposition: domain.SendNotSent, Reason: domain.ReasonProviderUnavailable}, nil
		})
		pipeline := dispatchPipeline(f, store, sender)
		for attempt := 1; attempt <= 5; attempt++ {
			if _, err := pipeline.Schedule(f.ctx); err != nil {
				t.Fatal(err)
			}
			e := envelopeFor(t, f)
			if e.Sequence != int64(attempt) {
				t.Fatal("missing retry dispatch", attempt, e.Sequence)
			}
			if err := pipeline.Consume(f.ctx, e); err != nil || f.state(t) != "failed" || calls != attempt {
				t.Fatal("not-sent attempt failed", err)
			}
			before := f.count(t, "notification_outbox")
			if _, err := pipeline.Schedule(f.ctx); err != nil || f.count(t, "notification_outbox") != before {
				t.Fatal("backoff ignored", err)
			}
			f.exec(t, "UPDATE notification_attempts SET started_at=started_at-interval '1 hour',finished_at=finished_at-interval '1 hour' WHERE shop_id=$1", f.shop)
			f.exec(t, "UPDATE notification_jobs SET dispatch_next_at=clock_timestamp()-interval '1 hour' WHERE shop_id=$1", f.shop)
		}
		if _, err := pipeline.Schedule(f.ctx); err != nil || f.count(t, "notification_outbox") != 5 || f.count(t, "notification_attempts") != 5 {
			t.Fatal("retry limit exceeded", err)
		}
	})
}
func TestNotificationSenderSerializationAndRecovery(t *testing.T) {
	f, _ := dispatchFixture(t, "request")
	var reminder uuid.UUID
	if err := f.pool.QueryRow(f.ctx, "SELECT id FROM notification_jobs WHERE shop_id=$1 AND kind='booking_reminder'", f.shop).Scan(&reminder); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var calls int
	sender := senderFunc(func(ctx context.Context, m app.OutboundNotification) (domain.SendOutcome, error) {
		calls++
		if calls == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return domain.SendOutcome{}, ctx.Err()
			}
		}
		return domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: m.AttemptID.String()}, nil
	})
	deliver := app.NewDeliverNotification(f.tx, f.cipher, sender)
	if r, err := deliver.Recover(f.ctx, f.key); err != nil || r.State != "ready" || calls != 0 {
		t.Fatal("maintenance initiated a send", r, err)
	}
	done := make(chan error, 1)
	go func() { _, err := deliver.Execute(f.ctx, f.key); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first sender never entered")
	}
	key := app.DeliveryKey{ShopID: f.shop, JobID: reminder}
	if r, err := deliver.Execute(f.ctx, key); err != nil || r.State != "sender_busy" {
		t.Fatal("same phone sent concurrently", r, err)
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r, err := deliver.Execute(f.ctx, key); err != nil || r.State != "accepted" || calls != 2 {
		t.Fatal("sender lease did not release", r, err)
	}
}
