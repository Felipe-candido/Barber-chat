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
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWhatsAppConnections(t *testing.T) {
	dsn := requiredEnv(t, "WHATSAPP_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_whatsapp_") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("local disposable codex_whatsapp_* database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var auth bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name='auth')").Scan(&auth); err != nil || auth {
		t.Fatal("refusing auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider, shop, other, professional := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber'),($3,'Other')", owner, barber, outsider)
	exec("INSERT INTO shops(id,name,slug) VALUES($1,'Shop',$2),($3,'Other',$4)", shop, "wa-"+shop.String(), other, "wa-"+other.String())
	exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Test')", professional, shop)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$6,'owner',NULL)", owner, shop, barber, professional, outsider, other)
	r := notificationspg.NewConnections(pool, func(q *db.Queries) app.ConnectionOwnerAccess { return identitypg.NewConnectionOwnerGuard(q) })
	scope := app.ConnectionScope{ActorID: owner, ShopID: shop}
	t.Run("authorization and initial state", func(t *testing.T) {
		c, err := r.ReadOwner(ctx, scope)
		if err != nil || c.State != domain.ConnectionNotConfigured || c.InstanceName != "" {
			t.Fatal(c, err)
		}
		for _, s := range []app.ConnectionScope{{ActorID: barber, ShopID: shop}, {ActorID: outsider, ShopID: shop}, {ActorID: owner, ShopID: other}} {
			if _, err := r.ClaimOwner(ctx, s, true); !errors.Is(err, app.ErrConnectionAccessDenied) {
				t.Fatal("unauthorized actor claimed a provider operation", err)
			}
		}
	})
	var current domain.Connection
	t.Run("one instance busy claims and CAS fencing", func(t *testing.T) {
		current, err = r.ClaimOwner(ctx, scope, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.ClaimOwner(ctx, scope, true); !errors.Is(err, app.ErrConnectionBusy) {
			t.Fatal("concurrent owner claim accepted", err)
		}
		if applied, err := r.Complete(ctx, current, app.ConnectionObservation{State: domain.ConnectionConnected}); err != nil || !applied {
			t.Fatal("claim did not complete", err)
		}
		next, err := r.ClaimOwner(ctx, scope, false)
		if err != nil || next.InstanceName != current.InstanceName || next.Generation <= current.Generation {
			t.Fatal("reconnect changed instance identity", err)
		}
		if applied, err := r.Complete(ctx, current, app.ConnectionObservation{State: domain.ConnectionConnected}); err != nil || applied {
			t.Fatal("late result overwrote newer intent", err)
		}
		current = next
	})
	t.Run("expired lease rejects its previous outcome", func(t *testing.T) {
		exec("UPDATE shop_whatsapp_connections SET lease_until=clock_timestamp()-interval '1 second',next_check_at=clock_timestamp() WHERE shop_id=$1", shop)
		if applied, err := r.Complete(ctx, current, app.ConnectionObservation{State: domain.ConnectionConnected}); err != nil || applied {
			t.Fatal("expired action revived connection", err)
		}
		claimed, found, err := r.ClaimDue(ctx)
		if err != nil || !found {
			t.Fatal("expired lease was not recovered", err)
		}
		if claimed.ShopID != shop {
			t.Fatal("wrong tenant claim")
		}
		current = claimed
		if applied, err := r.Complete(ctx, current, app.ConnectionObservation{State: domain.ConnectionDisconnected}); err != nil || !applied {
			t.Fatal(err)
		}
	})
	t.Run("event hint does not overwrite observed state", func(t *testing.T) {
		c, err := r.ClaimOwner(ctx, scope, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Complete(ctx, c, app.ConnectionObservation{State: domain.ConnectionConnected}); err != nil {
			t.Fatal(err)
		}
		if err := r.MarkDirty(ctx, c.InstanceName); err != nil {
			t.Fatal(err)
		}
		c, err = r.ReadOwner(ctx, scope)
		if err != nil || c.State != domain.ConnectionConnected || !c.FreshAt(c.ObservedAt) {
			t.Fatal("hint regressed verified connection", err)
		}
		exec("UPDATE shop_whatsapp_connections SET verified_at=clock_timestamp()-interval '3 minutes',next_check_at=clock_timestamp() WHERE shop_id=$1", shop)
		results := make(chan bool, 2)
		for range 2 {
			go func() { _, found, err := r.ClaimDue(ctx); results <- found && err == nil }()
		}
		a, b := <-results, <-results
		if a == b {
			t.Fatal("two monitors claimed the same connection", a, b)
		}
	})
	t.Run("revoked owner and RLS", func(t *testing.T) {
		exec("UPDATE shop_memberships SET active=FALSE WHERE user_id=$1 AND shop_id=$2", owner, shop)
		if _, err := r.ReadOwner(ctx, scope); !errors.Is(err, app.ErrConnectionAccessDenied) {
			t.Fatal("revoked owner read connection", err)
		}
		var protected bool
		if err := pool.QueryRow(ctx, "SELECT relrowsecurity FROM pg_class WHERE oid='public.shop_whatsapp_connections'::regclass").Scan(&protected); err != nil || !protected {
			t.Fatal("connection table lacks RLS", err)
		}
	})
}
