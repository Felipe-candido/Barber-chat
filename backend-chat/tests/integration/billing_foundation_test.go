//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Explicitly separate from DATABASE_URL: never use the application's Supabase.
// DDL, fixtures, roles and all writes are rolled back; the target must be empty.
func TestBillingFoundation(t *testing.T) {
	dsn := requiredEnv(t, "BILLING_TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("could not connect to disposable billing test database")
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	expectSQLState := func(code, sql string, args ...any) {
		t.Helper()
		exec("SAVEPOINT rejected_write")
		_, err := tx.Exec(ctx, sql, args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("expected SQLSTATE %s, got %v", code, err)
		}
		exec("ROLLBACK TO SAVEPOINT rejected_write; RELEASE SAVEPOINT rejected_write")
	}
	exec(`DO $$ BEGIN
		IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public')
			OR to_regnamespace('auth') IS NOT NULL THEN
			RAISE EXCEPTION 'Billing tests require an empty disposable database';
		END IF;
	END $$;`)
	paths, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var billingUp, billingDown string
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		up, down, ok := strings.Cut(string(contents), "-- +goose Down")
		if !ok {
			t.Fatal("migration is not reversible")
		}
		if filepath.Base(path) == "00009_billing_foundation.sql" {
			billingUp, billingDown = up, down
			break
		}
		exec(up)
	}
	if billingUp == "" {
		t.Fatal("billing migration not found")
	}
	// The current reader also supports the invoice ledger. Preserve the original
	// foundation fixtures/rollback checks while installing that schema explicitly.
	reconciliationContents, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "00014_stripe_webhook_reconciliation.sql"))
	if err != nil {
		t.Fatal(err)
	}
	reconciliationUp, reconciliationDown, ok := strings.Cut(string(reconciliationContents), "-- +goose Down")
	if !ok {
		t.Fatal("webhook migration missing Down")
	}
	shopA, shopB, userID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO shops(id,name,slug) VALUES ($1,'Existing','billing-existing'),($2,'Other','billing-other')", shopA, shopB)
	exec("INSERT INTO users(id,display_name) VALUES ($1,'Existing owner')", userID)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role) VALUES ($1,$2,'owner')", userID, shopA)
	// Simulate Supabase's default browser grants before creating billing tables.
	exec(`DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='anon') THEN CREATE ROLE anon NOLOGIN; END IF;
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN CREATE ROLE authenticated NOLOGIN; END IF;
	END $$;
	ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO anon, authenticated;`)
	exec(billingUp)
	exec(reconciliationUp)
	var active, publication, owner bool
	if err := tx.QueryRow(ctx, `SELECT s.active,s.publication_requested,m.active AND m.role='owner'
		FROM shops s JOIN shop_memberships m ON m.shop_id=s.id WHERE s.id=$1`, shopA).Scan(&active, &publication, &owner); err != nil {
		t.Fatal(err)
	}
	if !active || publication || !owner {
		t.Fatal("migration changed existing shop or owner access")
	}
	exec("SAVEPOINT empty_billing")
	exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id) VALUES ($1,'cus_billing_a'),($2,'cus_billing_b')", shopA, shopB)
	query := db.New(tx)
	uc := application.NewGetShopEntitlements(billingpg.NewReader(query), application.ClockFunc(func() time.Time {
		return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	}))
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	run := func(name string, test func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			exec("SAVEPOINT billing_case")
			defer exec("ROLLBACK TO SAVEPOINT billing_case; RELEASE SAVEPOINT billing_case")
			test()
		})
	}
	insertSubscription := func(id, shopID uuid.UUID, customer, external, status string, from, until any) {
		t.Helper()
		exec(`INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until)
			VALUES ($1,$2,$3,$4,'monthly',$5,$6,$7)`, id, shopID, customer, external, status, from, until)
	}
	read := func(id uuid.UUID) application.ShopEntitlementsOutput {
		t.Helper()
		got, err := uc.Execute(ctx, application.GetShopEntitlementsInput{ShopID: id})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	run("reader handles missing left join without scan failure", func() {
		got := read(shopA)
		if got.Entitlement.Allowed || got.Entitlement.Reason != domain.AccessSubscriptionRequired {
			t.Fatal(got)
		}
		if _, err := uc.Execute(ctx, application.GetShopEntitlementsInput{ShopID: uuid.New()}); !errors.Is(err, application.ErrShopUnavailable) {
			t.Fatal(err)
		}
	})
	run("coverage and current contract are scoped", func() {
		insertSubscription(uuid.New(), shopA, "cus_billing_a", "sub_old", "canceled", nil, nil)
		insertSubscription(uuid.New(), shopA, "cus_billing_a", "sub_current", "active", now.Add(-time.Hour), now.Add(time.Hour))
		got := read(shopA)
		if !got.Entitlement.Allowed || read(shopB).Entitlement.Allowed {
			t.Fatal("tenant access leaked")
		}
		exec("UPDATE billing_subscriptions SET cancel_at_period_end=true WHERE stripe_subscription_id='sub_current'")
		if !read(shopA).Entitlement.Allowed {
			t.Fatal("scheduled cancellation revoked paid coverage")
		}
		exec("UPDATE billing_subscriptions SET paid_until=$1 WHERE stripe_subscription_id='sub_current'", now)
		if read(shopA).Entitlement.Allowed {
			t.Fatal("expired active projection granted access")
		}
		exec("UPDATE billing_subscriptions SET status='past_due',grace_until=$1 WHERE stripe_subscription_id='sub_current'", now.Add(time.Hour))
		if read(shopA).Entitlement.Reason != domain.AccessGracePeriod {
			t.Fatal("explicit grace not honored")
		}
		exec("UPDATE billing_subscriptions SET status='canceled',grace_until=NULL WHERE stripe_subscription_id='sub_current'")
		if read(shopA).Entitlement.Reason != domain.AccessCanceled {
			t.Fatal("cancellation not applied")
		}
		exec("UPDATE shops SET active=false WHERE id=$1", shopA)
		if _, err := uc.Execute(ctx, application.GetShopEntitlementsInput{ShopID: shopA}); !errors.Is(err, application.ErrShopUnavailable) {
			t.Fatal(err)
		}
	})
	run("subscription constraints", func() {
		insert := `INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until)
			VALUES ($1,$2,$3,$4,'monthly',$5,$6,$7)`
		expectSQLState("23503", insert, uuid.New(), shopA, "cus_billing_b", "sub_cross", "pending", nil, nil)
		for _, values := range [][]any{
			{uuid.New(), shopA, "cus_billing_a", "sub_invalid", "unknown", nil, nil},
			{uuid.New(), shopA, "cus_billing_a", "sub_invalid", "active", nil, nil},
			{uuid.New(), shopA, "cus_billing_a", "sub_invalid", "past_due", nil, now},
			{uuid.New(), shopA, "cus_billing_a", "sub_invalid", "active", now, now},
			{uuid.New(), shopA, "cus_billing_a", "sub_invalid", "pending", now, now.Add(time.Hour)},
			{uuid.New(), shopA, "cus_billing_a", "sub_invalid", "active", now, "infinity"},
		} {
			expectSQLState("23514", insert, values...)
		}
		insertSubscription(uuid.New(), shopA, "cus_billing_a", "sub_pending", "pending", nil, nil)
		expectSQLState("23505", insert, uuid.New(), shopA, "cus_billing_a", "sub_duplicate", "pending", nil, nil)
		expectSQLState("23514", "UPDATE billing_subscriptions SET status='past_due',grace_until=$1 WHERE shop_id=$2", now.Add(time.Hour), shopA)
		expectSQLState("23505", "UPDATE billing_accounts SET stripe_customer_id='cus_billing_a' WHERE shop_id=$1", shopB)
	})
	run("checkout uniqueness and uncertain attempts", func() {
		insert := `INSERT INTO billing_checkout_attempts(id,shop_id,intent,plan_key,state) VALUES ($1,$2,'publish','monthly',$3)`
		exec(insert, uuid.New(), shopA, "preparing")
		expectSQLState("23505", insert, uuid.New(), shopA, "preparing")
		exec("UPDATE billing_checkout_attempts SET state='review' WHERE shop_id=$1", shopA)
		expectSQLState("23505", insert, uuid.New(), shopA, "preparing")
		expectSQLState("23514", "UPDATE billing_checkout_attempts SET state='open' WHERE shop_id=$1", shopA)
		exec("UPDATE billing_checkout_attempts SET state='expired' WHERE shop_id=$1", shopA)
		exec(insert, uuid.New(), shopA, "preparing")
		exec(insert, uuid.New(), shopB, "preparing")
	})
	run("webhook inbox deduplication and envelope integrity", func() {
		insert := `INSERT INTO billing_webhook_events(stripe_event_id,event_type,livemode,payload) VALUES ('evt_test','invoice.paid',false,$1::jsonb)`
		exec(insert, `{"id":"evt_test","type":"invoice.paid"}`)
		expectSQLState("23505", insert, `{"id":"evt_test","type":"invoice.paid"}`)
		expectSQLState("23514", "UPDATE billing_webhook_events SET payload='{}'")
		expectSQLState("23514", "UPDATE billing_webhook_events SET state='processed'")
		expectSQLState("23514", "UPDATE billing_webhook_events SET attempts=-1")
		exec("UPDATE billing_webhook_events SET state='processed',processed_at=$1", now)
	})
	run("server only tables", func() {
		for _, table := range []string{"billing_accounts", "billing_subscriptions", "billing_checkout_attempts", "billing_webhook_events", "billing_paid_invoices"} {
			var rls, anonSelect, browserInsert, browserTruncate bool
			if err := tx.QueryRow(ctx, `SELECT relrowsecurity,has_table_privilege('anon',oid,'SELECT'),
				has_table_privilege('authenticated',oid,'INSERT'),has_table_privilege('authenticated',oid,'TRUNCATE')
				FROM pg_class WHERE oid=$1::regclass`, "public."+table).Scan(&rls, &anonSelect, &browserInsert, &browserTruncate); err != nil {
				t.Fatal(err)
			}
			if !rls || anonSelect || browserInsert || browserTruncate {
				t.Fatalf("table %s exposed", table)
			}
		}
		exec("GRANT SELECT ON billing_accounts TO authenticated; SET LOCAL ROLE authenticated")
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM billing_accounts").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("RLS exposed billing without a policy")
		}
		exec("RESET ROLE")
	})
	t.Run("rollback refuses commercial data", func(t *testing.T) { expectSQLState("P0001", billingDown) })
	exec("ROLLBACK TO SAVEPOINT empty_billing; RELEASE SAVEPOINT empty_billing")
	exec(reconciliationDown)
	exec(billingDown)
	exec(billingUp)
	exec(reconciliationUp)
	if read(shopA).Entitlement.Reason != domain.AccessSubscriptionRequired {
		t.Fatal("reapplied migration unexpectedly activated shop")
	}
}
