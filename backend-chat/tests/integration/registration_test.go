//go:build integration

package integration

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	billing "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	shopsdomain "github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failedInitialOwner struct{ shops.InitialOwnerStore }

var errInitialOwnerWrite = errors.New("injected owner write failure")

func (f failedInitialOwner) CreateOwnerMembership(context.Context, uuid.UUID, uuid.UUID) error {
	return errInitialOwnerWrite
}

// The dedicated DSN and name guard prevent use of the real application DB.
// Tests commit for concurrency checks: use a disposable PostgreSQL with migrations.
func TestShopRegistration(t *testing.T) {
	dsn := requiredEnv(t, "REGISTRATION_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_registration_") {
		t.Fatal("registration requires a disposable codex_registration_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to registration database")
	}
	defer pool.Close()
	var hasAuth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&hasAuth); err != nil || hasAuth {
		t.Fatal("registration tests refuse an Auth schema", err)
	}
	bind := func(q *db.Queries) shops.ShopCreationTx {
		return shops.ShopCreationTx{Owners: identitypg.NewInitialOwnerStore(q), Shops: shopspg.NewCreationStore(q)}
	}
	uc := shops.NewCreateShop(shopspg.NewCreationTransactor(pool, bind))
	input := func(slug string) shops.CreateShopInput {
		return shops.CreateShopInput{OwnerID: uuid.New(), DisplayName: "Original owner", Name: "Registration shop", Slug: slug, Timezone: "America/Sao_Paulo", IdempotencyKey: bookingKey(),
			Address: shopsdomain.Address{PostalCode: "01310-100", Street: "Avenida Paulista", Number: "100", District: "Bela Vista", City: "São Paulo", State: "SP"}}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	initial := input("registration-" + uuid.NewString())
	created, err := uc.Execute(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}
	var active, published bool
	var role, name, source string
	if err := pool.QueryRow(ctx, `SELECT s.active,s.publication_requested,s.provisioning_source,m.role,u.display_name
		FROM shops s JOIN shop_memberships m ON m.shop_id=s.id JOIN users u ON u.id=m.user_id WHERE s.id=$1`, created.Shop.ID).Scan(&active, &published, &source, &role, &name); err != nil {
		t.Fatal(err)
	}
	if !active || published || source != "self_service" || role != "owner" || name != "Original owner" {
		t.Fatal("wrong initial state")
	}
	queries := db.New(pool)
	if count("SELECT count(*) FROM shop_addresses WHERE shop_id=$1 AND postal_code='01310100'", created.Shop.ID) != 1 {
		t.Fatal("address was not committed and normalized")
	}
	t.Run("address constraints and browser privileges", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "UPDATE shop_addresses SET city='' WHERE shop_id=$1", created.Shop.ID); err == nil {
			t.Fatal("database accepted incomplete address")
		}
		var enabled bool
		if err := pool.QueryRow(ctx, "SELECT relrowsecurity FROM pg_class WHERE oid='public.shop_addresses'::regclass").Scan(&enabled); err != nil || !enabled {
			t.Fatal("address RLS missing", err)
		}
	})
	identityRepo := identitypg.NewRepository(queries)
	access := identity.NewAuthorizeShopAction(identityRepo, shopspg.NewResolver(queries))
	if got, err := access.Execute(ctx, identity.AuthorizeShopActionInput{UserID: initial.OwnerID, ShopSlug: created.Shop.Slug}); err != nil || string(got.Role) != "owner" {
		t.Fatal("new owner cannot configure shop", err)
	}
	t.Run("retry and mismatched input", func(t *testing.T) {
		got, err := uc.Execute(ctx, initial)
		if err != nil || !got.Replayed || got.Shop.ID != created.Shop.ID {
			t.Fatalf("%+v %v", got, err)
		}
		if count("SELECT count(*) FROM shop_creation_requests WHERE user_id=$1", initial.OwnerID) != 1 || count("SELECT count(*) FROM shop_memberships WHERE user_id=$1", initial.OwnerID) != 1 {
			t.Fatal("duplicate registration")
		}
		changed := initial
		changed.Name = "Changed"
		if _, err := uc.Execute(ctx, changed); !errors.Is(err, shops.ErrCreationConflict) {
			t.Fatal(err)
		}
	})
	t.Run("slug conflict rolls back new local user", func(t *testing.T) {
		other := input(initial.Slug)
		if _, err := uc.Execute(ctx, other); !errors.Is(err, shops.ErrSlugTaken) {
			t.Fatal(err)
		}
		if count("SELECT count(*) FROM users WHERE id=$1", other.OwnerID) != 0 {
			t.Fatal("orphan local user after conflict")
		}
	})
	t.Run("membership failure rolls back user and shop", func(t *testing.T) {
		failUC := shops.NewCreateShop(shopspg.NewCreationTransactor(pool, func(q *db.Queries) shops.ShopCreationTx {
			b := bind(q)
			b.Owners = failedInitialOwner{b.Owners}
			return b
		}))
		candidate := input("rollback-" + uuid.NewString())
		if _, err := failUC.Execute(ctx, candidate); !errors.Is(err, errInitialOwnerWrite) {
			t.Fatal(err)
		}
		if count("SELECT count(*) FROM shops WHERE slug=$1", candidate.Slug) != 0 || count("SELECT count(*) FROM users WHERE id=$1", candidate.OwnerID) != 0 {
			t.Fatal("partial registration committed")
		}
	})
	t.Run("concurrent identical requests create one unit", func(t *testing.T) {
		candidate := input("concurrent-" + uuid.NewString())
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]shops.CreatedShop, 2)
		errs := make([]error, 2)
		for i := range 2 {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; results[i], errs[i] = uc.Execute(ctx, candidate) }(i)
		}
		close(start)
		wg.Wait()
		if errs[0] != nil || errs[1] != nil || results[0].Shop.ID != results[1].Shop.ID || results[0].Replayed == results[1].Replayed {
			t.Fatalf("%+v %v", results, errs)
		}
		if count("SELECT count(*) FROM shop_creation_requests WHERE user_id=$1", candidate.OwnerID) != 1 {
			t.Fatal("duplicate key committed")
		}
	})
	t.Run("concurrent slug claims cannot steal shop", func(t *testing.T) {
		a := input("claimed-" + uuid.NewString())
		b := input(a.Slug)
		candidates := []shops.CreateShopInput{a, b}
		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; _, errs[i] = uc.Execute(ctx, candidates[i]) }(i)
		}
		close(start)
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatal(errs)
		}
		for i, err := range errs {
			if err != nil {
				if !errors.Is(err, shops.ErrSlugTaken) || count("SELECT count(*) FROM users WHERE id=$1", candidates[i].OwnerID) != 0 {
					t.Fatal("loser left data", err)
				}
			}
		}
		if count("SELECT count(*) FROM shop_memberships m JOIN shops s ON s.id=m.shop_id WHERE s.slug=$1", a.Slug) != 1 {
			t.Fatal("existing shop claimed by another user")
		}
	})
	t.Run("profile and inactive state are preserved", func(t *testing.T) {
		another := initial
		another.IdempotencyKey = bookingKey()
		another.Slug = "another-" + uuid.NewString()
		another.DisplayName = "Do not overwrite"
		if _, err := uc.Execute(ctx, another); err != nil {
			t.Fatal(err)
		}
		var got string
		if err := pool.QueryRow(ctx, "SELECT display_name FROM users WHERE id=$1", initial.OwnerID).Scan(&got); err != nil || got != "Original owner" {
			t.Fatal("profile overwritten", err)
		}
		exec("UPDATE users SET active=false WHERE id=$1", initial.OwnerID)
		if _, err := uc.Execute(ctx, initial); !errors.Is(err, shops.ErrOwnerInactive) {
			t.Fatal(err)
		}
		exec("UPDATE users SET active=true WHERE id=$1", initial.OwnerID)
		exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", initial.OwnerID, created.Shop.ID)
		if _, err := uc.Execute(ctx, initial); !errors.Is(err, shops.ErrOwnershipDenied) {
			t.Fatal(err)
		}
		exec("UPDATE shop_memberships SET active=true WHERE user_id=$1 AND shop_id=$2", initial.OwnerID, created.Shop.ID)
	})
	t.Run("public access needs paid coverage and publication", func(t *testing.T) {
		now := time.Now().UTC()
		eligibility := billing.NewGetShopEntitlements(billingpg.NewReader(queries), billing.ClockFunc(func() time.Time { return now }))
		gate := shops.NewCheckPublicBooking(shopspg.NewResolver(queries), eligibility)
		if err := gate.Execute(ctx, initial.Slug); !errors.Is(err, shops.ErrPublicShopNotFound) {
			t.Fatal("draft publicly available", err)
		}
		exec("UPDATE shops SET publication_requested=true WHERE id=$1", created.Shop.ID)
		if err := gate.Execute(ctx, initial.Slug); !errors.Is(err, shops.ErrPublicShopNotFound) {
			t.Fatal("unpaid publication allowed", err)
		}
		exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id) VALUES ($1,$2)", created.Shop.ID, "cus_"+created.Shop.ID.String())
		exec(`INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until)
			VALUES ($1,$2,$3,$4,'monthly','active',$5,$6)`, uuid.New(), created.Shop.ID, "cus_"+created.Shop.ID.String(), "sub_"+created.Shop.ID.String(), now.Add(-time.Hour), now.Add(time.Hour))
		if err := gate.Execute(ctx, initial.Slug); err != nil {
			t.Fatal(err)
		}
		exec("UPDATE billing_subscriptions SET paid_until=$1 WHERE shop_id=$2", now, created.Shop.ID)
		if err := gate.Execute(ctx, initial.Slug); !errors.Is(err, shops.ErrPublicShopNotFound) {
			t.Fatal("expired coverage remained public", err)
		}
		exec("UPDATE shops SET publication_requested=false WHERE id=$1", created.Shop.ID)
	})
	t.Run("browser roles cannot access retry records", func(t *testing.T) {
		exec(`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN CREATE ROLE authenticated NOLOGIN; END IF; END $$;`)
		var enabled, privilege bool
		if err := pool.QueryRow(ctx, `SELECT relrowsecurity,has_table_privilege('authenticated',oid,'INSERT') FROM pg_class WHERE oid='public.shop_creation_requests'::regclass`).Scan(&enabled, &privilege); err != nil || !enabled || privilege {
			t.Fatal("retry records exposed", err)
		}
	})
}
