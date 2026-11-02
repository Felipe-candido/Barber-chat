//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/simulation"
	"github.com/google/uuid"
)

func TestNotificationProviderReceipts(t *testing.T) {
	t.Run("authenticated correlation resolves uncertainty and never regresses receipt", func(t *testing.T) {
		f := newDeliveryFixture(t)
		sender := simulation.NewSender(domain.SendUnknown)
		if _, err := f.run(sender); err != nil {
			t.Fatal(err)
		}
		r := app.ProviderReceipt{Stage: 3}
		if err := f.pool.QueryRow(f.ctx, "SELECT attempt_id,instance_name FROM notification_attempts WHERE shop_id=$1", f.shop).Scan(&r.AttemptID, &r.InstanceName); err != nil {
			t.Fatal(err)
		}
		u := app.NewRecordProviderReceipt(notificationspg.NewReceiptStore(f.pool))
		foreign := r
		foreign.InstanceName = "bc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err := u.RecordReceipt(f.ctx, foreign); err != nil || f.state(t) != "unknown" {
			t.Fatal("foreign instance changed attempt", err)
		}
		foreign = r
		foreign.AttemptID = uuid.New()
		if err := u.RecordReceipt(f.ctx, foreign); err != nil || f.state(t) != "unknown" {
			t.Fatal("foreign correlation changed attempt", err)
		}
		for _, stage := range []int16{3, 1, 2, 3} {
			r.Stage = stage
			if err := u.RecordReceipt(f.ctx, r); err != nil {
				t.Fatal(err)
			}
		}
		var stage int16
		var leaseReleased bool
		if err := f.pool.QueryRow(f.ctx, "SELECT a.receipt_state,c.send_token IS NULL FROM notification_attempts a JOIN shop_whatsapp_connections c USING(shop_id) WHERE a.shop_id=$1", f.shop).Scan(&stage, &leaseReleased); err != nil {
			t.Fatal(err)
		}
		if stage != 3 || f.state(t) != "accepted" || !leaseReleased {
			t.Fatal("receipt regression or acceptance lost")
		}
		if _, err := f.run(sender); err != nil || sender.Calls() != 1 {
			t.Fatal("receipt resolution repeated send", err)
		}
	})
	t.Run("receipt before lost HTTP response wins final durable result", func(t *testing.T) {
		f := newDeliveryFixture(t)
		u := app.NewRecordProviderReceipt(notificationspg.NewReceiptStore(f.pool))
		sender := senderFunc(func(ctx context.Context, m app.OutboundNotification) (domain.SendOutcome, error) {
			if err := u.RecordReceipt(ctx, app.ProviderReceipt{InstanceName: m.InstanceName, AttemptID: m.AttemptID, Stage: 2}); err != nil {
				return domain.SendOutcome{}, err
			}
			return domain.SendOutcome{Disposition: domain.SendUnknown, Reason: domain.ReasonTransportUnknown}, nil
		})
		r, err := f.run(sender)
		if err != nil || r.State != "accepted" || f.state(t) != "accepted" {
			t.Fatal("late HTTP result regressed receipt evidence", r, err)
		}
	})
}

func TestNotificationPayloadRetention(t *testing.T) {
	n := newNotificationFixture(t)
	// Both intervals are calculated before encryption; changing a stored expiry
	// after encryption would invalidate the authenticated scope.
	n.exec(t, "UPDATE shop_notification_settings SET reminder_lead_minutes=10080 WHERE shop_id=$1", n.shop)
	n.configure(true, false, booking.ClockFunc(func() time.Time { return time.Now().Add(-3 * time.Hour) }))
	saved := n.reserve(t, true, bookingKey())
	fresh := newDeliveryFixtureMode(t, "request")
	store := notificationspg.NewDispatchStore(n.pool)
	if err := store.Maintenance(n.ctx); err != nil {
		t.Fatal(err)
	}
	var purged, kept, bookingKept bool
	if err := n.pool.QueryRow(n.ctx, "SELECT encrypted_payload IS NULL AND payload_purged_at>=payload_expires_at FROM appointment_notification_plans WHERE shop_id=$1", n.shop).Scan(&purged); err != nil {
		t.Fatal(err)
	}
	if err := n.pool.QueryRow(n.ctx, "SELECT EXISTS(SELECT 1 FROM appointments WHERE shop_id=$1 AND id=$2)", n.shop, saved.Appointment.ID).Scan(&bookingKept); err != nil {
		t.Fatal(err)
	}
	if err := fresh.pool.QueryRow(fresh.ctx, "SELECT encrypted_payload IS NOT NULL AND payload_purged_at IS NULL FROM appointment_notification_plans WHERE shop_id=$1", fresh.shop).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if !purged || !bookingKept || !kept {
		t.Fatal("retention removed history, retained expired content or removed live content")
	}
	if err := store.Maintenance(n.ctx); err != nil {
		t.Fatal("maintenance not idempotent", err)
	}
	if _, err := store.Metrics(n.ctx); err != nil {
		t.Fatal("operational aggregates failed", err)
	}
}
