//go:build integration

package integration

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/customers/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	customers "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	customerpg "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/postgres"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/google/uuid"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type contactsFixture struct {
	*bookingFixture
	uc            *app.ManageContacts
	owner, barber uuid.UUID
}

func newContactsFixture(t *testing.T) *contactsFixture {
	t.Helper()
	dsn := requiredEnv(t, "CONTACTS_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_booking_contacts_") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("requires local disposable codex_booking_contacts_* database")
	}
	t.Setenv("BOOKING_TEST_DATABASE_URL", dsn)
	f := &contactsFixture{bookingFixture: newBookingFixture(t), owner: uuid.New(), barber: uuid.New()}
	f.exec(t, "INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber')", f.owner, f.barber)
	f.exec(t, "INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$3,'owner',NULL),($2,$3,'barber',$4)", f.owner, f.barber, f.shop, f.professional)
	f.uc = app.NewManageContacts(customerpg.NewContactTransactor(f.pool, func(q *db.Queries) app.ContactAccess { return identitypg.NewContactAccess(q) }), customers.ContactNormalizer{})
	return f
}
func (f *contactsFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}
func (f *contactsFixture) scope(actor uuid.UUID) app.ContactScope {
	return app.ContactScope{ActorID: actor, ShopID: f.shop}
}
func contactInput() app.CreateContactInput {
	return app.CreateContactInput{FirstName: " João ", Phone: "(11) 91234-5678", PhoneCountry: "BR"}
}

func TestCustomerContacts(t *testing.T) {
	t.Run("shared directory normalizes phones preserves names and creates no messages", func(t *testing.T) {
		f := newContactsFixture(t)
		c, err := f.uc.Create(f.ctx, f.scope(f.barber), contactInput())
		if err != nil || c.FirstName != "João" || c.LastName != "" || c.Phone != "+5511912345678" || c.CreatedAt.IsZero() {
			t.Fatal("contact creation failed", err)
		}
		input := contactInput()
		input.FirstName = "Replacement"
		input.Phone = "+55 11 91234-5678"
		if _, err = f.uc.Create(f.ctx, f.scope(f.owner), input); !errors.Is(err, app.ErrContactExists) {
			t.Fatal("duplicate not reported", err)
		}
		for _, actor := range []uuid.UUID{f.owner, f.barber} {
			saved, err := f.uc.Get(f.ctx, f.scope(actor), c.ID)
			if err != nil || saved.FirstName != "João" {
				t.Fatal("shared contact overwritten or inaccessible", err)
			}
		}
		page, err := f.uc.List(f.ctx, f.scope(f.owner), app.ListContactsInput{Phone: "11912345678", PhoneCountry: "BR"})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != c.ID {
			t.Fatal("normalized lookup failed", err)
		}
		for _, table := range []string{"appointments", "customer_booking_message_permissions", "appointment_notification_plans", "notification_jobs"} {
			var count int
			if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE shop_id=$1", f.shop).Scan(&count); err != nil || count != 0 {
				t.Fatal("manual contact generated business effects", table, err)
			}
		}
	})
	t.Run("concurrent manual inserts produce one row", func(t *testing.T) {
		f := newContactsFixture(t)
		var wg sync.WaitGroup
		results := make(chan error, 8)
		start := make(chan struct{})
		for range 8 {
			wg.Go(func() { <-start; _, err := f.uc.Create(f.ctx, f.scope(f.barber), contactInput()); results <- err })
		}
		close(start)
		wg.Wait()
		close(results)
		success, conflicts := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, app.ErrContactExists) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflicts != 7 {
			t.Fatal("duplicate inserts not serialized", success, conflicts)
		}
	})
	t.Run("booking insert competes with manual creation without overwriting", func(t *testing.T) {
		f := newContactsFixture(t)
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(f.ctx)
		saved, err := customerpg.NewBookingContacts(db.New(tx)).ResolveForBooking(f.ctx, booking.CustomerResolutionInput{ShopID: f.shop, Customer: booking.CustomerInput{FirstName: "Original", LastName: "Booking", Phone: "+5511912345678"}, RecordedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { _, err := f.uc.Create(f.ctx, f.scope(f.owner), contactInput()); result <- err }()
		deadline := time.Now().Add(5 * time.Second)
		waiting := false
		for time.Now().Before(deadline) {
			var n int
			_ = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%INSERT INTO customers%' AND pid<>pg_backend_pid()").Scan(&n)
			if n > 0 {
				waiting = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !waiting {
			t.Fatal("manual insert did not overlap open booking transaction")
		}
		if err := tx.Commit(f.ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-result; !errors.Is(err, app.ErrContactExists) {
			t.Fatal("concurrent contact collision not translated", err)
		}
		c, err := f.uc.Get(f.ctx, f.scope(f.barber), saved.CustomerID)
		if err != nil || c.Name() != "Original Booking" {
			t.Fatal("booking name replaced", err)
		}
		tx2, _ := f.pool.Begin(f.ctx)
		_, err = customerpg.NewBookingContacts(db.New(tx2)).ResolveForBooking(f.ctx, booking.CustomerResolutionInput{ShopID: f.shop, Customer: booking.CustomerInput{FirstName: "Rollback", LastName: "Contact", Phone: "+5511987654321"}, RecordedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		_ = tx2.Rollback(f.ctx)
		page, err := f.uc.List(f.ctx, f.scope(f.owner), app.ListContactsInput{Phone: "+5511987654321"})
		if err != nil || len(page.Items) != 0 {
			t.Fatal("rolled back booking left contact", err)
		}
	})
	t.Run("tenant scope cursor binding ties and literal search", func(t *testing.T) {
		f := newContactsFixture(t)
		other := newContactsFixture(t)
		for _, phone := range []string{"11912345678", "11987654321", "21998765432"} {
			input := contactInput()
			input.Phone = phone
			if _, err := f.uc.Create(f.ctx, f.scope(f.owner), input); err != nil {
				t.Fatal(err)
			}
		}
		f.exec(t, "UPDATE customers SET created_at='2026-11-01T00:00:00Z' WHERE shop_id=$1", f.shop)
		page, err := f.uc.List(f.ctx, f.scope(f.barber), app.ListContactsInput{Limit: 2})
		if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
			t.Fatal("missing first page", err)
		}
		next, err := f.uc.List(f.ctx, f.scope(f.barber), app.ListContactsInput{Limit: 2, Cursor: page.NextCursor})
		if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.Items[0].ID == page.Items[1].ID || next.NextCursor != "" {
			t.Fatal("unstable timestamp-tie pagination", err)
		}
		if _, err = f.uc.List(f.ctx, other.scope(other.owner), app.ListContactsInput{Cursor: page.NextCursor}); !errors.Is(err, app.ErrInvalidContactQuery) {
			t.Fatal("foreign cursor accepted", err)
		}
		if _, err = f.uc.List(f.ctx, f.scope(f.owner), app.ListContactsInput{Name: "Another", Cursor: page.NextCursor}); !errors.Is(err, app.ErrInvalidContactQuery) {
			t.Fatal("cursor reused with another filter", err)
		}
		if _, err = other.uc.Get(other.ctx, other.scope(other.owner), page.Items[0].ID); !errors.Is(err, app.ErrContactNotFound) {
			t.Fatal("foreign contact exposed", err)
		}
		if _, err = other.uc.Create(other.ctx, other.scope(other.owner), contactInput()); err != nil {
			t.Fatal("phone shared across shops rejected", err)
		}
		input := contactInput()
		input.FirstName = "Literal%_Name"
		input.Phone = "31998765432"
		if _, err = f.uc.Create(f.ctx, f.scope(f.owner), input); err != nil {
			t.Fatal(err)
		}
		found, err := f.uc.List(f.ctx, f.scope(f.owner), app.ListContactsInput{Name: "%_"})
		if err != nil || len(found.Items) != 1 || found.Items[0].FirstName != "Literal%_Name" {
			t.Fatal("search wildcard was not escaped", err)
		}
	})
	t.Run("revocation and invalid data", func(t *testing.T) {
		f := newContactsFixture(t)
		for _, sql := range []string{"UPDATE shop_memberships SET active=FALSE WHERE shop_id=$1", "UPDATE users SET active=FALSE WHERE id IN(SELECT user_id FROM shop_memberships WHERE shop_id=$1)", "UPDATE shops SET active=FALSE WHERE id=$1"} {
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(f.ctx, sql, f.shop)
			if err != nil {
				t.Fatal(err)
			}
			_ = tx.Commit(f.ctx)
			if _, err := f.uc.List(f.ctx, f.scope(f.owner), app.ListContactsInput{}); !errors.Is(err, app.ErrContactAccessDenied) {
				t.Fatal("revoked access accepted", err)
			}
			f.exec(t, "UPDATE users SET active=TRUE WHERE id IN(SELECT user_id FROM shop_memberships WHERE shop_id=$1)", f.shop)
			f.exec(t, "UPDATE shop_memberships SET active=TRUE WHERE shop_id=$1", f.shop)
			f.exec(t, "UPDATE shops SET active=TRUE WHERE id=$1", f.shop)
		}
		in := contactInput()
		in.FirstName = ""
		if _, err := f.uc.Create(f.ctx, f.scope(f.owner), in); !errors.Is(err, domain.ErrInvalidContact) {
			t.Fatal("invalid name accepted", err)
		}
		in = contactInput()
		in.Phone = "123"
		if _, err := f.uc.Create(f.ctx, f.scope(f.barber), in); !errors.Is(err, domain.ErrInvalidContact) {
			t.Fatal("invalid phone accepted", err)
		}
		if _, err := f.uc.List(f.ctx, f.scope(uuid.New()), app.ListContactsInput{}); !errors.Is(err, app.ErrContactAccessDenied) {
			t.Fatal("unprovisioned actor accepted", err)
		}
		var rls bool
		if err := f.pool.QueryRow(f.ctx, "SELECT relrowsecurity FROM pg_class WHERE oid='public.customers'::regclass").Scan(&rls); err != nil || !rls {
			t.Fatal("RLS lost", err)
		}
	})
	t.Run("manual existing contact is reused in a real booking and replay", func(t *testing.T) {
		f := newContactsFixture(t)
		c, err := f.uc.Create(f.ctx, f.scope(f.owner), contactInput())
		if err != nil {
			t.Fatal(err)
		}
		key := bookingKey()
		saved := bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.service), key), 201)
		replay := bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.service), key), 200)
		if saved.Appointment.ID != replay.Appointment.ID {
			t.Fatal("booking replay duplicated")
		}
		var id uuid.UUID
		if err := f.pool.QueryRow(f.ctx, "SELECT customer_id FROM appointments WHERE id=$1 AND shop_id=$2", saved.Appointment.ID, f.shop).Scan(&id); err != nil || id != c.ID {
			t.Fatal("booking did not reuse manual contact", err)
		}
		current, err := f.uc.Get(f.ctx, f.scope(f.owner), c.ID)
		if err != nil || current.FirstName != "João" || current.LastName != "" {
			t.Fatal("public booking overwrote registered name", err)
		}
		detail, err := db.New(f.pool).GetAdminAppointmentDetail(context.Background(), db.GetAdminAppointmentDetailParams{ShopID: f.shop, ID: saved.Appointment.ID})
		if err != nil || detail.CustomerName != "João" {
			t.Fatal("optional surname leaked trailing space", err)
		}
	})
}
