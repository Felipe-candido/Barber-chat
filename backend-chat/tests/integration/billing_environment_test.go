//go:build integration

package integration

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/stripe"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLiveBillingDatabaseIsolation(t *testing.T) {
	dsn := requiredEnv(t, "LIVE_BILLING_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(u.Path, "/codex_billing_live_") {
		t.Fatal("requires local disposable live-mode test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var auth bool
	if err = pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&auth); err != nil || auth {
		t.Fatal("refusing auth schema", err)
	}
	if err = billingpg.EnsureBillingEnvironment(ctx, pool, domain.BillingLive); err != nil {
		t.Fatal(err)
	}
	if err = billingpg.EnsureBillingEnvironment(ctx, pool, domain.BillingTest); err == nil {
		t.Fatal("deployment switched a live database to test mode")
	}
	shop := uuid.New()
	if _, err = pool.Exec(ctx, "INSERT INTO shops(id,name,slug,provisioning_source) VALUES($1,'Mode fixture',$2,'self_service')", shop, "mode-"+shop.String()); err != nil {
		t.Fatal(err)
	}
	account, err := billingpg.NewCheckoutStoreForMode(db.New(pool), domain.BillingLive).ReserveAccount(ctx, shop)
	if err != nil || account.Mode != "live" {
		t.Fatal(account, err)
	}
	if _, err = billingpg.NewCheckoutStore(db.New(pool)).ReserveAccount(ctx, shop); err == nil {
		t.Fatal("test account entered live environment")
	}
	if _, err = pool.Exec(ctx, "UPDATE billing_accounts SET stripe_mode='test' WHERE shop_id=$1", shop); err == nil {
		t.Fatal("account environment was rewritten")
	}
	// Stripe envelopes use Unix seconds; match the verified production contract.
	event := app.VerifiedWebhook{ID: "evt_" + strings.ReplaceAll(uuid.NewString(), "-", ""), Type: "customer.created", APIVersion: stripe.APIVersion, Kind: "ignored", ResourceID: "cus_fixture", CreatedAt: time.Now().UTC().Truncate(time.Second), LiveMode: true}
	live := billingpg.NewWebhookStoreForMode(pool, domain.BillingLive)
	for i := 0; i < 2; i++ {
		terminal, err := live.AcceptWebhook(ctx, event)
		if err != nil || !terminal {
			t.Fatal("live receipt or replay rejected", err)
		}
	}
	if _, err = billingpg.NewWebhookStore(pool).AcceptWebhook(ctx, event); !errors.Is(err, app.ErrInvalidWebhook) {
		t.Fatal("test store accepted live event", err)
	}
	event.LiveMode = false
	event.ID += "test"
	if _, err = billingpg.NewWebhookStore(pool).AcceptWebhook(ctx, event); err == nil {
		t.Fatal("test webhook entered a live database")
	}
	for _, sql := range []string{"UPDATE billing_environment SET mode='test' WHERE id", "DELETE FROM billing_environment"} {
		if _, err = pool.Exec(ctx, sql); err == nil {
			t.Fatal("environment binding was mutable")
		}
	}
}
