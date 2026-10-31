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
	billingpg "github.com/Felipe-candido/Barber-chat/internal/modules/billing/infra/postgres"
	bookingpg "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	saleshttp "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/http"
	salespg "github.com/Felipe-candido/Barber-chat/internal/modules/sales/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failAfterFirstSaleStock struct{ sales.SaleStockRecorder }

type advanceAfterSaleStock struct {
	sales.SaleStockRecorder
	after func()
}

func (s advanceAfterSaleStock) RecordSaleStock(ctx context.Context, c sales.SaleStockCommand) error {
	if err := s.SaleStockRecorder.RecordSaleStock(ctx, c); err != nil {
		return err
	}
	s.after()
	return nil
}

func (s failAfterFirstSaleStock) RecordSaleStock(ctx context.Context, c sales.SaleStockCommand) error {
	c.Items = c.Items[:1]
	if err := s.SaleStockRecorder.RecordSaleStock(ctx, c); err != nil {
		return err
	}
	return errors.New("injected failure after first stock movement")
}

func TestRecordProductSale(t *testing.T) {
	dsn := requiredEnv(t, "SALES_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_sales_") {
		t.Fatal("requires disposable codex_sales_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	owner, barber, otherBarber, outsider := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	shop, other, professional, otherProfessional := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	slug := "sales-" + shop.String()
	exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber A'),($3,'Barber B'),($4,'Outsider')", owner, barber, otherBarber, outsider)
	exec("INSERT INTO shops(id,name,slug) VALUES($1,'Sales',$2),($3,'Other',$4)", shop, slug, other, "sales-"+other.String())
	exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Barber A'),($3,$2,'Barber B')", professional, shop, otherProfessional)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$2,'barber',$6),($7,$8,'owner',NULL)", owner, shop, barber, professional, otherBarber, otherProfessional, outsider, other)
	key := func() string { return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "") }
	bind := func(q *db.Queries) sales.SalesTx {
		store := salespg.NewStore(q)
		catalog := catalogpg.NewSalesCatalog(q)
		return sales.SalesTx{Access: identitypg.NewSalesGuard(q), Eligibility: billingpg.NewSalesEligibility(q), Catalog: catalog, Appointments: bookingpg.NewSalesAppointments(q), Stock: catalog, Sales: store, Commands: store, Cancellation: store, Returns: catalog, Reader: store, Calendar: shopspg.NewSalesCalendar(q)}
	}
	tx := salespg.NewTransactor(pool, bind)
	register := sales.NewRegisterSale(tx, time.Now)
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
		in := sales.RegisterSaleInput{Scope: sales.SalesScope{ActorID: owner, ShopID: shop}, IdempotencyKey: key()}
		for _, p := range ps {
			in.Items = append(in.Items, sales.SaleItemInput{ProductID: p.ID, Quantity: 1, ExpectedProductVersion: p.Version, ExpectedUnitPriceCents: p.PriceCents})
		}
		return in
	}
	assertStock := func(id uuid.UUID, want int64) {
		t.Helper()
		var quantity, sum int64
		err := pool.QueryRow(ctx, "SELECT s.quantity_on_hand,COALESCE((SELECT sum(quantity_delta) FROM product_stock_movements m WHERE m.shop_id=s.shop_id AND m.product_id=s.product_id),0) FROM product_stock s WHERE shop_id=$1 AND product_id=$2", shop, id).Scan(&quantity, &sum)
		if err != nil || quantity != want || sum != want {
			t.Fatal("stock ledger mismatch", quantity, sum, want, err)
		}
	}
	assertNoSales := func(id uuid.UUID) {
		t.Helper()
		var count int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM sale_items WHERE shop_id=$1 AND product_id=$2", shop, id).Scan(&count)
		if err != nil || count != 0 {
			t.Fatal("partial sale survived", count, err)
		}
	}
	fundCoverage := func(until time.Time) {
		t.Helper()
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM billing_accounts WHERE shop_id=$1)", shop).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			exec("UPDATE billing_subscriptions SET status='active',paid_until=$2 WHERE shop_id=$1", shop, until)
			exec("UPDATE billing_paid_invoices SET paid_until=$2 WHERE shop_id=$1", shop, until)
			return
		}
		customer := "cus_" + strings.ReplaceAll(shop.String(), "-", "")
		subscription := "sub_" + strings.ReplaceAll(shop.String(), "-", "")
		attempt := uuid.New()
		exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id,stripe_mode) VALUES($1,$2,'test')", shop, customer)
		exec("INSERT INTO billing_checkout_attempts(id,shop_id,intent,plan_key,state,protocol_version,stripe_price_id,return_url) VALUES($1,$2,'publish','standard_monthly','review',1,'price_sales','http://127.0.0.1:3000')", attempt, shop)
		exec("INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until,reconciliation_protocol,checkout_attempt_id,stripe_price_id) VALUES($1,$2,$3,$4,'standard_monthly','active',now()-interval '1 day',$6,1,$5,'price_sales')", uuid.New(), shop, customer, subscription, attempt, until)
		exec("INSERT INTO billing_paid_invoices(stripe_invoice_id,shop_id,stripe_subscription_id,stripe_price_id,paid_from,paid_until,amount_paid) VALUES($1,$2,$3,'price_sales',now()-interval '1 day',$4,3990)", "in_"+strings.ReplaceAll(uuid.NewString(), "-", ""), shop, subscription, until)
	}
	t.Run("sale receipt snapshots and reordered replay do not move stock again", func(t *testing.T) {
		p1, p2 := product(3), product(2)
		in := input(p1, p2)
		in.Items[0].Quantity = 2
		out, err := register.Execute(ctx, in)
		if err != nil || out.TotalCents != 9000 || out.ProfessionalID != uuid.Nil || out.Replayed || len(out.Items) != 2 {
			t.Fatal("sale failed", err)
		}
		assertStock(p1.ID, 1)
		assertStock(p2.ID, 1)
		exec("UPDATE products SET name='Changed',price_cents=4000,version=version+1,active=false,updated_at=now() WHERE id=$1", p1.ID)
		in.Items[0], in.Items[1] = in.Items[1], in.Items[0]
		replay, err := register.Execute(ctx, in)
		if err != nil || !replay.Replayed || replay.ID != out.ID || replay.TotalCents != 9000 || !replay.SoldAt.Equal(out.SoldAt) {
			t.Fatal("receipt not recovered", err)
		}
		for _, item := range replay.Items {
			if item.ProductID == p1.ID && (item.ProductName != "Pomada" || item.UnitPriceCents != 3000) {
				t.Fatal("catalog rewrote snapshot")
			}
		}
		assertStock(p1.ID, 1)
		assertStock(p2.ID, 1)
		in.Items[0].Quantity++
		if _, err := register.Execute(ctx, in); !errors.Is(err, sales.ErrSaleCommandConflict) {
			t.Fatal("changed input reused key", err)
		}
	})
	t.Run("two sales cannot consume the last unit", func(t *testing.T) {
		p := product(1)
		start := make(chan struct{})
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			in := input(p)
			go func() { <-start; _, err := register.Execute(ctx, in); results <- err }()
		}
		close(start)
		success, denied := 0, 0
		for i := 0; i < 2; i++ {
			err := <-results
			if err == nil {
				success++
			} else if errors.Is(err, sales.ErrSaleStockUnavailable) {
				denied++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || denied != 1 {
			t.Fatal("last unit consumed twice")
		}
		assertStock(p.ID, 0)
	})
	t.Run("same command concurrent with itself records exactly one sale", func(t *testing.T) {
		p := product(2)
		in := input(p)
		type result struct {
			out sales.SaleOutput
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() { <-start; out, err := register.Execute(ctx, in); results <- result{out, err} }()
		}
		close(start)
		a, b := <-results, <-results
		if a.err != nil || b.err != nil || a.out.ID != b.out.ID || a.out.Replayed == b.out.Replayed {
			t.Fatal("duplicate sale", a.err, b.err)
		}
		assertStock(p.ID, 1)
	})
	t.Run("opposite item order avoids deadlock and failed stock rolls back all items", func(t *testing.T) {
		p1, p2 := product(2), product(2)
		a, b := input(p1, p2), input(p2, p1)
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, in := range []sales.RegisterSaleInput{a, b} {
			go func() { <-start; _, err := register.Execute(ctx, in); results <- err }()
		}
		close(start)
		for i := 0; i < 2; i++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		assertStock(p1.ID, 0)
		assertStock(p2.ID, 0)
		p3, p4 := product(5), product(0)
		if _, err := register.Execute(ctx, input(p3, p4)); !errors.Is(err, sales.ErrSaleStockUnavailable) {
			t.Fatal("insufficient basket accepted", err)
		}
		assertStock(p3.ID, 5)
		assertNoSales(p3.ID)
		assertNoSales(p4.ID)
	})
	t.Run("failure after first withdrawal rolls back receipt items and command", func(t *testing.T) {
		p1, p2 := product(2), product(2)
		in := input(p1, p2)
		faultTx := salespg.NewTransactor(pool, func(q *db.Queries) sales.SalesTx {
			bound := bind(q)
			bound.Stock = failAfterFirstSaleStock{bound.Stock}
			return bound
		})
		if _, err := sales.NewRegisterSale(faultTx, time.Now).Execute(ctx, in); err == nil {
			t.Fatal("injected failure ignored")
		}
		assertStock(p1.ID, 2)
		assertStock(p2.ID, 2)
		assertNoSales(p1.ID)
		assertNoSales(p2.ID)
		if out, err := register.Execute(ctx, in); err != nil || out.Replayed {
			t.Fatal("rolled-back command was retained", err)
		}
	})
	t.Run("barber attribution owner attribution prices and revocation", func(t *testing.T) {
		p := product(5)
		in := input(p)
		in.Scope.ActorID = barber
		out, err := register.Execute(ctx, in)
		if err != nil || out.ProfessionalID != professional || out.RecordedByUserID != barber {
			t.Fatal("barber not attributed", err)
		}
		foreign := input(p)
		foreign.Scope.ActorID = barber
		foreign.ProfessionalID = otherProfessional
		if _, err := register.Execute(ctx, foreign); !errors.Is(err, sales.ErrSalesAccessDenied) {
			t.Fatal("barber chose other professional", err)
		}
		ownerSale := input(p)
		ownerSale.ProfessionalID = otherProfessional
		if out, err := register.Execute(ctx, ownerSale); err != nil || out.ProfessionalID != otherProfessional || out.RecordedByUserID != owner {
			t.Fatal("owner attribution failed", err)
		}
		changed := input(p)
		changed.Items[0].ExpectedUnitPriceCents--
		if _, err := register.Execute(ctx, changed); !errors.Is(err, sales.ErrSalePriceChanged) {
			t.Fatal("client supplied effective price", err)
		}
		exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", barber, shop)
		defer exec("UPDATE shop_memberships SET active=true WHERE user_id=$1 AND shop_id=$2", barber, shop)
		if _, err := register.Execute(ctx, in); !errors.Is(err, sales.ErrSalesAccessDenied) {
			t.Fatal("revoked barber recovered receipt", err)
		}
		assertStock(p.ID, 3)
	})
	t.Run("foreign products and invalid baskets never create a sale", func(t *testing.T) {
		p := product(2)
		for _, change := range []func(*sales.RegisterSaleInput){
			func(in *sales.RegisterSaleInput) { in.Items = append(in.Items, in.Items[0]) },
			func(in *sales.RegisterSaleInput) { in.Items[0].Quantity = 0 },
		} {
			in := input(p)
			change(&in)
			if _, err := register.Execute(ctx, in); !errors.Is(err, sales.ErrInvalidSalesInput) {
				t.Fatal("invalid basket accepted", err)
			}
		}
		in := input(p)
		in.Scope = sales.SalesScope{ActorID: outsider, ShopID: other}
		if _, err := register.Execute(ctx, in); !errors.Is(err, sales.ErrSaleProductUnavailable) {
			t.Fatal("foreign product resolved", err)
		}
		assertStock(p.ID, 2)
		assertNoSales(p.ID)
	})
	t.Run("database refuses incomplete receipts duplicate sources and history edits", func(t *testing.T) {
		p := product(2)
		_, err := pool.Exec(ctx, "INSERT INTO sales(id,shop_id,recorded_by_user_id,total_cents,sold_at,created_at) VALUES($1,$2,$3,3000,now(),now())", uuid.New(), shop, owner)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "recorded_sale_complete" {
			t.Fatal("partial SQL header committed", err)
		}
		for _, withMovement := range []bool{false, true} {
			transaction, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saleID, itemID := uuid.New(), uuid.New()
			total := int64(3000)
			if withMovement {
				total = 5000
			}
			_, err = transaction.Exec(ctx, "INSERT INTO sales(id,shop_id,recorded_by_user_id,total_cents,sold_at,created_at) VALUES($1,$2,$3,$4,now(),now())", saleID, shop, owner, total)
			if err == nil {
				_, err = transaction.Exec(ctx, "INSERT INTO sale_items(id,shop_id,sale_id,product_id,product_name_snapshot,quantity,unit_price_cents) VALUES($1,$2,$3,$4,'Pomada',1,3000)", itemID, shop, saleID, p.ID)
			}
			if err == nil && withMovement {
				_, err = transaction.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id) VALUES($1,$2,$3,$4,'sale',-1,'Mismatched total test',$5)", uuid.New(), shop, p.ID, owner, itemID)
			}
			if err != nil {
				_ = transaction.Rollback(ctx)
				t.Fatal(err)
			}
			err = transaction.Commit(ctx)
			_ = transaction.Rollback(ctx)
			if !errors.As(err, &pgErr) || pgErr.ConstraintName != "recorded_sale_complete" {
				t.Fatal("SQL accepted missing stock or wrong total", err)
			}
			assertStock(p.ID, 2)
			assertNoSales(p.ID)
		}
		out, err := register.Execute(ctx, input(p))
		if err != nil {
			t.Fatal(err)
		}
		for _, sql := range []string{"UPDATE sales SET total_cents=1 WHERE id=$1", "DELETE FROM sales WHERE id=$1", "DELETE FROM sale_items WHERE sale_id=$1"} {
			if _, err := pool.Exec(ctx, sql, out.ID); err == nil {
				t.Fatal("immutable sale edited")
			}
		}
		_, err = pool.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,sale_item_id) VALUES($1,$2,$3,$4,'sale',-1,'Duplicate source',$5)", uuid.New(), shop, p.ID, owner, out.Items[0].ID)
		if err == nil {
			t.Fatal("sale item debited twice")
		}
		assertStock(p.ID, 1)
	})
	t.Run("real billing projection denies unpaid new sales but preserves saved replay", func(t *testing.T) {
		p := product(3)
		in := input(p)
		out, err := register.Execute(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", shop)
		defer exec("UPDATE shops SET provisioning_source='manual' WHERE id=$1", shop)
		if replay, err := register.Execute(ctx, in); err != nil || replay.ID != out.ID || !replay.Replayed {
			t.Fatal("saved sale blocked by coverage", err)
		}
		if _, err := register.Execute(ctx, input(p)); !errors.Is(err, sales.ErrSaleSubscriptionRequired) {
			t.Fatal("unpaid sale allowed", err)
		}
		assertStock(p.ID, 2)
		fundCoverage(time.Now().Add(24 * time.Hour))
		if _, err := register.Execute(ctx, input(p)); err != nil {
			t.Fatal("paid unpublished unit cannot sell", err)
		}
		assertStock(p.ID, 1)
	})
	t.Run("coverage expiring during writes rolls back sale stock and command", func(t *testing.T) {
		p := product(2)
		now := time.Now().UTC()
		deadline := now.Add(time.Minute)
		fundCoverage(deadline)
		exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", shop)
		defer exec("UPDATE shops SET provisioning_source='manual' WHERE id=$1", shop)
		testTx := salespg.NewTransactor(pool, func(q *db.Queries) sales.SalesTx {
			bound := bind(q)
			bound.Stock = advanceAfterSaleStock{bound.Stock, func() { now = deadline.Add(time.Minute) }}
			return bound
		})
		out, err := sales.NewRegisterSale(testTx, func() time.Time { return now }).Execute(ctx, input(p))
		if !errors.Is(err, sales.ErrSaleSubscriptionRequired) || out.ID != uuid.Nil {
			t.Fatal("expired coverage committed", err)
		}
		assertStock(p.ID, 2)
		assertNoSales(p.ID)
	})
	t.Run("appointment binding preserves its lifecycle and validates attribution", func(t *testing.T) {
		p := product(3)
		service, customer, appointment := uuid.New(), uuid.New(), uuid.New()
		exec("INSERT INTO services(id,shop_id,name,duration_minutes,price_cents) VALUES($1,$2,'Cut',30,5000)", service, shop)
		exec("INSERT INTO customers(id,shop_id,first_name,last_name,phone_e164,created_at) VALUES($1,$2,'Test','Contact','+5511999991111',now())", customer, shop)
		exec("INSERT INTO appointments(id,shop_id,customer_id,service_id,professional_id,service_name,duration_minutes,price_cents,currency,professional_name,shop_timezone,starts_at,ends_at,status,version,code_hash,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'Cut',30,5000,'BRL','Barber A','America/Sao_Paulo',now()+interval '1 day',now()+interval '1 day 1 hour','scheduled',1,$6,now(),now())", appointment, shop, customer, service, professional, make([]byte, 32))
		in := input(p)
		in.AppointmentID = appointment
		out, err := register.Execute(ctx, in)
		if err != nil || out.AppointmentID != appointment || out.ProfessionalID != professional {
			t.Fatal("appointment not bound", err)
		}
		var status string
		_ = pool.QueryRow(ctx, "SELECT status FROM appointments WHERE id=$1", appointment).Scan(&status)
		if status != "scheduled" {
			t.Fatal("product sale completed an appointment")
		}
		other := input(p)
		other.Scope.ActorID = otherBarber
		other.AppointmentID = appointment
		if _, err := register.Execute(ctx, other); !errors.Is(err, sales.ErrSalesAccessDenied) {
			t.Fatal("barber used another appointment", err)
		}
		exec("UPDATE appointments SET status='cancelled',version=version+1,status_source='staff',status_actor_id=$2,status_reason='Fixture cancellation',updated_at=clock_timestamp() WHERE id=$1", appointment, owner)
		fresh := input(p)
		fresh.AppointmentID = appointment
		if _, err := register.Execute(ctx, fresh); !errors.Is(err, sales.ErrSaleAppointmentUnavailable) {
			t.Fatal("cancelled appointment accepted", err)
		}
		exec("UPDATE professionals SET active=false WHERE id=$1", professional)
		defer exec("UPDATE professionals SET active=true WHERE id=$1", professional)
		if replay, err := register.Execute(ctx, in); err != nil || replay.ID != out.ID || !replay.Replayed {
			t.Fatal("owner cannot recover old receipt", err)
		}
		assertStock(p.ID, 2)
	})
	t.Run("HTTP owner and barber register with strict JSON and no automatic duplication", func(t *testing.T) {
		p := product(5)
		q := db.New(pool)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := httpapi.NewHandler(httpapi.HandlerConfig{Sales: saleshttp.NewHandler(register, sales.NewCancelSale(tx, time.Now), sales.NewSaleQueries(tx), 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
		path := "/api/v1/admin/shops/" + slug + "/sales"
		body, _ := json.Marshal(map[string]any{"items": []map[string]any{{"product_id": p.ID, "quantity": 1, "expected_product_version": 1, "expected_unit_price_cents": 3000}}})
		request := func(actor uuid.UUID, raw, key string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("POST", path, strings.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			if actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+actor.String())
			}
			r.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		for _, tc := range []struct {
			actor     uuid.UUID
			body, key string
			want      int
		}{{uuid.Nil, string(body), key(), 401}, {outsider, string(body), key(), 403}, {owner, "null", key(), 400}, {owner, `{"items":[],"total_cents":1}`, key(), 400}, {owner, string(body) + " {}", key(), 400}, {owner, string(body), "", 400}, {owner, `{"items":[]}`, key(), 422}} {
			if w := request(tc.actor, tc.body, tc.key); w.Code != tc.want {
				t.Fatal("HTTP validation", w.Code, tc.want, w.Body.String())
			}
		}
		commandKey := key()
		first := request(owner, string(body), commandKey)
		if first.Code != 201 {
			t.Fatal(first.Code, first.Body.String())
		}
		second := request(owner, string(body), commandKey)
		if second.Code != 200 {
			t.Fatal(second.Code, second.Body.String())
		}
		var a, b map[string]any
		_ = json.Unmarshal(first.Body.Bytes(), &a)
		_ = json.Unmarshal(second.Body.Bytes(), &b)
		if a["id"] != b["id"] || b["replayed"] != true {
			t.Fatal("HTTP receipt changed")
		}
		if w := request(barber, string(body), key()); w.Code != 201 {
			t.Fatal("barber HTTP sale", w.Code, w.Body.String())
		}
		assertStock(p.ID, 3)
		if !strings.Contains(first.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("sale receipt cached")
		}
		r := httptest.NewRequest("OPTIONS", path, nil)
		r.Header.Set("Origin", "http://127.0.0.1:3000")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal("sale preflight rejected", w.Code)
		}
		exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", shop)
		defer exec("UPDATE shops SET provisioning_source='manual' WHERE id=$1", shop)
		exec("UPDATE billing_subscriptions SET status='canceled' WHERE shop_id=$1", shop)
		if w := request(owner, string(body), key()); w.Code != 402 {
			t.Fatal("owner subscription denial", w.Code, w.Body.String())
		}
		if w := request(barber, string(body), key()); w.Code != 403 {
			t.Fatal("barber subscription denial", w.Code, w.Body.String())
		}
		assertStock(p.ID, 3)
	})
}
