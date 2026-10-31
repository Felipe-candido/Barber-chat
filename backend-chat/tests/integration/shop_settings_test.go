//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	shopsdomain "github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The test replaces only JWT verification; local authentication, tenant
// resolution, authorization, settings queries and transactions are real.
type settingsVerifier struct{}

func (settingsVerifier) VerifyAccessToken(_ context.Context, token string) (uuid.UUID, error) {
	id, err := uuid.Parse(token)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, identity.ErrInvalidAccessToken
	}
	return id, nil
}

func TestShopSettings(t *testing.T) {
	dsn := requiredEnv(t, "SHOP_SETTINGS_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_shop_settings_") {
		t.Fatal("settings tests require a disposable codex_shop_settings_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to settings test database")
	}
	defer pool.Close()
	var hasAuth bool
	if err := pool.QueryRow(ctx, "SELECT to_regnamespace('auth') IS NOT NULL").Scan(&hasAuth); err != nil || hasAuth {
		t.Fatal("settings tests refuse an Auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider, shop, other, professional := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	slug, otherSlug := "settings-"+shop.String(), "settings-"+other.String()
	exec("INSERT INTO users(id,display_name) VALUES ($1,'Owner'),($2,'Barber'),($3,'Other owner')", owner, barber, outsider)
	exec("INSERT INTO shops(id,name,slug,timezone) VALUES ($1,'Legacy shop',$2,'America/Sao_Paulo'),($3,'Other shop',$4,'America/Sao_Paulo')", shop, slug, other, otherSlug)
	exec("INSERT INTO professionals(id,shop_id,name) VALUES ($1,$2,'Barber')", professional, shop)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES ($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$6,'owner',NULL)", owner, shop, barber, professional, outsider, other)
	q := db.New(pool)
	transactions := shopspg.NewSettingsTransactor(pool, func(q *db.Queries) shops.ShopSettingsTx {
		return shops.ShopSettingsTx{Access: identitypg.NewSettingsOwnerGuard(q), Shops: shopspg.NewSettingsStore(q)}
	})
	get, update := shops.NewGetShopSettings(transactions), shops.NewUpdateShopSettings(transactions)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := identitypg.NewRepository(q)
	handler := httpapi.NewHandler(httpapi.HandlerConfig{ShopSettings: shopshttp.NewSettingsHandler(get, update, 10*time.Second, logger),
		Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)),
		FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
	request := func(method, target string, actor uuid.UUID, body any) *httptest.ResponseRecorder {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "/api/v1/admin/shops/"+target+"/settings", bytes.NewReader(raw)).WithContext(ctx)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if actor != uuid.Nil {
			r.Header.Set("Authorization", "Bearer "+actor.String())
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	scope := shops.SettingsScope{ActorID: owner, ShopID: shop}
	input := func(version int64) shops.UpdateShopSettingsInput {
		return shops.UpdateShopSettingsInput{Scope: scope, ExpectedVersion: version, Settings: shopsdomain.BasicSettings{
			Name: "Updated shop", Phone: "(11) 91234-5678", Description: "A business description",
			Address: shopsdomain.Address{PostalCode: "01310-100", Street: "Avenida Paulista", Number: "100", District: "Bela Vista", City: "São Paulo", State: "SP"}}}
	}
	body := func(version int64) map[string]any {
		return map[string]any{"version": version, "name": "Updated shop", "phone": "(11) 91234-5678", "description": "A business description",
			"address": map[string]any{"postal_code": "01310-100", "street": "Avenida Paulista", "number": "100", "district": "Bela Vista", "city": "São Paulo", "state": "SP"}}
	}
	t.Run("legacy read then complete update", func(t *testing.T) {
		w := request("GET", slug, owner, nil)
		var before struct {
			Address *json.RawMessage `json:"address"`
			Version int64            `json:"version"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &before); err != nil || w.Code != 200 || before.Address != nil || before.Version != 1 {
			t.Fatal(w.Code, w.Body.String(), err)
		}
		w = request("PUT", slug, owner, body(1))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		got, err := get.Execute(ctx, scope)
		if err != nil || got.Version != 2 || got.Phone != "+5511912345678" || got.Address == nil || got.Address.PostalCode != "01310100" {
			t.Fatal(got, err)
		}
		var source string
		var published bool
		if err := pool.QueryRow(ctx, "SELECT provisioning_source,publication_requested FROM shops WHERE id=$1", shop).Scan(&source, &published); err != nil || source != "manual" || published || got.Slug != slug || got.Timezone != "America/Sao_Paulo" {
			t.Fatal("protected fields changed", err)
		}
		w = request("PUT", slug, owner, body(1))
		if w.Code != 409 || !strings.Contains(w.Body.String(), "shop_settings_conflict") {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("tenant and role isolation", func(t *testing.T) {
		for _, test := range []struct {
			target string
			actor  uuid.UUID
			want   int
		}{
			{slug, uuid.Nil, 401}, {slug, barber, 403}, {slug, outsider, 403}, {otherSlug, owner, 403},
		} {
			for _, method := range []string{"GET", "PUT"} {
				var data any
				if method == "PUT" {
					data = body(2)
				}
				w := request(method, test.target, test.actor, data)
				if w.Code != test.want {
					t.Fatal(method, w.Code, w.Body.String())
				}
			}
		}
		got, err := get.Execute(ctx, shops.SettingsScope{ActorID: outsider, ShopID: other})
		if err != nil || got.Version != 1 || got.Name != "Other shop" || got.Address != nil {
			t.Fatal("other tenant changed", got, err)
		}
	})
	t.Run("missing address cannot clear existing data", func(t *testing.T) {
		data := body(2)
		delete(data, "address")
		w := request("PUT", slug, owner, data)
		if w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
		got, err := get.Execute(ctx, scope)
		if err != nil || got.Version != 2 || got.Address == nil {
			t.Fatal(got, err)
		}
	})
	t.Run("address failure rolls back name contact and version", func(t *testing.T) {
		exec("ALTER TABLE shop_addresses ADD CONSTRAINT codex_settings_reject_city CHECK (city <> 'Rejected city') NOT VALID")
		defer exec("ALTER TABLE shop_addresses DROP CONSTRAINT codex_settings_reject_city")
		candidate := input(2)
		candidate.Settings.Name = "Must roll back"
		candidate.Settings.Address.City = "Rejected city"
		if _, err := update.Execute(ctx, candidate); err == nil {
			t.Fatal("injected database error accepted")
		}
		got, err := get.Execute(ctx, scope)
		if err != nil || got.Version != 2 || got.Name != "Updated shop" || got.Address.City != "São Paulo" {
			t.Fatal("partial update committed", got, err)
		}
	})
	t.Run("concurrent edits yield one winner and one conflict", func(t *testing.T) {
		candidates := []shops.UpdateShopSettingsInput{input(2), input(2)}
		candidates[0].Settings.Name, candidates[0].Settings.Address.Number = "Winner one", "111"
		candidates[1].Settings.Name, candidates[1].Settings.Address.Number = "Winner two", "222"
		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; _, errs[i] = update.Execute(ctx, candidates[i]) }(i)
		}
		close(start)
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatal("wrong concurrency results", errs)
		}
		winner := 0
		if errs[0] != nil {
			winner = 1
		}
		if !errors.Is(errs[1-winner], shops.ErrSettingsConflict) {
			t.Fatal(errs)
		}
		got, err := get.Execute(ctx, scope)
		if err != nil || got.Version != 3 || got.Name != candidates[winner].Settings.Name || got.Address.Number != candidates[winner].Settings.Address.Number {
			t.Fatal("mixed or lost update", got, err)
		}
	})
	t.Run("stale verified scope cannot bypass current owner state", func(t *testing.T) {
		for _, state := range []struct{ revoke, restore string }{
			{"UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", "UPDATE shop_memberships SET active=true WHERE user_id=$1 AND shop_id=$2"},
			{"UPDATE users SET active=false WHERE id=$1 AND $2::uuid IS NOT NULL", "UPDATE users SET active=true WHERE id=$1 AND $2::uuid IS NOT NULL"},
		} {
			exec(state.revoke, owner, shop)
			if _, err := get.Execute(ctx, scope); !errors.Is(err, shops.ErrSettingsAccessDenied) {
				t.Fatal(err)
			}
			if _, err := update.Execute(ctx, input(3)); !errors.Is(err, shops.ErrSettingsAccessDenied) {
				t.Fatal(err)
			}
			exec(state.restore, owner, shop)
		}
		exec("UPDATE shops SET active=false WHERE id=$1", shop)
		if _, err := update.Execute(ctx, input(3)); !errors.Is(err, shops.ErrSettingsNotFound) {
			t.Fatal(err)
		}
		exec("UPDATE shops SET active=true WHERE id=$1", shop)
	})
	t.Run("owner membership stays locked until settings transaction ends", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if err := identitypg.NewSettingsOwnerGuard(db.New(tx)).RequireSettingsOwner(ctx, owner, shop); err != nil {
			t.Fatal(err)
		}
		limited, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		_, err = pool.Exec(limited, "UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", owner, shop)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("membership lock did not protect the operation", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("self service settings do not require payment or grant public access", func(t *testing.T) {
		exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", other)
		w := request("GET", otherSlug, outsider, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var published bool
		if err := pool.QueryRow(ctx, "SELECT publication_requested FROM shops WHERE id=$1", other).Scan(&published); err != nil || published {
			t.Fatal("settings changed publication", err)
		}
	})
	t.Run("new columns remain inaccessible to browser roles", func(t *testing.T) {
		exec(`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN CREATE ROLE authenticated NOLOGIN; END IF; END $$`)
		var enabled, readable, writable bool
		if err := pool.QueryRow(ctx, `SELECT relrowsecurity,
			has_column_privilege('authenticated','public.shops','description','SELECT'),
			has_column_privilege('authenticated','public.shops','settings_version','UPDATE')
			FROM pg_class WHERE oid='public.shops'::regclass`).Scan(&enabled, &readable, &writable); err != nil || !enabled || readable || writable {
			t.Fatal("settings exposed to browser", err)
		}
	})
}
