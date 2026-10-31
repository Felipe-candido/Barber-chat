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
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	billing "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	billinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/http"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type portalProviderFake struct {
	calls       []billing.PortalRequest
	afterCreate func()
	failure     bool
	created     time.Time
}

func (p *portalProviderFake) CreatePortalSession(_ context.Context, request billing.PortalRequest) (billing.PortalSession, error) {
	p.calls = append(p.calls, request)
	if p.afterCreate != nil {
		p.afterCreate()
	}
	if p.failure {
		return billing.PortalSession{}, errors.New("response lost")
	}
	created := p.created
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return billing.PortalSession{URL: "https://billing.stripe.com/p/session/test", CreatedAt: created}, nil
}

func TestStripePortal(t *testing.T) {
	dsn := requiredEnv(t, "CHECKOUT_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_checkout_") {
		t.Fatal("portal tests require a disposable codex_checkout_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var hasAuth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&hasAuth); err != nil || hasAuth {
		t.Fatal("refusing Auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber'),($3,'Outsider')", owner, barber, outsider)
	fixture := func(status string) (billing.BillingScope, string) {
		t.Helper()
		shop, attempt := uuid.New(), uuid.New()
		slug := "portal-" + shop.String()
		customer := "cus_" + strings.ReplaceAll(shop.String(), "-", "")
		exec("INSERT INTO shops(id,name,slug,provisioning_source) VALUES($1,'Portal shop',$2,'self_service')", shop, slug)
		exec("INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, shop)
		exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id,stripe_mode) VALUES($1,$2,'test')", shop, customer)
		if status != "none" {
			exec("INSERT INTO billing_checkout_attempts(id,shop_id,intent,plan_key,state,protocol_version,stripe_price_id,return_url) VALUES($1,$2,'publish','standard_monthly','review',1,'price_test','http://127.0.0.1:3000')", attempt, shop)
			exec("INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until,reconciliation_protocol,checkout_attempt_id,stripe_price_id) VALUES($1,$2,$3,$4,'standard_monthly',$5,now()-interval '2 days',now()+interval '1 day',1,$6,'price_test')", uuid.New(), shop, customer, "sub_"+strings.ReplaceAll(shop.String(), "-", ""), status, attempt)
		}
		return billing.BillingScope{ActorID: owner, ShopID: shop}, slug
	}
	q := db.New(pool)
	tx := billingpg.NewCheckoutTransactor(pool, func(q *db.Queries) billing.CheckoutTx {
		return billing.CheckoutTx{Access: identitypg.NewBillingOwnerGuard(q), Shops: shopspg.NewResolver(q), Store: billingpg.NewCheckoutStore(q)}
	})
	uc := func(provider billing.PortalProvider) *billing.OpenPortal {
		return billing.NewOpenPortal(tx, provider, billing.ClockFunc(time.Now), "bpc_policy", "http://127.0.0.1:3000")
	}
	t.Run("active delinquent suspended and canceled can manage without publication or financial writes", func(t *testing.T) {
		for _, status := range []string{"active", "past_due", "suspended", "canceled"} {
			scope, slug := fixture(status)
			p := &portalProviderFake{}
			out, err := uc(p).Execute(ctx, scope)
			if err != nil || out.URL == "" || out.ShopID != scope.ShopID || len(p.calls) != 1 {
				t.Fatal("portal rejected", status, err)
			}
			request := p.calls[0]
			if request.CustomerID != "cus_"+strings.ReplaceAll(scope.ShopID.String(), "-", "") || request.ReturnURL != "http://127.0.0.1:3000/admin/"+slug+"/configuracoes?portal=returned#publication" || request.ConfigurationID != "bpc_policy" {
				t.Fatal("wrong tenant/return/configuration")
			}
			var current string
			var published bool
			var attempts, subscriptions int
			err = pool.QueryRow(ctx, "SELECT s.status,h.publication_requested,(SELECT count(*) FROM billing_checkout_attempts WHERE shop_id=$1),(SELECT count(*) FROM billing_subscriptions WHERE shop_id=$1) FROM billing_subscriptions s JOIN shops h ON h.id=s.shop_id WHERE s.shop_id=$1", scope.ShopID).Scan(&current, &published, &attempts, &subscriptions)
			if err != nil || current != status || published || attempts != 1 || subscriptions != 1 {
				t.Fatal("portal changed financial/publication state", err)
			}
		}
	})
	t.Run("abandoned checkout and unavailable provider expose no URL", func(t *testing.T) {
		scope, _ := fixture("none")
		p := &portalProviderFake{}
		if out, err := uc(p).Execute(ctx, scope); !errors.Is(err, billing.ErrPortalNotAvailable) || out.URL != "" || len(p.calls) != 0 {
			t.Fatal(err)
		}
		scope, _ = fixture("active")
		if out, err := uc(nil).Execute(ctx, scope); !errors.Is(err, billing.ErrPortalNotConfigured) || out.URL != "" {
			t.Fatal(err)
		}
		p.failure = true
		if out, err := uc(p).Execute(ctx, scope); !errors.Is(err, billing.ErrPortalUnavailable) || out.URL != "" || len(p.calls) != 1 {
			t.Fatal("automatic retry or disclosure", err)
		}
	})
	t.Run("revocation during network call cannot disclose session and does not hold locks", func(t *testing.T) {
		scope, _ := fixture("active")
		p := &portalProviderFake{afterCreate: func() {
			exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", owner, scope.ShopID)
		}}
		if out, err := uc(p).Execute(ctx, scope); !errors.Is(err, billing.ErrBillingAccessDenied) || out.URL != "" {
			t.Fatal(err)
		}
	})
	t.Run("stale session rejected", func(t *testing.T) {
		scope, _ := fixture("active")
		p := &portalProviderFake{created: time.Now().Add(-10 * time.Minute)}
		if out, err := uc(p).Execute(ctx, scope); !errors.Is(err, billing.ErrPortalUnavailable) || out.URL != "" {
			t.Fatal(err)
		}
	})
	t.Run("HTTP authentication owner scope strict body CORS and no-store", func(t *testing.T) {
		scope, slug := fixture("active")
		p := &portalProviderFake{}
		professional := uuid.New()
		exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Barber')", professional, scope.ShopID)
		exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'barber',$3)", barber, scope.ShopID, professional)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := httpapi.NewHandler(httpapi.HandlerConfig{Portal: billinghttp.NewPortalHandler(uc(p), 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
		for _, tc := range []struct {
			actor uuid.UUID
			body  string
			want  int
		}{{uuid.Nil, "{}", 401}, {barber, "{}", 403}, {outsider, "{}", 403}, {owner, `{"customer":"cus_other"}`, 400}, {owner, `{"return_url":"https://evil.test"}`, 400}, {owner, `{"configuration":"bpc_other"}`, 400}, {owner, "null", 400}, {owner, "{} {}", 400}, {owner, "{}", 200}} {
			r := httptest.NewRequest("POST", "/api/v1/admin/shops/"+slug+"/billing/portal", strings.NewReader(tc.body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			if tc.actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+tc.actor.String())
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal("unexpected status", tc.want, w.Code, w.Body.String())
			}
			if tc.want == 200 {
				var result map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &result)
				if result["shop_id"] != scope.ShopID.String() || result["livemode"] != false || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
					t.Fatal("unsafe response")
				}
			}
		}
		if len(p.calls) != 1 {
			t.Fatal("unauthorized call reached Stripe")
		}
		r := httptest.NewRequest("OPTIONS", "/api/v1/admin/shops/"+slug+"/billing/portal", nil)
		r.Header.Set("Origin", "http://127.0.0.1:3000")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal("preflight rejected", w.Code)
		}
	})
}
