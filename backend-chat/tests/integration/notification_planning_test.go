//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	bookingsecurity "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	customers "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	customerspg "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	notificationsinfra "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	notificationssecurity "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/security"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type notificationFixture struct {
	*bookingFixture
	cipher *notificationssecurity.PayloadCipher
	tx     *bookingpg.Transactor
	create *booking.CreateAppointment
}

type failSavedNotificationPlan struct{ notifications.PlanningStore }

func (s failSavedNotificationPlan) Save(ctx context.Context, i notifications.PrepareBookingNotificationsInput, p notifications.PreparedBookingNotifications) error {
	if err := s.PlanningStore.Save(ctx, i, p); err != nil {
		return err
	}
	return errors.New("injected failure after permission, protected plan and jobs")
}

func newNotificationFixture(t *testing.T) *notificationFixture {
	t.Helper()
	dsn := requiredEnv(t, "NOTIFICATIONS_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_booking_notifications_") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("NOTIFICATIONS_TEST_DATABASE_URL must name a local disposable codex_booking_notifications_* database")
	}
	t.Setenv("BOOKING_TEST_DATABASE_URL", dsn)
	f := newBookingFixture(t)
	cipher, err := notificationssecurity.NewPayloadCipher("test-key", map[string][]byte{"test-key": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	n := &notificationFixture{bookingFixture: f, cipher: cipher}
	n.exec(t, "INSERT INTO shop_addresses(shop_id,postal_code,street,number,district,city,state) VALUES($1,'01001000','Test Street','123','Center','Test City','SP')", f.shop)
	n.exec(t, "INSERT INTO shop_notification_settings(shop_id,enabled) VALUES($1,TRUE)", f.shop)
	n.configure(true, false, booking.ClockFunc(time.Now))
	return n
}

func (n *notificationFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := n.pool.Exec(n.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}
func (n *notificationFixture) count(t *testing.T, table string) int {
	t.Helper()
	var c int
	if err := n.pool.QueryRow(n.ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE shop_id=$1", table), n.shop).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (n *notificationFixture) configure(enabled, fail bool, clock booking.Clock) {
	n.tx = bookingpg.NewTransactor(n.pool, func(q *db.Queries) booking.BookingTx {
		b := bindBookingQueries(q)
		var store notifications.PlanningStore = notificationspg.NewPlanningStore(q)
		if fail {
			store = failSavedNotificationPlan{store}
		}
		b.Notifications = notificationsinfra.NewBookingPlanner(notifications.NewPlanBookingNotifications(enabled,
			notifications.NewPrepareBookingNotifications(n.cipher), store, customerspg.NewBookingPermissions(q), shopspg.NewMessageShops(q)))
		return b
	})
	n.create = booking.NewCreateAppointment(n.tx, customers.ContactNormalizer{}, bookingsecurity.Codes{}, clock)
	h := bookinghttp.NewHandler(n.create, booking.NewListAvailability(n.tx, clock), booking.NewListProfessionals(n.tx, clock),
		booking.NewManageAppointment(n.tx, bookingsecurity.Codes{}, clock), 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	n.handler = httpapi.NewHandler(httpapi.HandlerConfig{Booking: h, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func (n *notificationFixture) reserve(t *testing.T, permission bool, key string) bookingHTTPResult {
	t.Helper()
	body := n.body(n.start.Add(48*time.Hour), n.service)
	body["whatsapp_notifications"] = permission
	return bookingResult(t, n.request("POST", "/appointments", body, key), 201)
}

func TestNotificationPlanning(t *testing.T) {
	t.Run("atomic encrypted plan replay and permission fingerprint", func(t *testing.T) {
		n := newNotificationFixture(t)
		key := bookingKey()
		saved := n.reserve(t, true, key)
		if n.count(t, "notification_jobs") != 2 || n.count(t, "customer_booking_message_permissions") != 1 || n.count(t, "appointment_notification_plans") != 1 {
			t.Fatal("missing atomic plan")
		}
		var scope notifications.PayloadScope
		var payload notifications.ProtectedPayload
		var due time.Time
		err := n.pool.QueryRow(n.ctx, `SELECT p.shop_id,p.appointment_id,p.schedule_generation,p.payload_expires_at,p.payload_version,p.payload_key_id,p.encrypted_payload,j.due_at
            FROM appointment_notification_plans p JOIN notification_jobs j USING(shop_id,appointment_id,schedule_generation)
            WHERE p.shop_id=$1 AND j.kind='booking_reminder'`, n.shop).Scan(&scope.ShopID, &scope.AppointmentID, &scope.ScheduleGeneration, &scope.ExpiresAt, &payload.Version, &payload.KeyID, &payload.Ciphertext, &due)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := n.cipher.Open(n.ctx, due, scope, payload)
		if err != nil || opened.AppointmentCode != saved.Code || opened.RecipientPhone != "+5511912345678" || opened.ShopAddress == "" {
			t.Fatal("persisted snapshot cannot be recovered", err)
		}
		if bytes.Contains(payload.Ciphertext, []byte(saved.Code)) {
			t.Fatal("code stored in clear")
		}
		body := n.body(n.start.Add(48*time.Hour), n.service)
		body["whatsapp_notifications"] = true
		replay := bookingResult(t, n.request("POST", "/appointments", body, key), 200)
		if replay.Code != saved.Code || !replay.Replayed || n.count(t, "notification_jobs") != 2 {
			t.Fatal("replay duplicated jobs")
		}
		body["whatsapp_notifications"] = false
		if w := n.request("POST", "/appointments", body, key); w.Code != 409 {
			t.Fatal("permission change did not conflict", w.Code)
		}
	})
	t.Run("rollback includes booking contact permission plan and jobs", func(t *testing.T) {
		n := newNotificationFixture(t)
		n.configure(true, true, booking.ClockFunc(time.Now))
		body := n.body(n.start.Add(48*time.Hour), n.service)
		body["whatsapp_notifications"] = true
		if w := n.request("POST", "/appointments", body, bookingKey()); w.Code != 500 {
			t.Fatal("injected failure was not returned", w.Code)
		}
		for _, table := range []string{"appointments", "customers", "booking_requests", "customer_booking_message_permissions", "appointment_notification_plans", "notification_jobs"} {
			if n.count(t, table) != 0 {
				t.Fatal("partial transaction survived", table)
			}
		}
	})
	t.Run("permission missing and disabled policy keep booking usable", func(t *testing.T) {
		for _, disabled := range []bool{false, true} {
			t.Run(fmt.Sprint(disabled), func(t *testing.T) {
				n := newNotificationFixture(t)
				if disabled {
					n.exec(t, "UPDATE shop_notification_settings SET enabled=FALSE,version=version+1 WHERE shop_id=$1", n.shop)
				}
				n.reserve(t, disabled, bookingKey())
				var reason string
				var noPayload bool
				if err := n.pool.QueryRow(n.ctx, "SELECT skip_reason,encrypted_payload IS NULL FROM appointment_notification_plans WHERE shop_id=$1", n.shop).Scan(&reason, &noPayload); err != nil {
					t.Fatal(err)
				}
				expected := "permission_missing"
				if disabled {
					expected = "policy_disabled"
				}
				if reason != expected || !noPayload || n.count(t, "notification_jobs") != 0 {
					t.Fatal("skipped plan retained sendable content")
				}
			})
		}
	})
	t.Run("deployment off and legacy receipt fingerprints", func(t *testing.T) {
		n := newNotificationFixture(t)
		n.configure(false, false, booking.ClockFunc(time.Now))
		key := bookingKey()
		saved := n.reserve(t, false, key)
		if n.count(t, "appointment_notification_plans") != 0 || n.count(t, "notification_jobs") != 0 {
			t.Fatal("disabled deployment planned work")
		}
		// Reconstruct the pre-notification canonical command exactly.
		legacy := struct {
			ShopSlug                  string
			ServiceID, ProfessionalID uuid.UUID
			StartsAt                  time.Time
			Customer                  booking.CustomerInput
			IdempotencyKey            string
		}{
			ShopSlug: n.slug, ServiceID: n.service, ProfessionalID: n.professional, StartsAt: n.start.Add(48 * time.Hour),
			Customer: booking.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "+5511912345678", PhoneCountry: "BR"},
		}
		data, _ := json.Marshal(legacy)
		raw, _ := hex.DecodeString(key)
		mac := hmac.New(sha256.New, raw)
		_, _ = mac.Write(data)
		var stored []byte
		if err := n.pool.QueryRow(n.ctx, "SELECT fingerprint FROM booking_requests WHERE shop_id=$1", n.shop).Scan(&stored); err != nil || !hmac.Equal(stored, mac.Sum(nil)) {
			t.Fatal("legacy fingerprint changed", err)
		}
		n.configure(true, false, booking.ClockFunc(time.Now))
		body := n.body(n.start.Add(48*time.Hour), n.service)
		replay := bookingResult(t, n.request("POST", "/appointments", body, key), 200)
		if replay.Code != saved.Code || n.count(t, "notification_jobs") != 0 {
			t.Fatal("enabling planning backfilled a replay")
		}
	})
	t.Run("public cancellation invalidates even with planning disabled", func(t *testing.T) {
		n := newNotificationFixture(t)
		saved := n.reserve(t, true, bookingKey())
		n.configure(false, false, booking.ClockFunc(time.Now))
		bookingResult(t, n.request("POST", "/appointments/cancel", map[string]any{"code": saved.Code}, ""), 200)
		var pending int
		if err := n.pool.QueryRow(n.ctx, "SELECT count(*) FROM notification_jobs WHERE shop_id=$1 AND state='pending'", n.shop).Scan(&pending); err != nil || pending != 0 {
			t.Fatal("cancelled booking retained pending jobs", err)
		}
	})
	t.Run("owner cancellation and permission origin", func(t *testing.T) {
		n := newNotificationFixture(t)
		owner := uuid.New()
		n.exec(t, "INSERT INTO users(id,display_name) VALUES($1,'Test Owner')", owner)
		n.exec(t, "INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, n.shop)
		input := booking.CreateAppointmentInput{ShopSlug: n.slug, ServiceID: n.service, ProfessionalID: n.professional, StartsAt: n.start.Add(48 * time.Hour), IdempotencyKey: bookingKey(), WhatsAppNotifications: true,
			Customer: booking.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "11912345678", PhoneCountry: "BR"}}
		saved, err := n.create.ExecuteForStaff(n.ctx, input, owner, n.shop, uuid.Nil)
		if err != nil {
			t.Fatal(err)
		}
		var source string
		var actor uuid.UUID
		if err := n.pool.QueryRow(n.ctx, "SELECT source,actor_id FROM customer_booking_message_permissions WHERE shop_id=$1", n.shop).Scan(&source, &actor); err != nil || source != "staff_booking" || actor != owner {
			t.Fatal("staff permission lost actor", err)
		}
		tx := bookingpg.NewLifecycleTransactor(n.pool, func(q *db.Queries) booking.AppointmentLifecycleTx {
			return booking.AppointmentLifecycleTx{Access: identitypg.NewRepository(q), Store: bookingpg.NewLifecycleStore(q)}
		})
		_, err = booking.NewManageAppointmentStatus(tx).Change(n.ctx, booking.ChangeAppointmentStatusInput{Scope: booking.LifecycleScope{ActorID: owner, ShopID: n.shop}, AppointmentID: saved.Appointment.AppointmentID, Status: domain.StatusCancelled, ExpectedVersion: 1, Reason: "Test cancellation", IdempotencyKey: bookingKey()})
		if err != nil {
			t.Fatal(err)
		}
		var invalidated int
		_ = n.pool.QueryRow(n.ctx, "SELECT count(*) FROM notification_jobs WHERE shop_id=$1 AND state='invalidated'", n.shop).Scan(&invalidated)
		if invalidated != 2 {
			t.Fatal("owner cancellation did not invalidate jobs")
		}
	})
	t.Run("revocation and shop disable invalidate pending jobs", func(t *testing.T) {
		for _, revoke := range []bool{true, false} {
			t.Run(fmt.Sprint(revoke), func(t *testing.T) {
				n := newNotificationFixture(t)
				n.reserve(t, true, bookingKey())
				if revoke {
					n.exec(t, "UPDATE customer_booking_message_permissions SET revoked_at=clock_timestamp() WHERE shop_id=$1", n.shop)
				} else {
					n.exec(t, "UPDATE shop_notification_settings SET enabled=FALSE,version=version+1 WHERE shop_id=$1", n.shop)
				}
				var pending int
				if err := n.pool.QueryRow(n.ctx, "SELECT count(*) FROM notification_jobs WHERE shop_id=$1 AND state='pending'", n.shop).Scan(&pending); err != nil || pending != 0 {
					t.Fatal("revoked settings retained pending jobs", err)
				}
			})
		}
	})
	t.Run("uniqueness and cross tenant protection", func(t *testing.T) {
		n := newNotificationFixture(t)
		n.reserve(t, true, bookingKey())
		_, err := n.pool.Exec(n.ctx, `INSERT INTO notification_jobs(id,shop_id,appointment_id,schedule_generation,kind,due_at,expires_at)
            SELECT gen_random_uuid(),shop_id,appointment_id,schedule_generation,kind,due_at,expires_at FROM notification_jobs WHERE shop_id=$1 AND kind='booking_confirmation'`, n.shop)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatal("duplicate logical job accepted", err)
		}
		foreign := uuid.New()
		n.exec(t, "INSERT INTO shops(id,name,slug) VALUES($1,'Other',$2)", foreign, "other-"+foreign.String())
		_, err = n.pool.Exec(n.ctx, `INSERT INTO notification_jobs(id,shop_id,appointment_id,schedule_generation,kind,due_at,expires_at)
            SELECT gen_random_uuid(),$2,appointment_id,schedule_generation,kind,due_at,expires_at FROM notification_jobs WHERE shop_id=$1 AND kind='booking_confirmation'`, n.shop, foreign)
		if !errors.As(err, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
			t.Fatal("cross tenant job accepted", err)
		}
	})
	t.Run("concurrent replay creates one notification plan", func(t *testing.T) {
		n := newNotificationFixture(t)
		key := bookingKey()
		body := n.body(n.start.Add(48*time.Hour), n.service)
		body["whatsapp_notifications"] = true
		start := make(chan struct{})
		results := make(chan int, 2)
		for range 2 {
			go func() { <-start; results <- n.request("POST", "/appointments", body, key).Code }()
		}
		close(start)
		a, b := <-results, <-results
		if !((a == 201 && b == 200) || (a == 200 && b == 201)) || n.count(t, "appointments") != 1 || n.count(t, "notification_jobs") != 2 {
			t.Fatal("concurrent replay duplicated durable work", a, b)
		}
	})
	t.Run("cancel rollback restores appointment and pending jobs", func(t *testing.T) {
		n := newNotificationFixture(t)
		saved := n.reserve(t, true, bookingKey())
		intentional := errors.New("test rollback after cancellation")
		err := n.tx.WithinBookingTx(n.ctx, func(ctx context.Context, tx booking.BookingTx) error {
			if err := tx.Schedule.LockProfessionalSchedule(ctx, n.shop, n.professional); err != nil {
				return err
			}
			digest, _ := (bookingsecurity.Codes{}).Hash(saved.Code)
			a, found, err := tx.Appointments.FindByCode(ctx, n.shop, digest)
			if err != nil || !found {
				return errors.New("missing appointment")
			}
			changed, err := a.Cancel(time.Now())
			if err != nil {
				return err
			}
			if err := tx.Appointments.SaveCancellation(ctx, changed); err != nil {
				return err
			}
			return intentional
		})
		if !errors.Is(err, intentional) {
			t.Fatal(err)
		}
		var pending int
		var state string
		if err := n.pool.QueryRow(n.ctx, "SELECT count(*) FROM notification_jobs WHERE shop_id=$1 AND state='pending'", n.shop).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if err := n.pool.QueryRow(n.ctx, "SELECT status FROM appointments WHERE shop_id=$1", n.shop).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if pending != 2 || state != "scheduled" {
			t.Fatal("cancellation escaped its transaction")
		}
	})
	t.Run("terminal states invalidate pending messages", func(t *testing.T) {
		for _, target := range []string{"completed", "no_show", "automatic"} {
			t.Run(target, func(t *testing.T) {
				n := newNotificationFixture(t)
				owner, customer, id := uuid.New(), uuid.New(), uuid.New()
				n.exec(t, "INSERT INTO users(id,display_name) VALUES($1,'Owner')", owner)
				n.exec(t, "INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, n.shop)
				n.exec(t, "INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at) VALUES($1,$2,'Test','Customer','+5511912345678',now())", customer, n.shop)
				end := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Second)
				code, digest, err := (bookingsecurity.Codes{}).New()
				if err != nil {
					t.Fatal(err)
				}
				appointment := domain.Appointment{ID: id, ShopID: n.shop, CustomerID: customer, ServiceID: n.service, ProfessionalID: n.professional,
					Service: domain.ServiceSnapshot{Name: "Haircut", DurationMinutes: 40, PriceCents: 5000, Currency: "BRL"}, ProfessionalName: "Test barber", ShopTimezone: "America/Sao_Paulo",
					StartsAt: end.Add(-time.Hour), EndsAt: end, CreatedAt: end.Add(-72 * time.Hour), UpdatedAt: end.Add(-72 * time.Hour), Status: domain.StatusScheduled, Version: 1}
				// Seed an ended but otherwise valid appointment to exercise the real
				// lifecycle executor, without sleeps or modifying its activation policy.
				err = n.tx.WithinBookingTx(n.ctx, func(ctx context.Context, tx booking.BookingTx) error {
					if err := tx.Schedule.LockProfessionalSchedule(ctx, n.shop, n.professional); err != nil {
						return err
					}
					if _, err := tx.Appointments.Insert(ctx, appointment, digest); err != nil {
						return err
					}
					return tx.Notifications.Plan(ctx, booking.BookingNotificationInput{Appointment: appointment, ShopName: "Booking fixture", ShopSlug: n.slug,
						Customer: booking.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "+5511912345678"}, Code: code, WhatsAppNotifications: true})
				})
				if err != nil {
					t.Fatal(err)
				}
				if target == "automatic" {
					if _, err := bookingpg.NewLifecycleStore(db.New(n.pool)).CompleteDue(n.ctx, 100); err != nil {
						t.Fatal(err)
					}
				} else {
					tx := bookingpg.NewLifecycleTransactor(n.pool, func(q *db.Queries) booking.AppointmentLifecycleTx {
						return booking.AppointmentLifecycleTx{Access: identitypg.NewRepository(q), Store: bookingpg.NewLifecycleStore(q)}
					})
					_, err := booking.NewManageAppointmentStatus(tx).Change(n.ctx, booking.ChangeAppointmentStatusInput{Scope: booking.LifecycleScope{ActorID: owner, ShopID: n.shop}, AppointmentID: id, Status: domain.Status(target), ExpectedVersion: 1, Reason: "Test terminal state", IdempotencyKey: bookingKey()})
					if err != nil {
						t.Fatal(err)
					}
				}
				var pending int
				var status string
				if err := n.pool.QueryRow(n.ctx, "SELECT count(*) FROM notification_jobs WHERE shop_id=$1 AND state='pending'", n.shop).Scan(&pending); err != nil {
					t.Fatal(err)
				}
				if err := n.pool.QueryRow(n.ctx, "SELECT status FROM appointments WHERE shop_id=$1 AND id=$2", n.shop, id).Scan(&status); err != nil {
					t.Fatal(err)
				}
				expected := target
				if target == "automatic" {
					expected = "completed"
				}
				if pending != 0 || status != expected {
					t.Fatal("terminal appointment retained pending notifications", status)
				}
			})
		}
	})
	t.Run("concurrent job insertion rechecks cancellation after lock", func(t *testing.T) {
		n := newNotificationFixture(t)
		saved := n.reserve(t, true, bookingKey())
		producer, err := n.pool.Acquire(n.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer producer.Release()
		var pid int
		if err := producer.QueryRow(n.ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		blocker, err := n.pool.Begin(n.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback(n.ctx)
		r := bookingpg.NewRepository(db.New(blocker))
		if err := r.LockProfessionalSchedule(n.ctx, n.shop, n.professional); err != nil {
			t.Fatal(err)
		}
		digest, _ := (bookingsecurity.Codes{}).Hash(saved.Code)
		a, found, err := r.FindByCode(n.ctx, n.shop, digest)
		if err != nil || !found {
			t.Fatal("missing appointment", err)
		}
		changed, err := a.Cancel(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := r.SaveCancellation(n.ctx, changed); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, err := producer.Exec(n.ctx, `INSERT INTO notification_jobs(id,shop_id,appointment_id,schedule_generation,kind,due_at,expires_at)
              SELECT gen_random_uuid(),shop_id,appointment_id,schedule_generation,kind,due_at,expires_at FROM notification_jobs WHERE shop_id=$1 AND kind='booking_confirmation'`, n.shop)
			result <- err
		}()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var waiting bool
			if err := n.pool.QueryRow(n.ctx, "SELECT cardinality(pg_blocking_pids($1))>0", pid).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("job insert did not acquire the professional lock")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := blocker.Commit(n.ctx); err != nil {
			t.Fatal(err)
		}
		var pgErr *pgconn.PgError
		if err := <-result; !errors.As(err, &pgErr) || pgErr.ConstraintName != "notification_job_eligible" {
			t.Fatal("stale job eligibility survived cancellation", err)
		}
	})
	t.Run("notification tables enforce RLS and reject a foreign customer", func(t *testing.T) {
		n := newNotificationFixture(t)
		n.configure(false, false, booking.ClockFunc(time.Now))
		saved := n.reserve(t, false, bookingKey())
		foreignShop, foreignCustomer := uuid.New(), uuid.New()
		n.exec(t, "INSERT INTO shops(id,name,slug) VALUES($1,'Other',$2)", foreignShop, "other-"+foreignShop.String())
		n.exec(t, "INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at) VALUES($1,$2,'Test','Customer','+5511912345678',now())", foreignCustomer, foreignShop)
		_, err := n.pool.Exec(n.ctx, `INSERT INTO customer_booking_message_permissions(shop_id,appointment_id,customer_id,allowed,notice_version,source,recorded_at)
            VALUES($1,$2,$3,TRUE,1,'public_booking',clock_timestamp())`, n.shop, saved.Appointment.ID, foreignCustomer)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Fatal("foreign customer bound to notification permission", err)
		}
		var protected bool
		err = n.pool.QueryRow(n.ctx, `SELECT count(*)=4 AND bool_and(c.relrowsecurity AND NOT EXISTS(
              SELECT 1 FROM aclexplode(COALESCE(c.relacl,acldefault('r',c.relowner))) acl
              LEFT JOIN pg_roles r ON r.oid=acl.grantee WHERE acl.grantee=0 OR r.rolname IN('anon','authenticated')))
            FROM pg_class c JOIN pg_namespace ns ON ns.oid=c.relnamespace WHERE ns.nspname='public'
              AND c.relname IN('shop_notification_settings','customer_booking_message_permissions','appointment_notification_plans','notification_jobs')`).Scan(&protected)
		if err != nil || !protected {
			t.Fatal("browser roles can access notification storage", err)
		}
	})
}
