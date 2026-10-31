//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	saleshttp "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/http"
	salespg "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failAfterFirstSaleReturn struct{ sales.SaleStockReturner }

func (s failAfterFirstSaleReturn) RestockCancelledSale(ctx context.Context, c sales.SaleStockCommand) error {
	c.Items = c.Items[:1]
	if err := s.SaleStockReturner.RestockCancelledSale(ctx, c); err != nil {
		return err
	}
	return errors.New("injected failure after first stock return")
}

func TestManageProductSales(t *testing.T) {
	dsn := requiredEnv(t, "SALES_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_sales_") {
		t.Fatal("disposable codex_sales_ database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	key := func() string { return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "") }
	owner, barber, otherBarber, outsider := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	shop, other, professional, otherProfessional := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	slug := "sales-management-" + shop.String()
	exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber'),($3,'Other barber'),($4,'Outsider')", owner, barber, otherBarber, outsider)
	exec("INSERT INTO shops(id,name,slug,timezone) VALUES($1,'Management',$2,'America/Sao_Paulo'),($3,'Other',$4,'UTC')", shop, slug, other, "sales-management-"+other.String())
	exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Barber'),($3,$2,'Other barber')", professional, shop, otherProfessional)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$2,'barber',$6),($7,$8,'owner',NULL)", owner, shop, barber, professional, otherBarber, otherProfessional, outsider, other)
	scope := sales.SalesScope{ActorID: owner, ShopID: shop}
	barberScope := sales.SalesScope{ActorID: barber, ShopID: shop}
	bind := func(q *db.Queries) sales.SalesTx {
		store := salespg.NewStore(q)
		catalog := catalogpg.NewSalesCatalog(q)
		return sales.SalesTx{Access: identitypg.NewSalesGuard(q), Eligibility: billingpg.NewSalesEligibility(q), Catalog: catalog, Appointments: bookingpg.NewSalesAppointments(q), Stock: catalog, Sales: store, Commands: store, Cancellation: store, Returns: catalog, Reader: store, Calendar: shopspg.NewSalesCalendar(q)}
	}
	tx := salespg.NewTransactor(pool, bind)
	register := sales.NewRegisterSale(tx, time.Now)
	cancelSale := sales.NewCancelSale(tx, time.Now)
	queries := sales.NewSaleQueries(tx)
	productTx := catalogpg.NewProductTransactor(pool, func(q *db.Queries) catalog.ProductTx {
		s := catalogpg.NewProductStore(q)
		return catalog.ProductTx{Access: identitypg.NewProductGuard(q), Products: s, Reader: s, Stock: s, Commands: s}
	})
	products := catalog.NewProductCommands(productTx, time.Now)
	product := func(quantity int64) catalog.ProductOutput {
		t.Helper()
		out, err := products.Create(ctx, catalog.CreateProductInput{Scope: catalog.ProductScope{ActorID: owner, ShopID: shop}, Name: "Pomada", PriceCents: 3000, InitialQuantity: quantity, IdempotencyKey: key()})
		if err != nil {
			t.Fatal(err)
		}
		return out.Product
	}
	input := func(ps ...catalog.ProductOutput) sales.RegisterSaleInput {
		in := sales.RegisterSaleInput{Scope: scope, IdempotencyKey: key()}
		for _, p := range ps {
			in.Items = append(in.Items, sales.SaleItemInput{ProductID: p.ID, Quantity: 1, ExpectedProductVersion: p.Version, ExpectedUnitPriceCents: p.PriceCents})
		}
		return in
	}
	record := func(in sales.RegisterSaleInput) sales.SaleOutput {
		t.Helper()
		out, err := register.Execute(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	cancellation := func(id uuid.UUID, restock bool) sales.CancelSaleInput {
		return sales.CancelSaleInput{Scope: scope, SaleID: id, ExpectedVersion: 1, Reason: "Incorrect sale", Restock: restock, IdempotencyKey: key()}
	}
	assertStock := func(id uuid.UUID, want int64) {
		t.Helper()
		var quantity, sum int64
		err := pool.QueryRow(ctx, "SELECT s.quantity_on_hand,COALESCE((SELECT sum(quantity_delta) FROM product_stock_movements m WHERE m.shop_id=s.shop_id AND m.product_id=s.product_id),0) FROM product_stock s WHERE shop_id=$1 AND product_id=$2", shop, id).Scan(&quantity, &sum)
		if err != nil || quantity != want || sum != want {
			t.Fatal("stock/ledger mismatch", quantity, sum, want, err)
		}
	}
	assertRecorded := func(id uuid.UUID) {
		t.Helper()
		out, err := queries.Get(ctx, sales.GetSaleInput{Scope: scope, SaleID: id})
		if err != nil || out.Status != domain.SaleRecorded || out.Version != 1 || out.Cancellation != nil {
			t.Fatal("cancellation escaped rollback", out, err)
		}
	}

	t.Run("owner_cancels_barber_sale_returns_exactly_once_and_preserves_original_receipt", func(t *testing.T) {
		p := product(5)
		in := input(p)
		in.Scope = barberScope
		sale := record(in)
		exec("UPDATE products SET active=FALSE,name='Renamed',price_cents=4000,version=version+1 WHERE shop_id=$1 AND id=$2", shop, p.ID)
		command := cancellation(sale.ID, true)
		command.IdempotencyKey = in.IdempotencyKey // Different operation namespace.
		first, err := cancelSale.Execute(ctx, command)
		if err != nil || first.Status != domain.SaleCancelled || first.Version != 2 || first.Replayed || first.Cancellation == nil || !first.Cancellation.Restock || first.Cancellation.ActorID != owner || first.RecordedByUserID != barber || first.TotalCents != sale.TotalCents || first.Items[0].ProductName != "Pomada" {
			t.Fatal(first, err)
		}
		second, err := cancelSale.Execute(ctx, command)
		if err != nil || !second.Replayed || !second.Cancellation.CancelledAt.Equal(first.Cancellation.CancelledAt) {
			t.Fatal("same key changed cancellation", second, err)
		}
		command.IdempotencyKey = key()
		third, err := cancelSale.Execute(ctx, command)
		if err != nil || !third.Replayed || third.Cancellation.ActorID != owner {
			t.Fatal("new key repeated transition", third, err)
		}
		replay, err := register.Execute(ctx, in)
		if err != nil || !replay.Replayed || replay.Status != domain.SaleCancelled || replay.Cancellation == nil {
			t.Fatal("record replay hid cancellation", replay, err)
		}
		assertStock(p.ID, 5)
		var count int
		execQuery := pool.QueryRow(ctx, "SELECT count(*) FROM product_stock_movements WHERE shop_id=$1 AND product_id=$2 AND kind='sale_reversal'", shop, p.ID).Scan(&count)
		if execQuery != nil || count != 1 {
			t.Fatal("duplicate return", count, execQuery)
		}
		command.Restock = false
		if _, err := cancelSale.Execute(ctx, command); !errors.Is(err, sales.ErrSaleCommandConflict) {
			t.Fatal("same-key mismatch", err)
		}
		command.IdempotencyKey = key()
		if _, err := cancelSale.Execute(ctx, command); !errors.Is(err, domain.ErrSaleAlreadyCancelled) {
			t.Fatal("different cancellation instructions accepted", err)
		}
	})
	t.Run("no_restock_is_explicit_and_does_not_change_the_ledger", func(t *testing.T) {
		p := product(2)
		sale := record(input(p))
		command := cancellation(sale.ID, false)
		out, err := cancelSale.Execute(ctx, command)
		if err != nil || out.Cancellation == nil || out.Cancellation.Restock {
			t.Fatal(out, err)
		}
		assertStock(p.ID, 1)
		if _, err := pool.Exec(ctx, "UPDATE sales SET cancellation_reason='Changed' WHERE shop_id=$1 AND id=$2", shop, sale.ID); err == nil {
			t.Fatal("audit can be rewritten")
		}
	})
	t.Run("concurrent_cancellation_keys_restore_only_one_time", func(t *testing.T) {
		p := product(2)
		sale := record(input(p))
		a, b := cancellation(sale.ID, true), cancellation(sale.ID, true)
		type result struct {
			out sales.SaleOutput
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for _, command := range []sales.CancelSaleInput{a, b} {
			go func(in sales.CancelSaleInput) {
				<-start
				out, err := cancelSale.Execute(ctx, in)
				results <- result{out, err}
			}(command)
		}
		close(start)
		first, second := <-results, <-results
		if first.err != nil || second.err != nil || first.out.Version != 2 || second.out.Version != 2 || first.out.Replayed == second.out.Replayed {
			t.Fatal(first, second)
		}
		assertStock(p.ID, 2)
	})
	t.Run("failure_after_first_return_rolls_back_header_movements_and_command", func(t *testing.T) {
		a, b := product(2), product(2)
		sale := record(input(a, b))
		command := cancellation(sale.ID, true)
		failing := salespg.NewTransactor(pool, func(q *db.Queries) sales.SalesTx {
			bound := bind(q)
			bound.Returns = failAfterFirstSaleReturn{bound.Returns}
			return bound
		})
		if _, err := sales.NewCancelSale(failing, time.Now).Execute(ctx, command); err == nil {
			t.Fatal("injected failure accepted")
		}
		assertRecorded(sale.ID)
		assertStock(a.ID, 1)
		assertStock(b.ID, 1)
		out, err := cancelSale.Execute(ctx, command)
		if err != nil || out.Replayed {
			t.Fatal("failed claim survived", out, err)
		}
		assertStock(a.ID, 2)
		assertStock(b.ID, 2)
	})
	t.Run("same_cancellation_key_concurrently_has_one_result", func(t *testing.T) {
		p := product(2)
		sale := record(input(p))
		command := cancellation(sale.ID, true)
		type result struct {
			out sales.SaleOutput
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() { <-start; out, err := cancelSale.Execute(ctx, command); results <- result{out, err} }()
		}
		close(start)
		a, b := <-results, <-results
		if a.err != nil || b.err != nil || a.out.Replayed == b.out.Replayed || !a.out.Cancellation.CancelledAt.Equal(b.out.Cancellation.CancelledAt) {
			t.Fatal(a, b)
		}
		assertStock(p.ID, 2)
	})
	t.Run("opposite_baskets_cancel_while_a_new_sale_uses_the_same_stock", func(t *testing.T) {
		a, b := product(3), product(3)
		first, second := record(input(a, b)), record(input(b, a))
		start := make(chan struct{})
		results := make(chan error, 3)
		for _, id := range []uuid.UUID{first.ID, second.ID} {
			go func(id uuid.UUID) { <-start; _, err := cancelSale.Execute(ctx, cancellation(id, true)); results <- err }(id)
		}
		newSale := input(b, a)
		go func() { <-start; _, err := register.Execute(ctx, newSale); results <- err }()
		close(start)
		for i := 0; i < 3; i++ {
			if err := <-results; err != nil {
				t.Fatal("opposite lock order failed", err)
			}
		}
		assertStock(a.ID, 2)
		assertStock(b.ID, 2)
	})
	t.Run("stock_overflow_rolls_back_the_entire_cancellation", func(t *testing.T) {
		p := product(math.MaxInt32)
		sale := record(input(p))
		exec("INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason) VALUES($1,$2,$3,$4,'receipt',1,'Restock')", uuid.New(), shop, p.ID, owner)
		if _, err := cancelSale.Execute(ctx, cancellation(sale.ID, true)); !errors.Is(err, sales.ErrSaleStockLimitExceeded) {
			t.Fatal("overflow not translated", err)
		}
		assertRecorded(sale.ID)
		assertStock(p.ID, math.MaxInt32)
	})
	t.Run("owner_only_scope_version_and_revoked_access_are_enforced", func(t *testing.T) {
		p := product(2)
		sale := record(input(p))
		command := cancellation(sale.ID, true)
		command.Scope = barberScope
		if _, err := cancelSale.Execute(ctx, command); !errors.Is(err, sales.ErrSalesAccessDenied) {
			t.Fatal("barber cancelled sale", err)
		}
		command.Scope = sales.SalesScope{ActorID: outsider, ShopID: other}
		if _, err := cancelSale.Execute(ctx, command); !errors.Is(err, sales.ErrSaleNotFound) {
			t.Fatal("foreign sale found", err)
		}
		command.Scope = scope
		command.ExpectedVersion = 2
		if _, err := cancelSale.Execute(ctx, command); !errors.Is(err, domain.ErrSaleVersionConflict) {
			t.Fatal("stale version accepted", err)
		}
		command.ExpectedVersion = 1
		if _, err := cancelSale.Execute(ctx, command); err != nil {
			t.Fatal(err)
		}
		exec("UPDATE shop_memberships SET active=FALSE WHERE shop_id=$1 AND user_id=$2", shop, owner)
		_, err := cancelSale.Execute(ctx, command)
		exec("UPDATE shop_memberships SET active=TRUE WHERE shop_id=$1 AND user_id=$2", shop, owner)
		if !errors.Is(err, sales.ErrSalesAccessDenied) {
			t.Fatal("revoked owner replayed", err)
		}
		assertStock(p.ID, 2)
	})
	t.Run("database_requires_complete_returns_exact_origin_and_immutable_audit", func(t *testing.T) {
		p := product(3)
		sale := record(input(p))
		update := "UPDATE sales SET status='cancelled',version=2,cancelled_by_user_id=$3,cancellation_reason='SQL cancellation',restock=TRUE,cancelled_at=$4 WHERE shop_id=$1 AND id=$2"
		at := time.Now().UTC().Truncate(time.Microsecond)
		if _, err := pool.Exec(ctx, update, shop, sale.ID, owner, at); err == nil {
			t.Fatal("incomplete return committed")
		}
		assertRecorded(sale.ID)
		assertStock(p.ID, 2)
		insert := "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id,reverses_movement_id,created_at) SELECT $1,shop_id,product_id,$2,'sale_reversal',$3,'SQL cancellation',sale_item_id,id,$4 FROM product_stock_movements WHERE shop_id=$5 AND sale_item_id=$6 AND kind='sale'"
		for _, bad := range []struct {
			actor uuid.UUID
			qty   int64
		}{{owner, 2}, {barber, 1}} {
			transaction, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = transaction.Exec(ctx, update, shop, sale.ID, owner, at); err != nil {
				t.Fatal(err)
			}
			_, err = transaction.Exec(ctx, insert, uuid.New(), bad.actor, bad.qty, at, shop, sale.Items[0].ID)
			_ = transaction.Rollback(ctx)
			if err == nil {
				t.Fatal("invalid return accepted", bad)
			}
		}
		if _, err := cancelSale.Execute(ctx, cancellation(sale.ID, true)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id,reverses_movement_id,created_at) SELECT $1,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id,reverses_movement_id,created_at FROM product_stock_movements WHERE shop_id=$2 AND sale_item_id=$3 AND kind='sale_reversal'", uuid.New(), shop, sale.Items[0].ID); err == nil {
			t.Fatal("duplicate return accepted")
		}
		if _, err := pool.Exec(ctx, "UPDATE sales SET total_cents=1 WHERE shop_id=$1 AND id=$2", shop, sale.ID); err == nil {
			t.Fatal("cancelled total changed")
		}
		if _, err := pool.Exec(ctx, "DELETE FROM sales WHERE shop_id=$1 AND id=$2", shop, sale.ID); err == nil {
			t.Fatal("cancelled history deleted")
		}
		assertStock(p.ID, 3)
	})
	t.Run("civil_period_keyset_pagination_and_professional_scope", func(t *testing.T) {
		dateUser, dateProfessional := uuid.New(), uuid.New()
		exec("INSERT INTO users(id,display_name) VALUES($1,'Date fixture')", dateUser)
		exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Date fixture')", dateProfessional, shop)
		exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'barber',$3)", dateUser, shop, dateProfessional)
		dateScope := sales.SalesScope{ActorID: dateUser, ShopID: shop}
		p := product(10)
		var saved []sales.SaleOutput
		for _, instant := range []string{"2026-10-30T02:59:59Z", "2026-10-30T03:00:00Z", "2026-10-31T02:59:59Z", "2026-10-31T03:00:00Z"} {
			at, _ := time.Parse(time.RFC3339, instant)
			in := input(p)
			in.Scope = dateScope
			out, err := sales.NewRegisterSale(tx, func() time.Time { return at }).Execute(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			saved = append(saved, out)
		}
		filter := sales.ListSalesInput{Scope: dateScope, From: "2026-10-30", Through: "2026-10-30", PageSize: 1}
		first, err := queries.List(ctx, filter)
		if err != nil || first.Timezone != "America/Sao_Paulo" || len(first.Items) != 1 || first.Items[0].ID != saved[2].ID || first.NextCursor == "" {
			t.Fatal(first, err)
		}
		filter.Cursor = first.NextCursor
		second, err := queries.List(ctx, filter)
		if err != nil || len(second.Items) != 1 || second.Items[0].ID != saved[1].ID || second.NextCursor != "" {
			t.Fatal(second, err)
		}
		filter.Scope = scope
		if _, err := queries.List(ctx, filter); !errors.Is(err, sales.ErrInvalidSalesQuery) {
			t.Fatal("cursor reused by different actor", err)
		}
		filter.Cursor = ""
		filter.ProfessionalID = dateProfessional
		if page, err := queries.List(ctx, filter); err != nil || len(page.Items) != 1 {
			t.Fatal(page, err)
		}
		filter.Scope = barberScope
		filter.ProfessionalID = otherProfessional
		if _, err := queries.List(ctx, filter); !errors.Is(err, sales.ErrSalesAccessDenied) {
			t.Fatal("barber filtered another professional", err)
		}
		if _, err := queries.Get(ctx, sales.GetSaleInput{Scope: sales.SalesScope{ActorID: otherBarber, ShopID: shop}, SaleID: saved[1].ID}); !errors.Is(err, sales.ErrSaleNotFound) {
			t.Fatal("barber read other sale", err)
		}
		if _, err := queries.Get(ctx, sales.GetSaleInput{Scope: scope, SaleID: saved[1].ID}); err != nil {
			t.Fatal("owner cannot read barber sale", err)
		}
		fixedCancellation := sales.NewCancelSale(tx, func() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) })
		if _, err := fixedCancellation.Execute(ctx, cancellation(saved[1].ID, false)); err != nil {
			t.Fatal(err)
		}
		filter = sales.ListSalesInput{Scope: scope, ProfessionalID: dateProfessional, From: "2026-10-30", Through: "2026-10-30", Status: domain.SaleCancelled}
		page, err := queries.List(ctx, filter)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != saved[1].ID || page.Items[0].Cancellation == nil {
			t.Fatal(page, err)
		}
		for _, change := range []func(*sales.ListSalesInput){func(in *sales.ListSalesInput) { in.From = "2026-02-30" }, func(in *sales.ListSalesInput) { in.Through = "2026-10-29" }, func(in *sales.ListSalesInput) { in.Through = "2027-10-30" }, func(in *sales.ListSalesInput) { in.PageSize = 101 }, func(in *sales.ListSalesInput) { in.Status = "unknown" }, func(in *sales.ListSalesInput) { in.Cursor = "invalid" }} {
			bad := filter
			change(&bad)
			if _, err := queries.List(ctx, bad); !errors.Is(err, sales.ErrInvalidSalesQuery) {
				t.Fatal("invalid filters accepted", bad, err)
			}
		}
	})
	t.Run("expired_subscription_and_inactive_attribution_preserve_owner_management", func(t *testing.T) {
		p := product(2)
		in := input(p)
		in.ProfessionalID = professional
		sale := record(in)
		exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", shop)
		exec("UPDATE professionals SET active=FALSE WHERE shop_id=$1 AND id=$2", shop, professional)
		defer exec("UPDATE professionals SET active=TRUE WHERE shop_id=$1 AND id=$2", shop, professional)
		defer exec("UPDATE shops SET provisioning_source='manual' WHERE id=$1", shop)
		if _, err := queries.Get(ctx, sales.GetSaleInput{Scope: scope, SaleID: sale.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := cancelSale.Execute(ctx, cancellation(sale.ID, true)); err != nil {
			t.Fatal(err)
		}
		replay, err := register.Execute(ctx, in)
		if err != nil || replay.Status != domain.SaleCancelled {
			t.Fatal(replay, err)
		}
		assertStock(p.ID, 2)
	})
	t.Run("HTTP_reads_and_cancellation_validate_JSON_roles_CORS_and_cache", func(t *testing.T) {
		p := product(5)
		in := input(p)
		in.Scope = barberScope
		sale := record(in)
		q := db.New(pool)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := httpapi.NewHandler(httpapi.HandlerConfig{Sales: saleshttp.NewHandler(register, cancelSale, queries, 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
		base := "/api/v1/admin/shops/" + slug + "/sales"
		path := base + "/" + sale.ID.String()
		request := func(method, path string, actor uuid.UUID, body, key string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", key)
			if actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+actor.String())
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		for _, tc := range []struct {
			method, path string
			actor        uuid.UUID
			body         string
			want         int
		}{
			{"GET", path, uuid.Nil, "", 401}, {"GET", path, otherBarber, "", 404}, {"GET", path, barber, "", 200}, {"GET", path, owner, "", 200}, {"GET", base + "?from=2026-10-30&through=2026-10-30", owner, "", 200}, {"GET", base + "?from=2026-10-30&from=2026-10-31&through=2026-10-31", owner, "", 400}, {"GET", base + "?from=2026-10-30&through=2026-10-30&typo=1", owner, "", 400},
			{"POST", path + "/cancel", barber, `{"expected_version":1,"reason":"Wrong sale","restock":true}`, 403}, {"POST", path + "/cancel", owner, `null`, 400}, {"POST", path + "/cancel", owner, `{"expected_version":1,"reason":"Wrong sale"}`, 400}, {"POST", path + "/cancel", owner, `{"expected_version":1,"reason":"Wrong sale","restock":null}`, 400}, {"POST", path + "/cancel", owner, `{"expected_version":1,"reason":"Wrong sale","restock":true,"quantity":1}`, 400}, {"POST", path + "/cancel", owner, `{"expected_version":1,"reason":"Wrong sale","restock":true} {}`, 400},
		} {
			w := request(tc.method, tc.path, tc.actor, tc.body, key())
			if w.Code != tc.want {
				t.Fatal(tc.method, tc.path, w.Code, tc.want, w.Body.String())
			}
		}
		commandKey := key()
		body := `{"expected_version":1,"reason":"Wrong sale","restock":true}`
		first := request("POST", path+"/cancel", owner, body, commandKey)
		if first.Code != 200 {
			t.Fatal(first.Code, first.Body.String())
		}
		second := request("POST", path+"/cancel", owner, body, commandKey)
		var out map[string]any
		if second.Code != 200 || json.Unmarshal(second.Body.Bytes(), &out) != nil || out["replayed"] != true || out["status"] != "cancelled" || out["cancellation"] == nil {
			t.Fatal(second.Code, second.Body.String())
		}
		if !strings.Contains(first.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("cancellation cached")
		}
		for _, tc := range []struct{ path, method string }{{base, "GET"}, {path, "GET"}, {path + "/cancel", "POST"}} {
			r := httptest.NewRequest("OPTIONS", tc.path, nil)
			r.Header.Set("Origin", "http://127.0.0.1:3000")
			r.Header.Set("Access-Control-Request-Method", tc.method)
			r.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 204 {
				t.Fatal("preflight rejected", tc, w.Code)
			}
		}
		assertStock(p.ID, 5)
	})
}
