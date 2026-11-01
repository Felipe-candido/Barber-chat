//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	customerspg "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/simulation"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
)

type deliveryFixture struct {
	*notificationFixture
	key         app.DeliveryKey
	appointment uuid.UUID
	tx          *notificationspg.DeliveryTransactor
}

func newDeliveryFixture(t *testing.T) *deliveryFixture {
	t.Helper()
	n := newNotificationFixture(t)
	saved := n.reserve(t, true, bookingKey())
	f := &deliveryFixture{notificationFixture: n, appointment: saved.Appointment.ID, key: app.DeliveryKey{ShopID: n.shop}}
	if err := n.pool.QueryRow(n.ctx, "SELECT id FROM notification_jobs WHERE shop_id=$1 AND kind='booking_confirmation'", n.shop).Scan(&f.key.JobID); err != nil {
		t.Fatal(err)
	}
	n.exec(t, `INSERT INTO shop_whatsapp_connections(shop_id,instance_name,state,desired_connected,verified_at)
 VALUES($1,$2,'connected',TRUE,clock_timestamp())`, n.shop, "bc_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	f.tx = notificationspg.NewDeliveryTransactor(n.pool, bindDeliveryQueries)
	return f
}
func bindDeliveryQueries(q *db.Queries) app.DeliveryTx {
	return app.DeliveryTx{Store: notificationspg.NewDeliveryStore(q), Shops: shopspg.NewMessageShops(q), Permissions: customerspg.NewBookingPermissions(q), Bookings: bookingpg.NewNotificationDeliveryBookings(q)}
}
func (f *deliveryFixture) run(sender app.NotificationSender) (app.DeliveryResult, error) {
	return app.NewDeliverNotification(f.tx, f.cipher, sender).Execute(f.ctx, f.key)
}
func (f *deliveryFixture) state(t *testing.T) string {
	t.Helper()
	var state string
	if err := f.pool.QueryRow(f.ctx, "SELECT delivery_state FROM notification_jobs WHERE shop_id=$1 AND id=$2", f.shop, f.key.JobID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}
func (f *deliveryFixture) cancelBooking(t *testing.T) {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if _, err = tx.Exec(f.ctx, "SELECT id FROM professionals WHERE shop_id=$1 AND id=$2 FOR UPDATE", f.shop, f.professional); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.ctx, "UPDATE appointments SET status='cancelled',version=version+1,updated_at=clock_timestamp(),status_source='public',status_reason='Delivery cancellation test',status_actor_id=NULL WHERE shop_id=$1 AND id=$2", f.shop, f.appointment); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
}

type senderFunc func(context.Context, app.OutboundNotification) (domain.SendOutcome, error)

func (f senderFunc) Send(c context.Context, m app.OutboundNotification) (domain.SendOutcome, error) {
	return f(c, m)
}

type openerFunc func(context.Context, time.Time, app.PayloadScope, app.ProtectedPayload) (app.BookingSnapshot, error)

func (f openerFunc) Open(c context.Context, n time.Time, s app.PayloadScope, p app.ProtectedPayload) (app.BookingSnapshot, error) {
	return f(c, n, s, p)
}

func TestNotificationDelivery(t *testing.T) {
	t.Run("outcomes and replay do not send again", func(t *testing.T) {
		for _, disposition := range []domain.SendDisposition{domain.SendAccepted, domain.SendNotSent, domain.SendUnknown} {
			t.Run(string(disposition), func(t *testing.T) {
				f := newDeliveryFixture(t)
				sender := simulation.NewSender(disposition)
				r, err := f.run(sender)
				want := map[domain.SendDisposition]string{domain.SendAccepted: "accepted", domain.SendNotSent: "failed", domain.SendUnknown: "unknown"}[disposition]
				if err != nil || r.State != want || f.state(t) != want {
					t.Fatal("wrong outcome", r, err)
				}
				r, err = f.run(sender)
				if err != nil || r.State != want || sender.Calls() != 1 || f.count(t, "notification_attempts") != 1 {
					t.Fatal("replay repeated effect", r, err)
				}
			})
		}
	})
	t.Run("two callers one attempt and no SQL lock during sender", func(t *testing.T) {
		f := newDeliveryFixture(t)
		entered, release := make(chan struct{}), make(chan struct{})
		defer close(release)
		sender := senderFunc(func(ctx context.Context, m app.OutboundNotification) (domain.SendOutcome, error) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return domain.SendOutcome{}, ctx.Err()
			}
			return domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: "one_attempt"}, nil
		})
		done := make(chan error, 1)
		go func() { _, err := f.run(sender); done <- err }()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("sender never started")
		}
		// Acquiring both NOWAIT locks proves the sender is outside the transaction.
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(f.ctx, "SELECT id FROM professionals WHERE shop_id=$1 AND id=$2 FOR UPDATE NOWAIT", f.shop, f.professional)
		if err == nil {
			_, err = tx.Exec(f.ctx, "SELECT id FROM notification_jobs WHERE shop_id=$1 AND id=$2 FOR UPDATE NOWAIT", f.shop, f.key.JobID)
		}
		_ = tx.Rollback(f.ctx)
		if err != nil {
			t.Fatal("external I/O held SQL locks", err)
		}
		results := make(chan app.DeliveryResult, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				r, err := f.run(sender)
				if err != nil {
					results <- app.DeliveryResult{State: "error"}
					return
				}
				results <- r
			})
		}
		wg.Wait()
		close(results)
		for r := range results {
			if r.State != "busy" {
				t.Fatal("concurrent sender not fenced", r)
			}
		}
		// Cancellation is allowed after start; it cannot retract the external effect.
		f.cancelBooking(t)
		release <- struct{}{}
		if err := <-done; err != nil || f.state(t) != "accepted" || f.count(t, "notification_attempts") != 1 {
			t.Fatal("inflight evidence lost", err)
		}
	})
	t.Run("revalidate after decrypt before send", func(t *testing.T) {
		for _, change := range []string{"cancel", "revoke", "disable", "phone", "disconnect"} {
			t.Run(change, func(t *testing.T) {
				f := newDeliveryFixture(t)
				sender := simulation.NewSender(domain.SendAccepted)
				opener := openerFunc(func(ctx context.Context, now time.Time, scope app.PayloadScope, p app.ProtectedPayload) (app.BookingSnapshot, error) {
					snapshot, err := f.cipher.Open(ctx, now, scope, p)
					if err != nil {
						return snapshot, err
					}
					switch change {
					case "cancel":
						f.cancelBooking(t)
					case "revoke":
						f.exec(t, "UPDATE customer_booking_message_permissions SET revoked_at=clock_timestamp() WHERE shop_id=$1", f.shop)
					case "disable":
						f.exec(t, "UPDATE shop_notification_settings SET enabled=FALSE WHERE shop_id=$1", f.shop)
					case "phone":
						f.exec(t, "UPDATE customers SET phone_e164='+5511988887777' WHERE shop_id=$1", f.shop)
					case "disconnect":
						f.exec(t, "UPDATE shop_whatsapp_connections SET state='disconnected' WHERE shop_id=$1", f.shop)
					}
					return snapshot, nil
				})
				r, err := app.NewDeliverNotification(f.tx, opener, sender).Execute(f.ctx, f.key)
				want := "invalidated"
				if change == "disconnect" {
					want = "connection_unavailable"
				}
				if err != nil || r.State != want || sender.Calls() != 0 || f.count(t, "notification_attempts") != 0 {
					t.Fatal("eligibility was not revalidated", r, err)
				}
				if change == "disconnect" && f.state(t) != "ready" {
					t.Fatal("unstarted lease was not released")
				}
			})
		}
	})
	t.Run("window connection and tenant guards", func(t *testing.T) {
		for _, change := range []string{"not_due", "expired", "stale", "foreign"} {
			t.Run(change, func(t *testing.T) {
				f := newDeliveryFixture(t)
				sender := simulation.NewSender(domain.SendAccepted)
				key := f.key
				switch change {
				case "not_due":
					if err := f.pool.QueryRow(f.ctx, "SELECT id FROM notification_jobs WHERE shop_id=$1 AND kind='booking_reminder'", f.shop).Scan(&key.JobID); err != nil {
						t.Fatal(err)
					}
				case "expired":
					f.exec(t, "UPDATE notification_jobs SET due_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 hour' WHERE shop_id=$1 AND id=$2", f.shop, key.JobID)
				case "stale":
					f.exec(t, "UPDATE shop_whatsapp_connections SET verified_at=clock_timestamp()-interval '2 minutes' WHERE shop_id=$1", f.shop)
				case "foreign":
					key.ShopID = uuid.New()
				}
				r, err := app.NewDeliverNotification(f.tx, f.cipher, sender).Execute(f.ctx, key)
				want := map[string]string{"not_due": "not_due", "expired": "expired", "stale": "connection_unavailable", "foreign": "not_found"}[change]
				if err != nil || r.State != want || sender.Calls() != 0 {
					t.Fatal("guard failed", r, err)
				}
			})
		}
	})
	t.Run("provider error is uncertain and private", func(t *testing.T) {
		f := newDeliveryFixture(t)
		r, err := f.run(senderFunc(func(context.Context, app.OutboundNotification) (domain.SendOutcome, error) {
			return domain.SendOutcome{Disposition: domain.SendNotSent, Reason: domain.ReasonProviderRejected}, errors.New("private response body")
		}))
		if err != nil || r.State != "unknown" || r.Reason != domain.ReasonTransportUnknown {
			t.Fatal("uncertain result classified as not sent", r, err)
		}
	})
	t.Run("expired unstarted lease reclaimed and stale owner fenced", func(t *testing.T) {
		f := newDeliveryFixture(t)
		old := app.DeliveryClaim{Key: f.key, Token: uuid.New()}
		if err := f.tx.WithinDeliveryTx(f.ctx, func(ctx context.Context, tx app.DeliveryTx) error { return tx.Store.Claim(ctx, old) }); err != nil {
			t.Fatal(err)
		}
		f.exec(t, "UPDATE notification_jobs SET delivery_lease_until=clock_timestamp()-interval '1 second' WHERE shop_id=$1 AND id=$2", f.shop, f.key.JobID)
		sender := simulation.NewSender(domain.SendAccepted)
		if r, err := f.run(sender); err != nil || r.State != "accepted" {
			t.Fatal(r, err)
		}
		err := f.tx.WithinDeliveryTx(f.ctx, func(ctx context.Context, tx app.DeliveryTx) error {
			return tx.Store.Start(ctx, old, "bc_"+strings.Repeat("a", 32))
		})
		if !errors.Is(err, app.ErrDeliveryLeaseLost) || sender.Calls() != 1 {
			t.Fatal("stale lease started another attempt", err)
		}
	})
}

// Faults distinguish a lost commit response from a transaction that rolled back.
type deliveryFaultTx struct {
	base        app.DeliveryTransactor
	calls       int
	failAt      int
	afterCommit bool
}

func (f *deliveryFaultTx) WithinDeliveryTx(ctx context.Context, fn func(context.Context, app.DeliveryTx) error) error {
	f.calls++
	if f.calls != f.failAt {
		return f.base.WithinDeliveryTx(ctx, fn)
	}
	fault := errors.New("simulated database failure")
	if f.afterCommit {
		if err := f.base.WithinDeliveryTx(ctx, fn); err != nil {
			return err
		}
		return fault
	}
	return f.base.WithinDeliveryTx(ctx, func(ctx context.Context, tx app.DeliveryTx) error {
		if err := fn(ctx, tx); err != nil {
			return err
		}
		return fault
	})
}

func TestNotificationDeliveryRecovery(t *testing.T) {
	for _, phase := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("failure_at_phase_%d", phase), func(t *testing.T) {
			f := newDeliveryFixture(t)
			sender := simulation.NewSender(domain.SendAccepted)
			fault := &deliveryFaultTx{base: f.tx, failAt: phase, afterCommit: phase < 3}
			r, err := app.NewDeliverNotification(fault, f.cipher, sender).Execute(f.ctx, f.key)
			if !errors.Is(err, app.ErrDeliveryUnavailable) {
				t.Fatal("missing persistence error", r, err)
			}
			wantCalls := int64(0)
			if phase == 3 {
				wantCalls = 1
			}
			if sender.Calls() != wantCalls {
				t.Fatal("sender crossed uncertain commit boundary")
			}
			f.exec(t, "UPDATE notification_jobs SET delivery_lease_until=clock_timestamp()-interval '1 second' WHERE shop_id=$1 AND id=$2", f.shop, f.key.JobID)
			r, err = f.run(sender)
			if phase == 1 {
				if err != nil || r.State != "accepted" || sender.Calls() != 1 {
					t.Fatal("unstarted work not recovered", r, err)
				}
				return
			}
			if err != nil || r.State != "unknown" || sender.Calls() != wantCalls {
				t.Fatal("started work repeated", r, err)
			}
			// Conclusive late evidence for that exact attempt may resolve unknown, even
			// after cancellation. It cannot authorize a new external call.
			var token uuid.UUID
			if err := f.pool.QueryRow(f.ctx, "SELECT delivery_token FROM notification_jobs WHERE shop_id=$1 AND id=$2", f.shop, f.key.JobID).Scan(&token); err != nil {
				t.Fatal(err)
			}
			f.cancelBooking(t)
			if err := f.tx.WithinDeliveryTx(f.ctx, func(ctx context.Context, tx app.DeliveryTx) error {
				return tx.Store.Complete(ctx, app.DeliveryClaim{Key: f.key, Token: token}, domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: "late_evidence"})
			}); err != nil || f.state(t) != "accepted" {
				t.Fatal("late evidence lost", err)
			}
		})
	}
}
