//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Models only Stripe's key-based effects; PostgreSQL, use cases, ownership and
// HTTP routing are real. External contract/validation has separate adapter tests.
type checkoutProviderFake struct {
	mu                                                   sync.Mutex
	customers                                            map[uuid.UUID]string
	sessions                                             map[uuid.UUID]billing.CheckoutSession
	requests                                             map[uuid.UUID]billing.CheckoutAttempt
	customerCalls, sessionCalls, lookupCalls, priceCalls int
	loseCustomer, loseSession, failLookup                bool
	afterCreate                                          func()
	afterPrice                                           func()
	amounts                                              map[string]int64
}

func newCheckoutProviderFake() *checkoutProviderFake {
	return &checkoutProviderFake{customers: map[uuid.UUID]string{}, sessions: map[uuid.UUID]billing.CheckoutSession{}, requests: map[uuid.UUID]billing.CheckoutAttempt{}}
}
func (p *checkoutProviderFake) ReadCheckoutPrice(_ context.Context, id string) (billing.CheckoutPrice, error) {
	p.mu.Lock()
	p.priceCalls++
	amount := int64(9900)
	if value, ok := p.amounts[id]; ok {
		amount = value
	}
	callback := p.afterPrice
	p.mu.Unlock()
	if callback != nil {
		callback()
	}
	return billing.CheckoutPrice{ID: id, AmountCents: amount}, nil
}
func (p *checkoutProviderFake) CreateCheckoutCustomer(_ context.Context, shop uuid.UUID) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.customerCalls++
	if p.customers[shop] == "" {
		p.customers[shop] = "cus_" + strings.ReplaceAll(shop.String(), "-", "")
	}
	if p.loseCustomer {
		p.loseCustomer = false
		return "", errors.New("response lost after creating Customer")
	}
	return p.customers[shop], nil
}
func (p *checkoutProviderFake) CreateCheckoutSession(_ context.Context, a billing.CheckoutAttempt, customer string) (billing.CheckoutSession, error) {
	p.mu.Lock()
	p.sessionCalls++
	// State/session IDs may change locally; these are the frozen create inputs.
	frozen := a
	frozen.State = ""
	frozen.SessionID = ""
	frozen.CreatedAt = time.Time{}
	if before, ok := p.requests[a.ID]; ok && !reflect.DeepEqual(before, frozen) {
		p.mu.Unlock()
		return billing.CheckoutSession{}, errors.New("idempotent parameters changed")
	}
	p.requests[a.ID] = frozen
	if _, ok := p.sessions[a.ID]; !ok {
		p.sessions[a.ID] = billing.CheckoutSession{ID: "cs_test_" + strings.ReplaceAll(a.ID.String(), "-", ""), ShopID: a.ShopID, AttemptID: a.ID,
			Currency: "brl", AmountSubtotal: 9900, AmountTotal: 9900,
			CustomerID: customer, URL: "https://checkout.stripe.com/c/pay/" + a.ID.String(), Status: "open", ExpiresAt: time.Now().Add(24 * time.Hour).UTC()}
	}
	session, lose, callback := p.sessions[a.ID], p.loseSession, p.afterCreate
	p.loseSession = false
	p.mu.Unlock()
	if callback != nil {
		callback()
	}
	if lose {
		return billing.CheckoutSession{}, errors.New("response lost after creating Session")
	}
	return session, nil
}
func (p *checkoutProviderFake) FindCheckoutSession(_ context.Context, id string) (billing.CheckoutSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lookupCalls++
	if p.failLookup {
		return billing.CheckoutSession{}, errors.New("provider unavailable")
	}
	for _, session := range p.sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return billing.CheckoutSession{}, errors.New("unknown Session")
}
func (p *checkoutProviderFake) setStatus(id uuid.UUID, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.sessions[id]
	s.Status = status
	s.URL = ""
	p.sessions[id] = s
}

func TestStripeCheckout(t *testing.T) {
	dsn := requiredEnv(t, "CHECKOUT_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_checkout_") {
		t.Fatal("checkout tests require a disposable codex_checkout_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to checkout test database")
	}
	defer pool.Close()
	var hasAuth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&hasAuth); err != nil || hasAuth {
		t.Fatal("checkout tests refuse an Auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users(id,display_name) VALUES ($1,'Owner'),($2,'Barber'),($3,'Outsider')", owner, barber, outsider)
	q := db.New(pool)
	transactions := billingpg.NewCheckoutTransactor(pool, func(q *db.Queries) billing.CheckoutTx {
		return billing.CheckoutTx{Access: identitypg.NewBillingOwnerGuard(q), Shops: shopspg.NewResolver(q), Store: billingpg.NewCheckoutStore(q)}
	})
	plan := billing.CheckoutPlan{Key: "standard_monthly", PriceID: "price_original", FrontendOrigin: "http://127.0.0.1:3000"}
	fixture := func() billing.StartCheckoutInput {
		t.Helper()
		shop := uuid.New()
		slug := "checkout-" + shop.String()
		exec("INSERT INTO shops(id,name,slug,provisioning_source) VALUES ($1,'Checkout shop',$2,'self_service')", shop, slug)
		exec("INSERT INTO shop_memberships(user_id,shop_id,role) VALUES ($1,$2,'owner')", owner, shop)
		return billing.StartCheckoutInput{Scope: billing.BillingScope{ActorID: owner, ShopID: shop}, Intent: domain.CheckoutPublish,
			OfferID: (billing.CheckoutOffer{ShopID: shop, PlanKey: plan.Key, PriceID: plan.PriceID, AmountCents: 9900}).ConfirmationID()}
	}
	useCase := func(p billing.CheckoutProvider, config billing.CheckoutPlan) *billing.StartCheckout {
		return billing.NewStartCheckout(transactions, p, billing.ClockFunc(time.Now), config)
	}
	assertNoActivation := func(shop uuid.UUID) {
		t.Helper()
		var published bool
		var count int
		if err := pool.QueryRow(ctx, "SELECT publication_requested,(SELECT count(*) FROM billing_subscriptions WHERE shop_id=$1) FROM shops WHERE id=$1", shop).Scan(&published, &count); err != nil || published || count != 0 {
			t.Fatal("Checkout granted operation/publication", err)
		}
	}
	t.Run("offer reads price without reserving account and stale confirmation creates nothing", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		offer, err := uc.Offer(ctx, input.Scope)
		if err != nil || offer.AmountCents != 9900 || offer.OfferID != input.OfferID || offer.Recovering {
			t.Fatal(offer, err)
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM billing_accounts WHERE shop_id=$1)+(SELECT count(*) FROM billing_checkout_attempts WHERE shop_id=$1)", input.Scope.ShopID).Scan(&count)
		if count != 0 || p.customerCalls != 0 || p.sessionCalls != 0 {
			t.Fatal("GET had external/local writes", count)
		}
		input.OfferID = strings.Repeat("a", 64)
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutOfferChanged) {
			t.Fatal(err)
		}
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_accounts WHERE shop_id=$1", input.Scope.ShopID).Scan(&count)
		if count != 0 || p.customerCalls != 0 || p.sessionCalls != 0 {
			t.Fatal("stale offer created billing state")
		}
	})
	t.Run("offer recovers frozen price after configuration changes", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		if _, err := useCase(p, plan).Execute(ctx, input); err != nil {
			t.Fatal(err)
		}
		p.amounts = map[string]int64{"price_changed": 15000}
		changed := billing.CheckoutPlan{Key: "new_plan", PriceID: "price_changed", FrontendOrigin: plan.FrontendOrigin}
		offer, err := useCase(p, changed).Offer(ctx, input.Scope)
		if err != nil || !offer.Recovering || offer.PriceID != plan.PriceID || offer.AmountCents != 9900 || offer.OfferID != input.OfferID {
			t.Fatal(offer, err)
		}
	})
	t.Run("access revoked during price lookup does not reveal the offer", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		p.afterPrice = func() {
			exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", owner, input.Scope.ShopID)
		}
		if _, err := useCase(p, plan).Offer(ctx, input.Scope); !errors.Is(err, billing.ErrBillingAccessDenied) {
			t.Fatal(err)
		}
		if p.customerCalls != 0 || p.sessionCalls != 0 {
			t.Fatal("unauthorized offer had effects")
		}
	})
	t.Run("provider session total and currency must match the reviewed price", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		result, err := uc.Execute(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		session := p.sessions[result.AttemptID]
		session.AmountTotal++
		p.sessions[result.AttemptID] = session
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutReview) {
			t.Fatal(err)
		}
		session.AmountTotal--
		session.Currency = "usd"
		p.sessions[result.AttemptID] = session
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutReview) {
			t.Fatal(err)
		}
	})
	t.Run("concurrent calls share one durable attempt and external effects", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		outputs := make(chan billing.CheckoutOutput, 2)
		failures := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { result, err := uc.Execute(ctx, input); outputs <- result; failures <- err })
		}
		wg.Wait()
		close(outputs)
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
		var id uuid.UUID
		for result := range outputs {
			if id != uuid.Nil && result.AttemptID != id {
				t.Fatal("duplicate attempt")
			}
			id = result.AttemptID
		}
		if len(p.sessions) != 1 || len(p.customers) != 1 {
			t.Fatal("duplicate external effects")
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_checkout_attempts WHERE shop_id=$1", input.Scope.ShopID).Scan(&count)
		if count != 1 {
			t.Fatal(count)
		}
		assertNoActivation(input.Scope.ShopID)
	})
	t.Run("lost Session response recovers frozen parameters after configuration changes", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		p.loseSession = true
		if _, err := useCase(p, plan).Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutUnavailable) {
			t.Fatal(err)
		}
		changed := billing.CheckoutPlan{Key: "new_plan", PriceID: "price_changed", FrontendOrigin: "https://new.example.test"}
		result, err := useCase(p, changed).Execute(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.sessions) != 1 || p.requests[result.AttemptID].PriceID != plan.PriceID || !strings.HasPrefix(p.requests[result.AttemptID].ReturnURL, plan.FrontendOrigin) {
			t.Fatal("request not frozen")
		}
		assertNoActivation(input.Scope.ShopID)
	})
	t.Run("lost Customer response recovers original customer key", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		p.loseCustomer = true
		uc := useCase(p, plan)
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutUnavailable) {
			t.Fatal(err)
		}
		if _, err := uc.Execute(ctx, input); err != nil {
			t.Fatal(err)
		}
		if len(p.customers) != 1 || len(p.sessions) != 1 || p.customerCalls != 2 {
			t.Fatal("Customer duplicated/not recovered")
		}
	})
	t.Run("known Session failure never falls back to create", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		if _, err := uc.Execute(ctx, input); err != nil {
			t.Fatal(err)
		}
		creates := p.sessionCalls
		p.failLookup = true
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutUnavailable) {
			t.Fatal(err)
		}
		if p.sessionCalls != creates {
			t.Fatal("created another Session")
		}
	})
	t.Run("completed Checkout holds new checkout and never grants access", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		result, err := uc.Execute(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		p.setStatus(result.AttemptID, "complete")
		for range 2 {
			if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutPending) {
				t.Fatal(err)
			}
		}
		if len(p.sessions) != 1 {
			t.Fatal("new Session after complete")
		}
		assertNoActivation(input.Scope.ShopID)
	})
	t.Run("only confirmed expiration permits another explicit checkout", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		result, err := uc.Execute(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		p.setStatus(result.AttemptID, "expired")
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutExpired) {
			t.Fatal(err)
		}
		if len(p.sessions) != 1 {
			t.Fatal("expiration opened a new checkout implicitly")
		}
		next, err := uc.Execute(ctx, input)
		if err != nil || next.AttemptID == result.AttemptID || len(p.sessions) != 2 {
			t.Fatal(next, err)
		}
	})
	t.Run("unlinked attempt older than recovery window remains blocked", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		p.loseSession = true
		uc := useCase(p, plan)
		_, _ = uc.Execute(ctx, input)
		exec("UPDATE billing_checkout_attempts SET created_at=now()-interval '21 hours' WHERE shop_id=$1", input.Scope.ShopID)
		calls := p.sessionCalls
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutReview) {
			t.Fatal(err)
		}
		if p.sessionCalls != calls || len(p.sessions) != 1 {
			t.Fatal("reused a potentially pruned key")
		}
	})
	t.Run("unknown account mode is not silently reused", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id) VALUES ($1,$2)", input.Scope.ShopID, "cus_legacy_"+input.Scope.ShopID.String())
		if _, err := useCase(p, plan).Execute(ctx, input); !errors.Is(err, billing.ErrCheckoutReview) {
			t.Fatal(err)
		}
		if p.sessionCalls != 0 || p.customerCalls != 0 {
			t.Fatal("legacy account reached provider")
		}
	})
	t.Run("manual shops and existing subscriptions never open checkout", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		exec("UPDATE shops SET provisioning_source='manual' WHERE id=$1", input.Scope.ShopID)
		if _, err := useCase(p, plan).Execute(ctx, input); !errors.Is(err, billing.ErrBillingNotRequired) {
			t.Fatal(err)
		}
		if p.customerCalls != 0 {
			t.Fatal("manual shop charged")
		}
		input = fixture()
		uc := useCase(p, plan)
		if _, err := uc.Execute(ctx, input); err != nil {
			t.Fatal(err)
		}
		exec("INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status) SELECT $1,shop_id,stripe_customer_id,$2,'standard_monthly','pending' FROM billing_accounts WHERE shop_id=$3", uuid.New(), "sub_"+uuid.NewString(), input.Scope.ShopID)
		calls := p.sessionCalls
		if _, err := uc.Execute(ctx, input); !errors.Is(err, billing.ErrSubscriptionExists) {
			t.Fatal(err)
		}
		if p.sessionCalls != calls {
			t.Fatal("duplicate subscription requested")
		}
	})
	t.Run("ownership is rechecked after external result", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		p.afterCreate = func() {
			exec("UPDATE shop_memberships SET active=FALSE WHERE user_id=$1 AND shop_id=$2", owner, input.Scope.ShopID)
		}
		if _, err := useCase(p, plan).Execute(ctx, input); !errors.Is(err, billing.ErrBillingAccessDenied) {
			t.Fatal(err)
		}
		if len(p.sessions) != 1 {
			t.Fatal("expected external effect to remain recoverable")
		}
		p.afterCreate = nil
		exec("UPDATE shop_memberships SET active=TRUE WHERE user_id=$1 AND shop_id=$2", owner, input.Scope.ShopID)
		if _, err := useCase(p, plan).Execute(ctx, input); err != nil {
			t.Fatal(err)
		}
		if len(p.sessions) != 1 {
			t.Fatal("external effect duplicated after restore")
		}
	})
	t.Run("HTTP contract and tenant isolation", func(t *testing.T) {
		input, p := fixture(), newCheckoutProviderFake()
		uc := useCase(p, plan)
		prof := uuid.New()
		exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Barber')", prof, input.Scope.ShopID)
		exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'barber',$3)", barber, input.Scope.ShopID, prof)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := httpapi.NewHandler(httpapi.HandlerConfig{Checkout: billinghttp.NewCheckoutHandler(uc, 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: plan.FrontendOrigin, Logger: logger})
		path := "/api/v1/admin/shops/checkout-" + input.Scope.ShopID.String() + "/billing/checkout"
		for _, actor := range []uuid.UUID{uuid.Nil, barber, outsider, owner} {
			r := httptest.NewRequest("GET", strings.TrimSuffix(path, "checkout")+"offer", nil).WithContext(ctx)
			if actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+actor.String())
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := 403
			if actor == uuid.Nil {
				want = 401
			} else if actor == owner {
				want = 200
			}
			if w.Code != want {
				t.Fatal("offer authorization", want, w.Code, w.Body.String())
			}
			if actor == owner {
				var body map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body) != 9 ||
					body["shop_id"] != input.Scope.ShopID.String() || body["offer_id"] != input.OfferID ||
					body["amount_cents"] != float64(9900) || body["currency"] != "BRL" || body["livemode"] != false {
					t.Fatal("invalid public offer", body, err)
				}
				if strings.Contains(w.Body.String(), plan.PriceID) {
					t.Fatal("offer exposed a provider identifier")
				}
			}
		}
		if p.customerCalls != 0 || p.sessionCalls != 0 {
			t.Fatal("offer HTTP created external effects")
		}
		for _, tc := range []struct {
			actor uuid.UUID
			body  string
			want  int
		}{
			{uuid.Nil, `{"intent":"publish"}`, 401}, {barber, `{"intent":"publish"}`, 403}, {outsider, `{"intent":"publish"}`, 403},
			{owner, `null`, 400}, {owner, `{"intent":"publish","price_id":"price_other"}`, 400},
			{owner, `{"intent":"publish","shop_id":"` + uuid.NewString() + `"}`, 400},
			{owner, `{"intent":"publish","success_url":"https://evil.test"}`, 400}, {owner, `{"intent":"publish"} {}`, 400},
			{owner, `{}`, 422}, {owner, `{"intent":"other"}`, 422}, {owner, `{"intent":"publish"}`, 422},
			{owner, `{"intent":"publish","offer_id":"` + strings.Repeat("g", 64) + `"}`, 422},
			{owner, `{"intent":"publish","offer_id":"` + input.OfferID + `"}`, 200},
		} {
			r := httptest.NewRequest("POST", path, strings.NewReader(tc.body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			if tc.actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+tc.actor.String())
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(tc.want, w.Code, w.Body.String())
			}
			if tc.want == 200 {
				var body map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &body)
				if len(body) != 5 || body["livemode"] != false || body["intent"] != "publish" {
					t.Fatal(body)
				}
			}
		}
		assertNoActivation(input.Scope.ShopID)
	})
	t.Run("not configured stays closed without creating billing rows", func(t *testing.T) {
		input := fixture()
		if _, err := useCase(nil, plan).Execute(ctx, input); !errors.Is(err, billing.ErrBillingNotConfigured) {
			t.Fatal(err)
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM billing_accounts WHERE shop_id=$1", input.Scope.ShopID).Scan(&count)
		if count != 0 {
			t.Fatal("reserved account while disabled")
		}
	})
	t.Run("migration rollback refuses commercial records", func(t *testing.T) {
		contents, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "00013_stripe_test_checkout.sql"))
		if err != nil {
			t.Fatal(err)
		}
		_, down, ok := strings.Cut(string(contents), "-- +goose Down")
		if !ok {
			t.Fatal("Down missing")
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		_, err = tx.Exec(ctx, down)
		if err == nil || !strings.Contains(err.Error(), "protocol-1 commercial records") {
			t.Fatal("rollback failed to protect commercial records", err)
		}
	})
}
