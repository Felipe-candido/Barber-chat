//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	billing "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	billinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/http"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	customers "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
)

// JWT verification alone is replaced. Guards, billing ledger, staff permissions,
// receipt encryption, SQL locks, contact writes and overlap constraints are real.
func TestStaffBookingSubscription(t *testing.T) {
	f, other := newBookingFixture(t), newBookingFixture(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users(id,display_name) VALUES ($1,'Owner'),($2,'Barber'),($3,'Outsider')", owner, barber, outsider)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES ($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$6,'owner',NULL)", owner, f.shop, barber, f.professional, outsider, other.shop)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := booking.ClockFunc(time.Now)
	q := db.New(f.pool)
	resolver, identities := shopspg.NewResolver(q), identitypg.NewRepository(q)
	makeHandler := func(bind func(*db.Queries) booking.BookingTx) http.Handler {
		tx := bookingpg.NewTransactor(f.pool, bind)
		codes := security.Codes{}
		create := booking.NewCreateAppointment(tx, customers.ContactNormalizer{}, codes, clock)
		settingsTx := shopspg.NewSettingsTransactor(f.pool, func(q *db.Queries) shops.ShopSettingsTx {
			return shops.ShopSettingsTx{Access: identitypg.NewSettingsOwnerGuard(q), Shops: shopspg.NewSettingsStore(q)}
		})
		return httpapi.NewHandler(httpapi.HandlerConfig{
			Booking:      bookinghttp.NewHandler(create, booking.NewListAvailability(tx, clock), booking.NewListProfessionals(tx, clock), booking.NewManageAppointment(tx, codes, clock), 10*time.Second, logger),
			Agenda:       bookinghttp.NewAgendaHandler(booking.NewListAgenda(resolver, bookingpg.NewRepository(q), clock), 10*time.Second, logger),
			ShopSettings: shopshttp.NewSettingsHandler(shops.NewGetShopSettings(settingsTx), shops.NewUpdateShopSettings(settingsTx), 10*time.Second, logger),
			Checkout:     billinghttp.NewCheckoutHandler(billing.NewStartCheckout(nil, nil, billing.ClockFunc(time.Now), billing.CheckoutPlan{}), 10*time.Second, logger),
			Authenticate: identity.NewAuthenticate(settingsVerifier{}, identities), AuthorizeShop: identity.NewAuthorizeShopAction(identities, resolver), Logger: logger,
		})
	}
	handler := makeHandler(bindBookingQueries)
	request := func(h http.Handler, actor uuid.UUID, slug, method, path string, body any, key string) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "/api/v1/admin/shops/"+slug+path, bytes.NewReader(raw)).WithContext(f.ctx)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		if actor != uuid.Nil {
			r.Header.Set("Authorization", "Bearer "+actor.String())
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	post := func(actor uuid.UUID, body any, key string) *httptest.ResponseRecorder {
		return request(handler, actor, f.slug, "POST", "/appointments", body, key)
	}
	counts := func() [3]int64 {
		t.Helper()
		var result [3]int64
		if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM appointments WHERE shop_id=$1),
			(SELECT count(*) FROM customers WHERE shop_id=$1),(SELECT count(*) FROM booking_requests WHERE shop_id=$1)`, f.shop).Scan(&result[0], &result[1], &result[2]); err != nil {
			t.Fatal(err)
		}
		return result
	}
	t.Run("legacy manual reservations do not require billing", func(t *testing.T) {
		bookingResult(t, post(owner, f.body(f.start, f.service), bookingKey()), 201)
	})
	exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", f.shop)
	t.Run("private booking observations work unpaid and unpublished without public access", func(t *testing.T) {
		before := counts()
		secondProfessional := uuid.New()
		exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Second professional')", secondProfessional, f.shop)
		exec("INSERT INTO professional_services(shop_id,professional_id,service_id) VALUES($1,$2,$3)", f.shop, secondProfessional, f.service)
		professionalsPath := "/booking/professionals?service_id=" + f.service.String()
		for _, actor := range []uuid.UUID{owner, barber} {
			w := request(handler, actor, f.slug, "GET", professionalsPath, nil, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var body struct {
				Professionals []struct {
					ID uuid.UUID `json:"id"`
				} `json:"professionals"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if actor == barber && (len(body.Professionals) != 1 || body.Professionals[0].ID != f.professional) {
				t.Fatal("barber read other professionals", body)
			}
			if actor == owner && len(body.Professionals) != 2 {
				t.Fatal("owner cannot observe own catalog", body)
			}
		}
		location, _ := time.LoadLocation("America/Sao_Paulo")
		availabilityPath := "/booking/availability?service_id=" + f.service.String() + "&professional_id=" + f.professional.String() + "&date=" + f.start.In(location).Format("2006-01-02")
		for _, actor := range []uuid.UUID{owner, barber} {
			if w := request(handler, actor, f.slug, "GET", availabilityPath, nil, ""); w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		for _, test := range []struct {
			actor  uuid.UUID
			status int
		}{{uuid.Nil, 401}, {outsider, 403}} {
			for _, path := range []string{professionalsPath, availabilityPath} {
				if w := request(handler, test.actor, f.slug, "GET", path, nil, ""); w.Code != test.status {
					t.Fatal("unauthorized observation", w.Code, w.Body.String())
				}
			}
		}
		if w := request(handler, barber, f.slug, "GET", strings.ReplaceAll(availabilityPath, f.professional.String(), secondProfessional.String()), nil, ""); w.Code != 403 {
			t.Fatal("barber observed another calendar", w.Code, w.Body.String())
		}
		if w := request(handler, owner, other.slug, "GET", professionalsPath, nil, ""); w.Code != 403 {
			t.Fatal("other tenant observed", w.Code)
		}
		withoutGuard := makeHandler(func(q *db.Queries) booking.BookingTx { tx := bindBookingQueries(q); tx.Staff = nil; return tx })
		if w := request(withoutGuard, owner, f.slug, "GET", professionalsPath, nil, ""); w.Code != 403 {
			t.Fatal("missing transaction guard allowed read", w.Code)
		}
		if w := f.request("GET", "/professionals?service_id="+f.service.String(), nil, ""); w.Code != 404 {
			t.Fatal("public reads were released", w.Code)
		}
		if after := counts(); before != after {
			t.Fatal("observation persisted reservation data", before, after)
		}
	})
	t.Run("unpaid requests fail without any partial writes", func(t *testing.T) {
		before := counts()
		for _, test := range []struct {
			actor   uuid.UUID
			status  int
			message string
		}{{owner, 402, "manage billing"}, {barber, 403, "contact the shop owner"}} {
			w := post(test.actor, f.body(f.start.Add(time.Hour), f.service), bookingKey())
			if w.Code != test.status || !strings.Contains(w.Body.String(), "shop_subscription_required") || !strings.Contains(w.Body.String(), test.message) {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		if after := counts(); after != before {
			t.Fatal("blocked request persisted data", before, after)
		}
		if w := request(handler, barber, f.slug, "POST", "/billing/checkout", map[string]string{"intent": "manual_booking"}, ""); w.Code != 403 {
			t.Fatal("barber obtained billing access", w.Code, w.Body.String())
		}
	})
	t.Run("authorization precedes commercial response", func(t *testing.T) {
		for _, test := range []struct {
			actor  uuid.UUID
			status int
		}{{uuid.Nil, 401}, {outsider, 403}} {
			w := post(test.actor, f.body(f.start.Add(time.Hour), f.service), bookingKey())
			if w.Code != test.status || strings.Contains(w.Body.String(), "shop_subscription_required") {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		w := request(handler, owner, other.slug, "POST", "/appointments", other.body(other.start, other.service), bookingKey())
		if w.Code != 403 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	now := time.Now().UTC()
	customer, subscription := "cus_"+f.shop.String(), "sub_"+f.shop.String()
	attempt := uuid.New()
	invoice := "in_" + strings.ReplaceAll(f.shop.String(), "-", "")
	exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id,stripe_mode) VALUES ($1,$2,'test')", f.shop, customer)
	exec("INSERT INTO billing_checkout_attempts(id,shop_id,intent,plan_key,state,protocol_version,stripe_price_id,return_url) VALUES ($1,$2,'manual_booking','standard_monthly','review',1,'price_Test','http://127.0.0.1:3000/admin/shop/configuracoes')", attempt, f.shop)
	exec("INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until,reconciliation_protocol,checkout_attempt_id,stripe_price_id) VALUES ($1,$2,$3,$4,'standard_monthly','active',$5,$6,1,$7,'price_Test')", uuid.New(), f.shop, customer, subscription, now.Add(-time.Hour), now.Add(time.Hour), attempt)
	t.Run("active without eligible paid invoice is blocked", func(t *testing.T) {
		if w := post(owner, f.body(f.start.Add(time.Hour), f.service), bookingKey()); w.Code != 402 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	exec("INSERT INTO billing_paid_invoices(stripe_invoice_id,shop_id,stripe_subscription_id,stripe_price_id,paid_from,paid_until,amount_paid) VALUES ($1,$2,$3,'price_Test',$4,$5,9900)", invoice, f.shop, subscription, now.Add(-time.Hour), now.Add(time.Hour))
	key := bookingKey()
	body := f.body(f.start.Add(time.Hour), f.service)
	var created bookingHTTPResult
	t.Run("paid draft permits manual creation but remains private", func(t *testing.T) {
		created = bookingResult(t, post(barber, body, key), 201)
		var published bool
		if err := f.pool.QueryRow(f.ctx, "SELECT publication_requested FROM shops WHERE id=$1", f.shop).Scan(&published); err != nil || published {
			t.Fatal("manual booking published shop", err)
		}
		if w := f.request("POST", "/appointments", f.body(f.start.Add(2*time.Hour), f.service), bookingKey()); w.Code != 404 {
			t.Fatal("private draft became public", w.Code, w.Body.String())
		}
	})
	t.Run("barber cannot change professional even while paid", func(t *testing.T) {
		body := f.body(f.start.Add(2*time.Hour), f.service)
		body["professional_id"] = uuid.New()
		if w := post(barber, body, bookingKey()); w.Code != 403 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("loss of coverage denies new reservations but preserves replay and management", func(t *testing.T) {
		exec("UPDATE billing_subscriptions SET status='suspended' WHERE shop_id=$1", f.shop)
		before := counts()
		if w := post(owner, f.body(f.start.Add(2*time.Hour), f.service), bookingKey()); w.Code != 402 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, actor := range []uuid.UUID{owner, barber} {
			replay := bookingResult(t, post(actor, body, key), 200)
			if !replay.Replayed || replay.Code != created.Code || replay.Appointment.ID != created.Appointment.ID {
				t.Fatal("receipt changed", replay)
			}
		}
		if after := counts(); after != before {
			t.Fatal("replay inserted data", before, after)
		}
		conflict := post(owner, f.body(f.start.Add(3*time.Hour), f.service), key)
		if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), "idempotency_conflict") {
			t.Fatal(conflict.Code, conflict.Body.String())
		}
		for _, actor := range []uuid.UUID{owner, barber} {
			w := request(handler, actor, f.slug, "GET", "/appointments?month="+f.date[:7], nil, "")
			if w.Code != 200 || !strings.Contains(w.Body.String(), created.Appointment.ID.String()) {
				t.Fatal("agenda blocked", w.Code, w.Body.String())
			}
		}
		if w := request(handler, owner, f.slug, "GET", "/settings", nil, ""); w.Code != 200 {
			t.Fatal("settings blocked", w.Code, w.Body.String())
		}
		for _, path := range []string{"/appointments/lookup", "/appointments/cancel"} {
			bookingResult(t, f.request("POST", path, map[string]string{"code": created.Code}, ""), 200)
		}
	})
	t.Run("revoked membership cannot replay saved reservation", func(t *testing.T) {
		exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", barber, f.shop)
		if w := post(barber, body, key); w.Code != 403 || strings.Contains(w.Body.String(), created.Code) {
			t.Fatal(w.Code, w.Body.String())
		}
		exec("UPDATE shop_memberships SET active=true WHERE user_id=$1 AND shop_id=$2", barber, f.shop)
	})
	t.Run("expired invalidated and future paid intervals do not authorize now", func(t *testing.T) {
		exec("UPDATE billing_subscriptions SET status='active' WHERE shop_id=$1", f.shop)
		for _, period := range [][2]time.Time{{now.Add(-2 * time.Hour), now.Add(-time.Hour)}, {now.Add(time.Hour), now.Add(2 * time.Hour)}} {
			exec("UPDATE billing_paid_invoices SET paid_from=$1,paid_until=$2 WHERE stripe_invoice_id=$3", period[0], period[1], invoice)
			if w := post(owner, f.body(f.start.Add(2*time.Hour), f.service), bookingKey()); w.Code != 402 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		exec("UPDATE billing_paid_invoices SET paid_from=$1,paid_until=$2,eligible=false WHERE stripe_invoice_id=$3", now.Add(-time.Hour), now.Add(time.Hour), invoice)
		if w := post(owner, f.body(f.start.Add(2*time.Hour), f.service), bookingKey()); w.Code != 402 {
			t.Fatal(w.Code, w.Body.String())
		}
		exec("UPDATE billing_paid_invoices SET eligible=true WHERE stripe_invoice_id=$1", invoice)
	})
	t.Run("pending effective cancellation and suspension deny new bookings", func(t *testing.T) {
		for _, status := range []string{"pending", "canceled", "suspended"} {
			if status == "pending" {
				exec("UPDATE billing_subscriptions SET status='pending',paid_from=NULL,paid_until=NULL,cancel_at_period_end=false WHERE shop_id=$1", f.shop)
			} else {
				exec("UPDATE billing_subscriptions SET status=$1 WHERE shop_id=$2", status, f.shop)
			}
			if w := post(owner, f.body(f.start.Add(2*time.Hour), f.service), bookingKey()); w.Code != 402 {
				t.Fatal(status, w.Code, w.Body.String())
			}
		}
		exec("UPDATE billing_subscriptions SET status='active',paid_from=$1,paid_until=$2,cancel_at_period_end=true WHERE shop_id=$3", now.Add(-time.Hour), now.Add(time.Hour), f.shop)
	})
	t.Run("scheduled cancellation retains paid access and overlap protection", func(t *testing.T) {
		start := make(chan struct{})
		results := make(chan *httptest.ResponseRecorder, 2)
		var wg sync.WaitGroup
		for _, actor := range []uuid.UUID{owner, barber} {
			wg.Go(func() { <-start; results <- post(actor, f.body(f.start.Add(2*time.Hour), f.service), bookingKey()) })
		}
		close(start)
		wg.Wait()
		close(results)
		codes := map[int]int{}
		for result := range results {
			codes[result.Code]++
		}
		if codes[201] != 1 || codes[409] != 1 {
			t.Fatal(codes)
		}
	})
	t.Run("missing eligibility adapter is unavailable not unpaid and persists nothing", func(t *testing.T) {
		h := makeHandler(func(q *db.Queries) booking.BookingTx {
			tx := bindBookingQueries(q)
			tx.StaffEligibility = nil
			return tx
		})
		before := counts()
		w := request(h, owner, f.slug, "POST", "/appointments", f.body(f.start.Add(3*time.Hour), f.service), bookingKey())
		if w.Code != 503 || strings.Contains(w.Body.String(), "shop_subscription_required") || counts() != before {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("commit failure rolls back reservation contact and recovery receipt", func(t *testing.T) {
		exec(`CREATE FUNCTION codex_manual_booking_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected booking commit failure'; END $$`)
		exec(`CREATE CONSTRAINT TRIGGER codex_manual_booking_commit AFTER INSERT ON appointments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION codex_manual_booking_commit_failure()`)
		before := counts()
		h := makeHandler(bindBookingQueries)
		w := request(h, owner, f.slug, "POST", "/appointments", f.body(f.start.Add(7*time.Hour), f.service), bookingKey())
		exec("DROP TRIGGER codex_manual_booking_commit ON appointments")
		exec("DROP FUNCTION codex_manual_booking_commit_failure()")
		if w.Code < 500 || counts() != before {
			t.Fatal("commit failure persisted a partial reservation", w.Code, w.Body.String())
		}
	})
	t.Run("financial update committed first is seen after account lock", func(t *testing.T) {
		financial, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer financial.Rollback(context.Background())
		if _, err := financial.Exec(f.ctx, "UPDATE billing_accounts SET reconciliation_revision=reconciliation_revision+1 WHERE shop_id=$1", f.shop); err != nil {
			t.Fatal(err)
		}
		if _, err := financial.Exec(f.ctx, "UPDATE billing_subscriptions SET status='suspended' WHERE shop_id=$1", f.shop); err != nil {
			t.Fatal(err)
		}
		before := counts()
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- post(owner, f.body(f.start.Add(3*time.Hour), f.service), bookingKey()) }()
		waitManualSubscriptionLock(t, f, "%LockBillingOperationAccount%")
		if err := financial.Commit(f.ctx); err != nil {
			t.Fatal(err)
		}
		w := <-done
		if w.Code != 402 || counts() != before {
			t.Fatal("stale financial read created reservation", w.Code, w.Body.String())
		}
	})
	t.Run("booking decision holds account through its commit", func(t *testing.T) {
		exec("UPDATE billing_subscriptions SET status='active' WHERE shop_id=$1", f.shop)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		defer unblock()
		h := makeHandler(func(q *db.Queries) booking.BookingTx {
			tx := bindBookingQueries(q)
			tx.Customers = subscriptionCustomerBarrier{delegate: tx.Customers, entered: entered, release: release}
			return tx
		})
		booked := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			booked <- request(h, owner, f.slug, "POST", "/appointments", f.body(f.start.Add(3*time.Hour), f.service), bookingKey())
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("booking did not reach guarded write")
		}
		updated := make(chan error, 1)
		go func() {
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				updated <- err
				return
			}
			defer tx.Rollback(context.Background())
			if _, err = tx.Exec(f.ctx, "/* manual_subscription_suspend */ UPDATE billing_accounts SET reconciliation_revision=reconciliation_revision+1 WHERE shop_id=$1", f.shop); err == nil {
				_, err = tx.Exec(f.ctx, "UPDATE billing_subscriptions SET status='suspended' WHERE shop_id=$1", f.shop)
			}
			if err == nil {
				err = tx.Commit(f.ctx)
			}
			updated <- err
		}()
		waitManualSubscriptionLock(t, f, "/* manual_subscription_suspend */%")
		unblock()
		w := <-booked
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := <-updated; err != nil {
			t.Fatal(err)
		}
		if w := post(owner, f.body(f.start.Add(4*time.Hour), f.service), bookingKey()); w.Code != 402 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
}

type subscriptionCustomerBarrier struct {
	delegate booking.CustomerResolver
	entered  chan<- struct{}
	release  <-chan struct{}
}

func (b subscriptionCustomerBarrier) ResolveForBooking(ctx context.Context, input booking.CustomerResolutionInput) (booking.CustomerResolution, error) {
	close(b.entered)
	select {
	case <-b.release:
		return b.delegate.ResolveForBooking(ctx, input)
	case <-ctx.Done():
		return booking.CustomerResolution{}, ctx.Err()
	}
}

func waitManualSubscriptionLock(t *testing.T, f *bookingFixture, pattern string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := f.pool.QueryRow(f.ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)", pattern).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("PostgreSQL did not observe expected financial lock wait")
}
