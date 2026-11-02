//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/simulation"
	"github.com/google/uuid"
)

func deliveryActors(t *testing.T, f *deliveryFixture) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	owner, barber, other, professional := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec(t, "INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber'),($3,'Other barber')", owner, barber, other)
	f.exec(t, "INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Other professional')", professional, f.shop)
	f.exec(t, "INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$4,'owner',NULL),($2,$4,'barber',$5),($3,$4,'barber',$6)", owner, barber, other, f.shop, f.professional, professional)
	return owner, barber, other
}
func TestRequestDrivenNotifications(t *testing.T) {
	t.Run("manual window is protected at creation", func(t *testing.T) {
		f := newDeliveryFixtureMode(t, "request")
		owner, _, _ := deliveryActors(t, f)
		sender := simulation.NewSender(domain.SendAccepted)
		u := app.NewManageBookingMessages(f.tx, app.NewDeliverNotification(f.tx, f.cipher, sender), true)
		scope := app.BookingMessageScope{ActorID: owner, ShopID: f.shop, AppointmentID: f.appointment}
		view, err := u.Read(f.ctx, scope)
		if err != nil || len(view.Messages) != 2 {
			t.Fatal("missing message projection", err)
		}
		for _, m := range view.Messages {
			if m.Mode != "request" || !m.CanSend {
				t.Fatal("manual message not eligible", m.State, m.Mode)
			}
		}
		var payloadExpiry, starts time.Time
		if err := f.pool.QueryRow(f.ctx, "SELECT p.payload_expires_at,a.starts_at FROM appointment_notification_plans p JOIN appointments a ON a.shop_id=p.shop_id AND a.id=p.appointment_id WHERE p.shop_id=$1", f.shop).Scan(&payloadExpiry, &starts); err != nil || !payloadExpiry.Equal(starts.Add(-time.Minute)) {
			t.Fatal("manual retention was not planned before encryption", err)
		}
		r, err := u.Send(f.ctx, scope, "booking_reminder")
		if err != nil || r.State != "accepted" || sender.Calls() != 1 {
			t.Fatal("manual reminder did not use shared delivery", r, err)
		}
		r, err = u.Send(f.ctx, scope, "booking_reminder")
		if err != nil || r.State != "accepted" || sender.Calls() != 1 {
			t.Fatal("manual repeat sent again", r, err)
		}
	})
	t.Run("owner and assigned barber only", func(t *testing.T) {
		for _, kind := range []string{"owner", "barber", "other", "anonymous"} {
			t.Run(kind, func(t *testing.T) {
				f := newDeliveryFixtureMode(t, "request")
				owner, barber, other := deliveryActors(t, f)
				actor := map[string]uuid.UUID{"owner": owner, "barber": barber, "other": other}[kind]
				sender := simulation.NewSender(domain.SendAccepted)
				u := app.NewManageBookingMessages(f.tx, app.NewDeliverNotification(f.tx, f.cipher, sender), true)
				s := app.BookingMessageScope{ActorID: actor, ShopID: f.shop, AppointmentID: f.appointment}
				r, err := u.Send(f.ctx, s, "booking_reminder")
				if kind == "other" || kind == "anonymous" {
					if !errors.Is(err, app.ErrDeliveryAccessDenied) || sender.Calls() != 0 {
						t.Fatal("unassigned actor sent a message", r, err)
					}
				} else if err != nil || r.State != "accepted" {
					t.Fatal("valid staff denied", r, err)
				}
			})
		}
	})
	t.Run("revoked staff cannot begin after claim", func(t *testing.T) {
		f := newDeliveryFixtureMode(t, "request")
		_, barber, _ := deliveryActors(t, f)
		sender := simulation.NewSender(domain.SendAccepted)
		opener := openerFunc(func(ctx context.Context, now time.Time, s app.PayloadScope, p app.ProtectedPayload) (app.BookingSnapshot, error) {
			snapshot, err := f.cipher.Open(ctx, now, s, p)
			f.exec(t, "UPDATE shop_memberships SET active=FALSE WHERE user_id=$1 AND shop_id=$2", barber, f.shop)
			return snapshot, err
		})
		_, err := app.NewDeliverNotification(f.tx, opener, sender).Execute(f.ctx, app.DeliveryKey{ShopID: f.shop, JobID: f.key.JobID, ActorID: barber})
		if !errors.Is(err, app.ErrDeliveryAccessDenied) || sender.Calls() != 0 {
			t.Fatal("revocation ignored before I/O", err)
		}
	})
	t.Run("policy owner CAS and runtime capabilities", func(t *testing.T) {
		f := newDeliveryFixtureMode(t, "request")
		owner, barber, _ := deliveryActors(t, f)
		repository := notificationspg.NewSettings(notificationspg.NewConnections(f.pool, func(q *db.Queries) app.ConnectionOwnerAccess { return identitypg.NewConnectionOwnerGuard(q) }))
		settings := app.NewManageNotificationSettings(repository, true, false)
		s := app.ConnectionScope{ActorID: owner, ShopID: f.shop}
		current, err := settings.Read(f.ctx, s)
		if err != nil || !current.DeliveryAvailable || current.AutomaticAvailable {
			t.Fatal("wrong policy capabilities", err)
		}
		current.ReminderLeadMinutes = 120
		saved, err := settings.Save(f.ctx, s, current)
		if err != nil || saved.Version != current.Version+1 {
			t.Fatal("policy not saved", err)
		}
		if _, err = settings.Save(f.ctx, s, current); !errors.Is(err, app.ErrNotificationSettingsConflict) {
			t.Fatal("stale policy overwrite", err)
		}
		saved.DeliveryMode = "automatic"
		if _, err = settings.Save(f.ctx, s, saved); !errors.Is(err, app.ErrInvalidNotificationSettings) {
			t.Fatal("enabled automatic without worker availability", err)
		}
		if _, err = settings.Read(f.ctx, app.ConnectionScope{ActorID: barber, ShopID: f.shop}); !errors.Is(err, app.ErrConnectionAccessDenied) {
			t.Fatal("barber read owner policy", err)
		}
		automatic := app.NewManageNotificationSettings(repository, true, true)
		workerID := uuid.New()
		dispatch := notificationspg.NewDispatchStore(f.pool)
		if err := dispatch.Heartbeat(f.ctx, workerID, true); err != nil {
			t.Fatal(err)
		}
		current, err = automatic.Read(f.ctx, s)
		if err != nil || !current.AutomaticAvailable {
			t.Fatal("healthy worker unavailable", err)
		}
		f.exec(t, "UPDATE notification_worker_heartbeats SET checked_at=clock_timestamp()-interval '3 minutes' WHERE worker_id=$1", workerID)
		current, err = automatic.Read(f.ctx, s)
		if err != nil || current.AutomaticAvailable {
			t.Fatal("stale worker advertised as available", err)
		}
		if err := dispatch.Heartbeat(f.ctx, workerID, false); err != nil {
			t.Fatal(err)
		}
	})
}
