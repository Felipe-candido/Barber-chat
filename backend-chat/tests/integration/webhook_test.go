//go:build integration

package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	billing "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	billinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/http"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type webhookProviderFake struct {
	mu        sync.Mutex
	reference billing.BillingReference
	snapshot  billing.SubscriptionSnapshot
	fail      bool
	calls     int
}

func (p *webhookProviderFake) ResolveBillingReference(context.Context, billing.VerifiedWebhook) (billing.BillingReference, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail {
		return billing.BillingReference{}, errors.New("provider unavailable")
	}
	return p.reference, nil
}
func (p *webhookProviderFake) ReadSubscriptionSnapshot(context.Context, billing.ReconciliationTarget) (billing.SubscriptionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail {
		return billing.SubscriptionSnapshot{}, errors.New("provider unavailable")
	}
	return p.snapshot, nil
}

func signedWebhookBody(body []byte) string {
	seconds := fmt.Sprint(time.Now().Unix())
	mac := hmac.New(sha256.New, []byte("whsec_integration"))
	_, _ = mac.Write([]byte(seconds + "."))
	_, _ = mac.Write(body)
	return "t=" + seconds + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestStripeWebhook(t *testing.T) {
	dsn := requiredEnv(t, "WEBHOOK_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "codex_webhook_") {
		t.Fatal("webhook tests require disposable codex_webhook_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to disposable webhook database")
	}
	defer pool.Close()
	var auth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&auth); err != nil || auth {
		t.Fatal("webhook tests refuse Auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	store := billingpg.NewWebhookStore(pool)
	clock := billing.ClockFunc(time.Now)
	verifier, _ := stripe.NewWebhookVerifier("whsec_integration", clock)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fixture := func() (*webhookProviderFake, billing.CheckoutAttempt) {
		t.Helper()
		shop, id := uuid.New(), uuid.New()
		suffix := strings.ReplaceAll(shop.String(), "-", "")
		a := billing.CheckoutAttempt{ID: id, ShopID: shop, Intent: domain.CheckoutPublish, PlanKey: "standard_monthly", PriceID: "price_test", State: "open", SessionID: "cs_test_" + suffix, Protocol: 1}
		customer, subscription := "cus_"+suffix, "sub_"+suffix
		exec("INSERT INTO shops(id,name,slug,provisioning_source) VALUES($1,'Webhook shop',$2,'self_service')", shop, "webhook-"+shop.String())
		exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id,stripe_mode) VALUES($1,$2,'test')", shop, customer)
		exec(`INSERT INTO billing_checkout_attempts(id,shop_id,intent,plan_key,state,stripe_session_id,expires_at,protocol_version,stripe_price_id,return_url)
			VALUES($1,$2,'publish','standard_monthly','open',$3,now()+interval '1 day',1,'price_test','http://127.0.0.1:3000/admin/test/configuracoes')`, id, shop, a.SessionID)
		now := time.Now().UTC().Truncate(time.Second)
		p := &webhookProviderFake{reference: billing.BillingReference{CustomerID: customer, SubscriptionID: subscription, AttemptID: id, ShopID: shop},
			snapshot: billing.SubscriptionSnapshot{ID: subscription, CustomerID: customer, PriceID: a.PriceID, Status: domain.SubscriptionActive,
				Session:  billing.CheckoutSession{ID: a.SessionID, ShopID: shop, AttemptID: id, CustomerID: customer, SubscriptionID: subscription, Status: "complete", ExpiresAt: now.Add(time.Hour)},
				Invoices: []billing.ConfirmedInvoice{{ID: "in_" + suffix, PriceID: a.PriceID, Eligible: true, AmountPaid: 9900, Period: domain.Coverage{From: now.Add(-time.Hour), Until: now.Add(23 * time.Hour)}}}}}
		return p, a
	}
	event := func(p *webhookProviderFake, kind string) billing.VerifiedWebhook {
		resource, typ := p.snapshot.Invoices[0].ID, "invoice.paid"
		if kind == "subscription" {
			resource = p.reference.SubscriptionID
			typ = "customer.subscription.updated"
		}
		return billing.VerifiedWebhook{ID: "evt_" + strings.ReplaceAll(uuid.NewString(), "-", ""), Type: typ, APIVersion: stripe.APIVersion, Kind: kind, ResourceID: resource, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	}
	deliver := func(p *webhookProviderFake, e billing.VerifiedWebhook, validSignature bool) *httptest.ResponseRecorder {
		t.Helper()
		receiver := billing.NewReceiveWebhook(store, p)
		handler := httpapi.NewHandler(httpapi.HandlerConfig{StripeWebhook: billinghttp.NewWebhookHandler(verifier, receiver, 10*time.Second, logger), Logger: logger})
		body, _ := json.Marshal(map[string]any{"id": e.ID, "object": "event", "type": e.Type, "api_version": e.APIVersion, "livemode": false, "created": e.CreatedAt.Unix(), "data": map[string]any{"object": map[string]any{"id": e.ResourceID, "customer_email": "private@example.test"}}})
		r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(string(body))).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		if validSignature {
			r.Header.Set("Stripe-Signature", signedWebhookBody(body))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	entitlement := func(shop uuid.UUID, at time.Time) domain.Entitlement {
		t.Helper()
		uc := billing.NewGetShopEntitlements(billingpg.NewReader(db.New(pool)), billing.ClockFunc(func() time.Time { return at }))
		got, err := uc.Execute(ctx, billing.GetShopEntitlementsInput{ShopID: shop})
		if err != nil {
			t.Fatal(err)
		}
		return got.Entitlement
	}
	assertNoPublication := func(shop uuid.UUID) {
		t.Helper()
		var published bool
		if err := pool.QueryRow(ctx, "SELECT publication_requested FROM shops WHERE id=$1", shop).Scan(&published); err != nil || published {
			t.Fatal("webhook published shop", err)
		}
	}
	t.Run("signed payment is atomic deduplicated and retains no body PII", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		w := deliver(p, e, true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if !entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("paid coverage not applied")
		}
		calls := p.calls
		if w := deliver(p, e, true); w.Code != 200 || p.calls != calls {
			t.Fatal("duplicate processed again", w.Code)
		}
		var invoices int
		var payload string
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM billing_paid_invoices WHERE shop_id=$1", a.ShopID).Scan(&invoices); err != nil || invoices != 1 {
			t.Fatal(invoices, err)
		}
		if err := pool.QueryRow(ctx, "SELECT payload::text FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&payload); err != nil || strings.Contains(payload, "private") {
			t.Fatal("PII retained", err)
		}
		assertNoPublication(a.ShopID)
	})
	t.Run("CLI envelope version reconciles and deduplicates without publishing", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		e.APIVersion = "2026-09-30.endive"
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if !entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("CLI payment was not projected")
		}
		calls := p.calls
		if w := deliver(p, e, true); w.Code != 200 || p.calls != calls {
			t.Fatal("duplicate CLI event was reprocessed", w.Code)
		}
		var version string
		if err := pool.QueryRow(ctx, "SELECT api_version FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&version); err != nil || version != e.APIVersion {
			t.Fatal("original version not retained", version, err)
		}
		assertNoPublication(a.ShopID)
	})
	t.Run("unknown envelope versions never reach provider or inbox", func(t *testing.T) {
		p, _ := fixture()
		e := event(p, "invoice")
		e.APIVersion = "2026-11-30.endive"
		if w := deliver(p, e, true); w.Code != 400 || p.calls != 0 {
			t.Fatal(w.Code, p.calls)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("unknown-version receipt persisted", count, err)
		}
	})
	t.Run("invalid signature never reaches provider or inbox", func(t *testing.T) {
		p, _ := fixture()
		e := event(p, "invoice")
		e.APIVersion = "2026-09-30.endive"
		if w := deliver(p, e, false); w.Code != 400 || p.calls != 0 {
			t.Fatal(w.Code)
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&count)
		if count != 0 {
			t.Fatal("unauthenticated event persisted")
		}
	})
	t.Run("unsupported and unrelated events are durably ignored", func(t *testing.T) {
		p, _ := fixture()
		e := event(p, "invoice")
		e.Type = "customer.created"
		e.Kind = "ignored"
		e.ResourceID = ""
		if w := deliver(p, e, true); w.Code != 200 || p.calls != 0 {
			t.Fatal(w.Code)
		}
		e = event(p, "invoice")
		p.reference.CustomerID = "cus_unrelated"
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		var state string
		_ = pool.QueryRow(ctx, "SELECT state FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&state)
		if state != "ignored" {
			t.Fatal(state)
		}
	})
	t.Run("active without paid coverage is not enough", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		p.snapshot.Invoices[0].Eligible = false
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("unpaid active state granted access")
		}
	})
	t.Run("renewal extends paid coverage but a failed future period does not", func(t *testing.T) {
		p, a := fixture()
		first := p.snapshot.Invoices[0]
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		p.snapshot.Status = domain.SubscriptionPastDue
		p.snapshot.Invoices = []billing.ConfirmedInvoice{{ID: first.ID + "next", PriceID: a.PriceID}}
		if w := deliver(p, event(p, "subscription"), true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		if !entitlement(a.ShopID, time.Now()).Allowed || entitlement(a.ShopID, first.Period.Until).Allowed {
			t.Fatal("failed renewal coverage incorrect")
		}
		next := billing.ConfirmedInvoice{ID: first.ID + "next", PriceID: a.PriceID, Eligible: true, AmountPaid: 9900, Period: domain.Coverage{From: first.Period.Until, Until: first.Period.Until.Add(30 * 24 * time.Hour)}}
		p.snapshot.Status = domain.SubscriptionActive
		p.snapshot.Invoices = []billing.ConfirmedInvoice{next}
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if got := entitlement(a.ShopID, time.Now()); !got.Allowed || !got.Until.Equal(next.Period.Until) {
			t.Fatal(got)
		}
	})
	t.Run("two paid invoices never authorize an unpaid gap", func(t *testing.T) {
		p, a := fixture()
		first := p.snapshot.Invoices[0]
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		p.snapshot.Invoices = []billing.ConfirmedInvoice{{ID: first.ID + "future", PriceID: a.PriceID, Eligible: true, AmountPaid: 9900, Period: domain.Coverage{From: first.Period.Until.Add(24 * time.Hour), Until: first.Period.Until.Add(48 * time.Hour)}}}
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if entitlement(a.ShopID, first.Period.Until.Add(time.Hour)).Allowed || !entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("gap or current coverage incorrect")
		}
	})
	t.Run("late event consults canceled state instead of resurrecting access", func(t *testing.T) {
		p, a := fixture()
		old := event(p, "invoice")
		old.CreatedAt = old.CreatedAt.Add(-time.Hour)
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		p.snapshot.Status = domain.SubscriptionCanceled
		if w := deliver(p, event(p, "subscription"), true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if w := deliver(p, old, true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		if entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("old event resurrected canceled subscription")
		}
	})
	t.Run("scheduled cancellation preserves coverage and suspension denies it", func(t *testing.T) {
		p, a := fixture()
		p.snapshot.CancelAtPeriodEnd = true
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		if !entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("scheduled cancellation revoked current coverage")
		}
		p.snapshot.Status = domain.SubscriptionSuspended
		if w := deliver(p, event(p, "subscription"), true); w.Code != 200 {
			t.Fatal(w.Code)
		}
		if entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("suspension kept operation")
		}
	})
	t.Run("provider failure returns retry and a new receiver recovers after restart", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		p.fail = true
		if w := deliver(p, e, true); w.Code != 503 {
			t.Fatal(w.Code)
		}
		var state string
		_ = pool.QueryRow(ctx, "SELECT state FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&state)
		if state != "pending" {
			t.Fatal(state)
		}
		p.fail = false
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if !entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("restart recovery failed")
		}
	})
	t.Run("generation fencing rejects an older fetched snapshot", func(t *testing.T) {
		p, a := fixture()
		older, newer := event(p, "invoice"), event(p, "subscription")
		if _, err := store.AcceptWebhook(ctx, older); err != nil {
			t.Fatal(err)
		}
		first, err := store.ClaimReconciliation(ctx, older, p.reference)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AcceptWebhook(ctx, newer); err != nil {
			t.Fatal(err)
		}
		last, err := store.ClaimReconciliation(ctx, newer, p.reference)
		if err != nil {
			t.Fatal(err)
		}
		canceled := p.snapshot
		canceled.Status = domain.SubscriptionCanceled
		if err := store.ApplyReconciliation(ctx, *last, canceled); err != nil {
			t.Fatal(err)
		}
		if err := store.ApplyReconciliation(ctx, *first, p.snapshot); !errors.Is(err, billing.ErrReconciliationConflict) {
			t.Fatal(err)
		}
		if entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("stale snapshot overwrote cancellation")
		}
	})
	t.Run("simultaneous duplicate deliveries create one subscription and invoice", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		receiver := billing.NewReceiveWebhook(store, p)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for range 2 {
			wg.Go(func() { results <- receiver.Execute(ctx, e) })
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil && !errors.Is(err, billing.ErrWebhookRetry) {
				t.Fatal(err)
			}
		}
		if err := receiver.Execute(ctx, e); err != nil {
			t.Fatal(err)
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_subscriptions WHERE shop_id=$1", a.ShopID).Scan(&count)
		if count != 1 {
			t.Fatal(count)
		}
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_paid_invoices WHERE shop_id=$1", a.ShopID).Scan(&count)
		if count != 1 {
			t.Fatal(count)
		}
	})
	t.Run("tenant mismatch never updates either subscription", func(t *testing.T) {
		p, a := fixture()
		other, b := fixture()
		p.reference.ShopID = b.ShopID
		if w := deliver(p, event(p, "invoice"), true); w.Code != 503 {
			t.Fatal(w.Code)
		}
		if entitlement(a.ShopID, time.Now()).Allowed || entitlement(b.ShopID, time.Now()).Allowed {
			t.Fatal("cross tenant payment granted access")
		}
		p.reference.ShopID = a.ShopID
		p.snapshot.Session.CustomerID = other.reference.CustomerID
		if w := deliver(p, event(p, "invoice"), true); w.Code != 503 {
			t.Fatal(w.Code)
		}
	})
	t.Run("webhook arriving before local Session linkage recovers the existing attempt", func(t *testing.T) {
		p, a := fixture()
		exec("UPDATE billing_checkout_attempts SET state='preparing',stripe_session_id=NULL,expires_at=NULL WHERE id=$1", a.ID)
		if w := deliver(p, event(p, "invoice"), true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var state string
		_ = pool.QueryRow(ctx, "SELECT state FROM billing_checkout_attempts WHERE id=$1", a.ID).Scan(&state)
		if state != "completed" {
			t.Fatal(state)
		}
		assertNoPublication(a.ShopID)
	})
	t.Run("database failure rolls back invoice projection and receipt completion", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		// Force the final link to fail after the financial writes, then verify rollback.
		exec("UPDATE billing_checkout_attempts SET state='expired' WHERE id=$1", a.ID)
		if w := deliver(p, e, true); w.Code != 503 {
			t.Fatal(w.Code)
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_paid_invoices WHERE shop_id=$1", a.ShopID).Scan(&count)
		if count != 0 {
			t.Fatal("financial writes escaped rollback")
		}
		var state string
		_ = pool.QueryRow(ctx, "SELECT state FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&state)
		if state != "pending" {
			t.Fatal(state)
		}
		exec("UPDATE billing_checkout_attempts SET state='review' WHERE id=$1", a.ID)
		p.snapshot.Session.ExpiresAt = time.Now().Add(time.Hour)
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("failure at commit does not acknowledge or grant coverage", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		exec(`CREATE FUNCTION public.codex_fail_webhook_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.shop_id='` + a.ShopID.String() + `'::uuid THEN RAISE EXCEPTION 'injected deferred commit failure';END IF;RETURN NEW;END $$;
		CREATE CONSTRAINT TRIGGER codex_webhook_commit_failure AFTER INSERT OR UPDATE ON public.billing_subscriptions
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.codex_fail_webhook_commit();`)
		defer exec("DROP TRIGGER IF EXISTS codex_webhook_commit_failure ON public.billing_subscriptions;DROP FUNCTION IF EXISTS public.codex_fail_webhook_commit()")
		if w := deliver(p, e, true); w.Code != 503 {
			t.Fatal(w.Code)
		}
		if entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("failed commit granted coverage")
		}
		var state string
		_ = pool.QueryRow(ctx, "SELECT state FROM billing_webhook_events WHERE stripe_event_id=$1", e.ID).Scan(&state)
		if state != "pending" {
			t.Fatal(state)
		}
		exec("DROP TRIGGER codex_webhook_commit_failure ON public.billing_subscriptions")
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("confirmed expired Session releases only its existing attempt", func(t *testing.T) {
		p, a := fixture()
		e := event(p, "invoice")
		e.Kind = "checkout"
		e.Type = "checkout.session.expired"
		e.ResourceID = a.SessionID
		p.reference.SubscriptionID = ""
		p.snapshot.ID = ""
		p.snapshot.Session.SubscriptionID = ""
		p.snapshot.Session.Status = "expired"
		p.snapshot.Invoices = nil
		if w := deliver(p, e, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if entitlement(a.ShopID, time.Now()).Allowed {
			t.Fatal("expired checkout granted access")
		}
		var state string
		_ = pool.QueryRow(ctx, "SELECT state FROM billing_checkout_attempts WHERE id=$1", a.ID).Scan(&state)
		if state != "expired" {
			t.Fatal(state)
		}
	})
	t.Run("webhook HTTP limits and disabled configuration fail closed", func(t *testing.T) {
		p, _ := fixture()
		receiver := billing.NewReceiveWebhook(store, p)
		for _, tc := range []struct {
			verifier    billing.WebhookVerifier
			body, media string
			want        int
		}{
			{nil, `{}`, "application/json", 503}, {verifier, `{}`, "text/plain", 415}, {verifier, strings.Repeat(" ", 512*1024+1), "application/json", 413},
		} {
			h := billinghttp.NewWebhookHandler(tc.verifier, receiver, time.Second, logger)
			r := httptest.NewRequest("POST", "/api/v1/webhooks/stripe", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			h.Receive(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, tc.want)
			}
		}
		if p.calls != 0 {
			t.Fatal("invalid request reached Stripe")
		}
	})
	t.Run("migration rollback refuses reconciled records", func(t *testing.T) {
		contents, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "00014_stripe_webhook_reconciliation.sql"))
		if err != nil {
			t.Fatal(err)
		}
		_, down, _ := strings.Cut(string(contents), "-- +goose Down")
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, down); err == nil || !strings.Contains(err.Error(), "no reconciliation records") {
			t.Fatal(err)
		}
	})
}
