//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	billingapp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestShopPublication(t *testing.T) {
	dsn := requiredEnv(t, "PUBLICATION_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_publication_") {
		t.Fatal("publication tests require a disposable codex_publication_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to publication database")
	}
	defer pool.Close()
	var auth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&auth); err != nil || auth {
		t.Fatal("publication tests refuse Auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider, shop, legacy, other, prof, service := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	slug := "publication-" + shop.String()
	legacySlug := "legacy-" + legacy.String()
	otherSlug := "other-" + other.String()
	exec("INSERT INTO users(id,display_name) VALUES ($1,'Owner'),($2,'Barber'),($3,'Outsider')", owner, barber, outsider)
	exec("INSERT INTO shops(id,name,slug,timezone,provisioning_source) VALUES ($1,'Shop',$2,'America/Sao_Paulo','self_service'),($3,'Legacy',$4,'America/Sao_Paulo','manual'),($5,'Other',$6,'America/Sao_Paulo','self_service')", shop, slug, legacy, legacySlug, other, otherSlug)
	exec("INSERT INTO professionals(id,shop_id,name) VALUES ($1,$2,'Professional')", prof, shop)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES ($1,$2,'owner',NULL),($1,$3,'owner',NULL),($4,$2,'barber',$5),($6,$7,'owner',NULL)", owner, shop, legacy, barber, prof, outsider, other)
	bind := func(q *db.Queries) shops.PublicationTx {
		return shops.PublicationTx{Access: identitypg.NewSettingsOwnerGuard(q), Shops: shopspg.NewPublicationStore(q), Catalog: catalogpg.NewRepository(q), Schedule: bookingpg.NewRepository(q), Billing: billingpg.NewPublicationCoverage(billingpg.NewReader(q), time.Now)}
	}
	transactions := shopspg.NewPublicationTransactor(pool, bind)
	uc := shops.NewShopPublication(transactions)
	scope := shops.SettingsScope{ActorID: owner, ShopID: shop}
	q := db.New(pool)
	repo := identitypg.NewRepository(q)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Public gate below uses real local billing, without any Stripe calls.
	gate := shops.NewCheckPublicBooking(shopspg.NewResolver(q), billingapp.NewGetShopEntitlements(billingpg.NewReader(q), billingapp.ClockFunc(time.Now)))
	handler := httpapi.NewHandler(httpapi.HandlerConfig{ShopPublication: shopshttp.NewPublicationHandler(uc, 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
	request := func(method, target, route, body string, actor uuid.UUID) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/admin/shops/"+target+route, strings.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		if actor != uuid.Nil {
			r.Header.Set("Authorization", "Bearer "+actor.String())
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	get := func() shops.ShopCommercialStatus {
		t.Helper()
		w := request("GET", slug, "/commercial-status", "", owner)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out shops.ShopCommercialStatus
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	t.Run("owner only and strict HTTP boundary", func(t *testing.T) {
		for _, actor := range []uuid.UUID{uuid.Nil, barber, outsider} {
			want := 403
			if actor == uuid.Nil {
				want = 401
			}
			for _, route := range []string{"/commercial-status", "/publication"} {
				method := "GET"
				body := ""
				if route == "/publication" {
					method = "PUT"
					body = `{"requested":true}`
				}
				w := request(method, slug, route, body, actor)
				if w.Code != want {
					t.Fatal(w.Code, w.Body.String())
				}
			}
		}
		if w := request("PUT", otherSlug, "/publication", `{"requested":true}`, owner); w.Code != 403 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, body := range []string{`{}`, `null`, `{"requested":null}`, `{"requested":"true"}`, `{"requested":true,"shop_id":"` + other.String() + `"}`, `{"requested":false} {}`} {
			if w := request("PUT", slug, "/publication", body, owner); w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		if w := request("PUT", slug, "/publication", `{"requested":true,"padding":"`+strings.Repeat("x", 1200)+`"}`, owner); w.Code != 400 && w.Code != 413 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("draft cannot publish without coverage and configuration", func(t *testing.T) {
		out := get()
		if out.CanPublish || out.PublicBookingEnabled || out.Subscription.Reason != "subscription_required" || len(out.Blockers) != 5 {
			t.Fatal(out)
		}
		if w := request("PUT", slug, "/publication", `{"requested":true}`, owner); w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := gate.Execute(ctx, slug); err == nil {
			t.Fatal("draft is public")
		}
	})
	// Persist the same reconciled financial shape used by the webhook. No external
	// payment is simulated as a production event; this is a disposable fixture.
	customer, subscription, invoice := "cus_"+shop.String(), "sub_"+shop.String(), "in_"+strings.ReplaceAll(shop.String(), "-", "")
	attempt := uuid.New()
	now := time.Now().UTC()
	exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id,stripe_mode) VALUES ($1,$2,'test')", shop, customer)
	exec("INSERT INTO billing_checkout_attempts(id,shop_id,intent,plan_key,state,protocol_version,stripe_price_id,return_url,stripe_session_id,expires_at) VALUES ($1,$2,'publish','standard_monthly','completed',1,'price_Test','http://127.0.0.1:3000/admin/shop/configuracoes',$3,$4)", attempt, shop, "cs_test_"+attempt.String(), now.Add(time.Hour))
	exec("INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until,reconciliation_protocol,checkout_attempt_id,stripe_price_id) VALUES ($1,$2,$3,$4,'standard_monthly','active',$5,$6,1,$7,'price_Test')", uuid.New(), shop, customer, subscription, now.Add(-time.Hour), now.Add(time.Hour), attempt)
	t.Run("active contract without paid ledger does not grant publication", func(t *testing.T) {
		if out := get(); out.Subscription.Allowed || out.CanPublish {
			t.Fatal(out)
		}
	})
	exec("INSERT INTO billing_paid_invoices(stripe_invoice_id,shop_id,stripe_subscription_id,stripe_price_id,paid_from,paid_until,amount_paid) VALUES ($1,$2,$3,'price_Test',$4,$5,9900)", invoice, shop, subscription, now.Add(-time.Hour), now.Add(time.Hour))
	t.Run("payment alone never publishes", func(t *testing.T) {
		out := get()
		if !out.Subscription.Allowed || out.Requested || out.PublicBookingEnabled || out.CanPublish {
			t.Fatal(out)
		}
		if err := gate.Execute(ctx, slug); err == nil {
			t.Fatal("payment published draft")
		}
	})
	exec("INSERT INTO shop_addresses(shop_id,postal_code,street,number,district,city,state) VALUES ($1,'01310100','Avenida Paulista','100','Bela Vista','São Paulo','SP')", shop)
	exec("INSERT INTO services(id,shop_id,name,duration_minutes,price_cents) VALUES ($1,$2,'Haircut',80,8000)", service, shop)
	exec("INSERT INTO professional_services(shop_id,professional_id,service_id) VALUES ($1,$2,$3)", shop, prof, service)
	exec("INSERT INTO schedule_settings(shop_id,professional_id,interval_minutes,buffer_minutes) VALUES ($1,$2,60,0)", shop, prof)
	exec("INSERT INTO working_hours(id,shop_id,professional_id,weekday,opens_at_minute,closes_at_minute) VALUES ($1,$2,$3,1,570,630)", uuid.New(), shop, prof)
	t.Run("schedule must fit rounded service occupancy", func(t *testing.T) {
		out := get()
		if !out.Readiness.Address || !out.Readiness.Service || !out.Readiness.Professional || out.Readiness.Schedule || out.CanPublish {
			t.Fatal(out)
		}
		if w := request("PUT", slug, "/publication", `{"requested":true}`, owner); w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	exec("UPDATE working_hours SET closes_at_minute=690 WHERE shop_id=$1", shop)
	t.Run("publication explicit repeatable and independent from settings", func(t *testing.T) {
		out := get()
		if !out.CanPublish || out.PublicBookingEnabled {
			t.Fatal(out)
		}
		for range 2 {
			if w := request("PUT", slug, "/publication", `{"requested":true}`, owner); w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		out = get()
		if !out.PublicBookingEnabled || !out.Requested {
			t.Fatal(out)
		}
		if err := gate.Execute(ctx, slug); err != nil {
			t.Fatal(err)
		}
		var version int64
		if err := pool.QueryRow(ctx, "SELECT settings_version FROM shops WHERE id=$1", shop).Scan(&version); err != nil || version != 1 {
			t.Fatal("publication modified settings version", version, err)
		}
	})
	t.Run("ineligible invoice immediately hides without removing intent", func(t *testing.T) {
		exec("UPDATE billing_paid_invoices SET eligible=false WHERE stripe_invoice_id=$1", invoice)
		out := get()
		if out.PublicBookingEnabled || out.Subscription.Allowed || !out.Requested {
			t.Fatal(out)
		}
		if err := gate.Execute(ctx, slug); err == nil {
			t.Fatal("lost coverage remained public")
		}
		if w := request("PUT", slug, "/publication", `{"requested":false}`, owner); w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
		exec("UPDATE billing_paid_invoices SET eligible=true WHERE stripe_invoice_id=$1", invoice)
		if out := get(); out.Requested || out.PublicBookingEnabled {
			t.Fatal("renewal republished explicitly hidden shop", out)
		}
	})
	t.Run("legacy public behavior is not silently enrolled", func(t *testing.T) {
		out, err := uc.Get(ctx, shops.SettingsScope{ActorID: owner, ShopID: legacy})
		if err != nil || out.PublicationManaged || !out.PublicBookingEnabled || out.Subscription.Status != "not_required" {
			t.Fatal(out, err)
		}
		if w := request("PUT", legacySlug, "/publication", `{"requested":false}`, owner); w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := gate.Execute(ctx, legacySlug); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("revoked owner cannot call use case directly", func(t *testing.T) {
		exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", owner, shop)
		if err := uc.Set(ctx, scope, true); err == nil {
			t.Fatal("revoked owner published")
		}
		exec("UPDATE shop_memberships SET active=true WHERE user_id=$1 AND shop_id=$2", owner, shop)
	})
	t.Run("deferred commit failure cannot report success or publish", func(t *testing.T) {
		exec(`CREATE FUNCTION codex_publication_reject_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected commit failure'; END $$`)
		exec(`CREATE CONSTRAINT TRIGGER codex_publication_commit AFTER UPDATE ON shops DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION codex_publication_reject_commit()`)
		w := request("PUT", slug, "/publication", `{"requested":true}`, owner)
		exec("DROP TRIGGER codex_publication_commit ON shops")
		exec("DROP FUNCTION codex_publication_reject_commit()")
		if w.Code != 503 || get().Requested {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("simultaneous publication never toggles or loses intent", func(t *testing.T) {
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { <-start; results <- uc.Set(ctx, scope, true) })
		}
		close(start)
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			}
		}
		if success < 1 || !get().Requested {
			t.Fatal("no committed publication", success)
		}
	})
	t.Run("financial account lock and revalidation prevent stale payment publication", func(t *testing.T) {
		if err := uc.Set(ctx, scope, false); err != nil {
			t.Fatal(err)
		}
		financial, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer financial.Rollback(context.Background())
		if _, err := financial.Exec(ctx, "SELECT shop_id FROM billing_accounts WHERE shop_id=$1 FOR UPDATE", shop); err != nil {
			t.Fatal(err)
		}
		if _, err := financial.Exec(ctx, "UPDATE billing_paid_invoices SET eligible=false WHERE stripe_invoice_id=$1", invoice); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- uc.Set(ctx, scope, true) }()
		// Wait for PostgreSQL itself to report the publication transaction waiting,
		// not an arbitrary sleep used as proof of the race.
		deadline := time.Now().Add(5 * time.Second)
		waiting := false
		for time.Now().Before(deadline) {
			if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%LockBillingOperationAccount%')").Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !waiting {
			t.Fatal("publication did not wait on financial account")
		}
		// The real webhook also advances revision under the account lock.
		if _, err := financial.Exec(ctx, "UPDATE billing_accounts SET reconciliation_revision=reconciliation_revision+1 WHERE shop_id=$1", shop); err != nil {
			t.Fatal(err)
		}
		if err := financial.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err == nil {
			t.Fatal("publication used stale paid snapshot")
		}
		if get().Requested {
			t.Fatal("unpaid publication persisted")
		}
	})
}
