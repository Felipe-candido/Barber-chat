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
	app "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypg "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspg "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failAfterProductMovement struct{ app.ProductStockStore }

func (s failAfterProductMovement) AppendStockMovement(ctx context.Context, m domain.StockMovement) error {
	if err := s.ProductStockStore.AppendStockMovement(ctx, m); err != nil {
		return err
	}
	return errors.New("injected failure after stock trigger")
}

func TestProductsAndStock(t *testing.T) {
	dsn := requiredEnv(t, "PRODUCTS_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "codex_products_") {
		t.Fatal("requires a disposable codex_products_* database")
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
		t.Fatal("refusing database with Auth schema", err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	owner, barber, outsider, shop, other, professional := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	slug := "products-" + shop.String()
	exec("INSERT INTO users(id,display_name) VALUES($1,'Owner'),($2,'Barber'),($3,'Outsider')", owner, barber, outsider)
	exec("INSERT INTO shops(id,name,slug) VALUES($1,'Products',$2),($3,'Other',$4)", shop, slug, other, "products-"+other.String())
	exec("INSERT INTO professionals(id,shop_id,name) VALUES($1,$2,'Barber')", professional, shop)
	exec("INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES($1,$2,'owner',NULL),($3,$2,'barber',$4),($5,$6,'owner',NULL)", owner, shop, barber, professional, outsider, other)
	scope := app.ProductScope{ActorID: owner, ShopID: shop}
	key := func() string { return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "") }
	bind := func(q *db.Queries) app.ProductTx {
		store := catalogpg.NewProductStore(q)
		return app.ProductTx{Access: identitypg.NewProductGuard(q), Products: store, Stock: store, Commands: store, Reader: store}
	}
	tx := catalogpg.NewProductTransactor(pool, bind)
	commands := app.NewProductCommands(tx, time.Now)
	queries := app.NewProductQueries(tx)
	fixture := func(quantity int64) app.ProductMutationOutput {
		t.Helper()
		result, err := commands.Create(ctx, app.CreateProductInput{Scope: scope, Name: "Pomada", PriceCents: 3000, InitialQuantity: quantity, MinimumQuantity: 1, IdempotencyKey: key()})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	stockInput := func(id uuid.UUID, delta int64) app.RecordStockInput {
		return app.RecordStockInput{Scope: scope, ProductID: id, Operation: app.StockAdjust, QuantityDelta: delta, Reason: "Count correction", IdempotencyKey: key()}
	}
	assertBalance := func(id uuid.UUID, want int64) {
		t.Helper()
		var quantity, sum int64
		err := pool.QueryRow(ctx, "SELECT s.quantity_on_hand,COALESCE((SELECT sum(quantity_delta) FROM product_stock_movements m WHERE m.shop_id=s.shop_id AND m.product_id=s.product_id),0) FROM product_stock s WHERE shop_id=$1 AND product_id=$2", shop, id).Scan(&quantity, &sum)
		if err != nil || quantity != want || sum != want {
			t.Fatal("balance/ledger mismatch", quantity, sum, want, err)
		}
	}
	t.Run("create replay receipt and adjustment commit exactly once", func(t *testing.T) {
		input := app.CreateProductInput{Scope: scope, Name: "  Shampoo  ", Description: "  Neutral  ", PriceCents: 4000, InitialQuantity: 2, MinimumQuantity: 1, IdempotencyKey: key()}
		first, err := commands.Create(ctx, input)
		if err != nil || first.Replayed || first.MovementID == uuid.Nil {
			t.Fatal(err)
		}
		input.Name, input.Description = "Shampoo", "Neutral"
		again, err := commands.Create(ctx, input)
		if err != nil || !again.Replayed || again.Product.ID != first.Product.ID || again.MovementID != first.MovementID {
			t.Fatal("create not recovered", err)
		}
		assertBalance(first.Product.ID, 2)
		input.PriceCents++
		if _, err := commands.Create(ctx, input); !errors.Is(err, app.ErrProductCommandConflict) {
			t.Fatal("changed body reused key", err)
		}
		receipt := stockInput(first.Product.ID, 3)
		receipt.Operation = app.StockReceive
		out, err := commands.RecordStock(ctx, receipt)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			repeat, err := commands.RecordStock(ctx, receipt)
			if err != nil || !repeat.Replayed || repeat.MovementID != out.MovementID {
				t.Fatal("stock replay mutated", err)
			}
		}
		assertBalance(first.Product.ID, 5)
		withdraw := stockInput(first.Product.ID, -5)
		out, err = commands.RecordStock(ctx, withdraw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := commands.RecordStock(ctx, withdraw); err != nil {
			t.Fatal("replay checked empty stock before result", err)
		}
		assertBalance(first.Product.ID, 0)
		if _, err := commands.RecordStock(ctx, stockInput(first.Product.ID, -1)); !errors.Is(err, domain.ErrInsufficientStock) {
			t.Fatal("negative stock accepted", err)
		}
		assertBalance(first.Product.ID, 0)
	})
	t.Run("simultaneous identical commands return one product and opening", func(t *testing.T) {
		input := app.CreateProductInput{Scope: scope, Name: "Concurrent", PriceCents: 2000, InitialQuantity: 1, IdempotencyKey: key()}
		type result struct {
			out app.ProductMutationOutput
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() { <-start; out, err := commands.Create(ctx, input); results <- result{out, err} }()
		}
		close(start)
		a, b := <-results, <-results
		if a.err != nil || b.err != nil || a.out.Product.ID != b.out.Product.ID || a.out.Replayed == b.out.Replayed {
			t.Fatal("duplicate concurrent create", a.err, b.err)
		}
		assertBalance(a.out.Product.ID, 1)
	})
	t.Run("two withdrawals compete for the last unit", func(t *testing.T) {
		p := fixture(1)
		results := make(chan error, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			input := stockInput(p.Product.ID, -1)
			go func() { <-start; _, err := commands.RecordStock(ctx, input); results <- err }()
		}
		close(start)
		success, conflicts := 0, 0
		for i := 0; i < 2; i++ {
			err := <-results
			if err == nil {
				success++
			} else if errors.Is(err, domain.ErrInsufficientStock) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflicts != 1 {
			t.Fatal("last unit consumed twice")
		}
		assertBalance(p.Product.ID, 0)
	})
	t.Run("SQL trigger independently protects last unit and ignored duplicate", func(t *testing.T) {
		p := fixture(1)
		results := make(chan error, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() {
				<-start
				_, err := pool.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason) VALUES($1,$2,$3,$4,'adjustment',-1,'Direct SQL test')", uuid.New(), shop, p.Product.ID, owner)
				results <- err
			}()
		}
		close(start)
		success, conflicts := 0, 0
		for i := 0; i < 2; i++ {
			err := <-results
			var pgErr *pgconn.PgError
			if err == nil {
				success++
			} else if errors.As(err, &pgErr) && pgErr.ConstraintName == "product_stock_available" {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflicts != 1 {
			t.Fatal("database last unit race failed")
		}
		assertBalance(p.Product.ID, 0)
		exec("INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason,created_at) SELECT id,shop_id,product_id,actor_id,kind,quantity_delta,reason,created_at FROM product_stock_movements WHERE id=$1 ON CONFLICT DO NOTHING", p.MovementID)
		assertBalance(p.Product.ID, 0)
	})
	t.Run("failure after trigger rolls back product command and movement", func(t *testing.T) {
		input := app.CreateProductInput{Scope: scope, Name: "Rollback", PriceCents: 3000, InitialQuantity: 2, IdempotencyKey: key()}
		faultTx := catalogpg.NewProductTransactor(pool, func(q *db.Queries) app.ProductTx {
			bound := bind(q)
			bound.Stock = failAfterProductMovement{bound.Stock}
			return bound
		})
		if _, err := app.NewProductCommands(faultTx, time.Now).Create(ctx, input); err == nil {
			t.Fatal("injected failure ignored")
		}
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE shop_id=$1 AND name='Rollback'", shop).Scan(&count)
		if count != 0 {
			t.Fatal("partial product survived")
		}
		if out, err := commands.Create(ctx, input); err != nil || out.Replayed {
			t.Fatal("failed command could not be retried", err)
		}
	})
	t.Run("SQL forbids rewriting balances history and partial claims", func(t *testing.T) {
		p := fixture(2)
		for _, statement := range []string{
			"UPDATE product_stock SET quantity_on_hand=99,version=version+1 WHERE product_id=$1",
			"UPDATE product_stock_movements SET quantity_delta=99 WHERE product_id=$1",
			"DELETE FROM product_stock_movements WHERE product_id=$1",
		} {
			if _, err := pool.Exec(ctx, statement, p.Product.ID); err == nil {
				t.Fatal("protected direct write accepted", statement)
			}
		}
		if _, err := pool.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason) VALUES($1,$2,$3,$4,'receipt',1,'Foreign tenant')", uuid.New(), other, p.Product.ID, outsider); err == nil {
			t.Fatal("foreign tenant movement accepted")
		}
		if _, err := pool.Exec(ctx, "INSERT INTO product_commands(shop_id,actor_id,operation,key_digest,fingerprint) VALUES($1,$2,'record_stock',$3,$3)", shop, owner, make([]byte, 32)); err == nil {
			t.Fatal("partial claim committed")
		}
		assertBalance(p.Product.ID, 2)
	})
	t.Run("restricted runtime can append via trigger but cannot update quantity", func(t *testing.T) {
		p := fixture(0)
		role := "codex_products_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		quoted := pgx.Identifier{role}.Sanitize()
		exec("CREATE ROLE " + quoted + " NOLOGIN BYPASSRLS")
		defer func() {
			_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quoted)
			_, _ = pool.Exec(context.Background(), "DROP ROLE "+quoted)
		}()
		exec("GRANT USAGE ON SCHEMA public TO " + quoted)
		exec("GRANT SELECT ON product_stock TO " + quoted)
		exec("GRANT INSERT ON product_stock_movements TO " + quoted)
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		transaction, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = transaction.Exec(ctx, "SET LOCAL ROLE "+quoted)
		if err != nil {
			t.Fatal(err)
		}
		_, err = transaction.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason) VALUES($1,$2,$3,$4,'receipt',2,'Restricted role receipt')", uuid.New(), shop, p.Product.ID, owner)
		if err != nil {
			_ = transaction.Rollback(ctx)
			t.Fatal("trigger failed with least privilege", err)
		}
		if err := transaction.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		transaction, err = conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = transaction.Exec(ctx, "SET LOCAL ROLE "+quoted)
		_, err = transaction.Exec(ctx, "UPDATE product_stock SET quantity_on_hand=999 WHERE product_id=$1", p.Product.ID)
		_ = transaction.Rollback(ctx)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatal("restricted role changed balance", err)
		}
		assertBalance(p.Product.ID, 2)
	})
	t.Run("current membership and product versions are revalidated", func(t *testing.T) {
		p := fixture(2)
		if _, err := commands.RecordStock(ctx, app.RecordStockInput{Scope: app.ProductScope{ActorID: barber, ShopID: shop}, ProductID: p.Product.ID, Operation: app.StockReceive, QuantityDelta: 1, Reason: "Forbidden", IdempotencyKey: key()}); !errors.Is(err, app.ErrProductAccessDenied) {
			t.Fatal("barber changed inventory", err)
		}
		if _, err := commands.SetMinimum(ctx, app.UpdateStockMinimumInput{Scope: scope, ProductID: p.Product.ID, MinimumQuantity: 3, ExpectedStockVersion: p.Product.StockVersion}); err != nil {
			t.Fatal(err)
		}
		assertBalance(p.Product.ID, 2)
		if _, err := commands.SetMinimum(ctx, app.UpdateStockMinimumInput{Scope: scope, ProductID: p.Product.ID, MinimumQuantity: 4, ExpectedStockVersion: p.Product.StockVersion}); !errors.Is(err, domain.ErrStockVersionConflict) {
			t.Fatal("stale stock edit accepted", err)
		}
		details := domain.ProductDetails{Name: "New name", PriceCents: 3500, Active: false}
		if _, err := commands.Update(ctx, app.UpdateProductInput{Scope: scope, ProductID: p.Product.ID, Details: details, ExpectedVersion: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := commands.Update(ctx, app.UpdateProductInput{Scope: scope, ProductID: p.Product.ID, Details: details, ExpectedVersion: 1}); !errors.Is(err, domain.ErrProductVersionConflict) {
			t.Fatal("stale product edit accepted", err)
		}
		input := stockInput(p.Product.ID, 1)
		if _, err := commands.RecordStock(ctx, input); err != nil {
			t.Fatal("inactive product cannot be corrected", err)
		}
		exec("UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", owner, shop)
		defer exec("UPDATE shop_memberships SET active=true WHERE user_id=$1 AND shop_id=$2", owner, shop)
		if _, err := commands.RecordStock(ctx, input); !errors.Is(err, app.ErrProductAccessDenied) {
			t.Fatal("revoked actor replayed a command", err)
		}
	})
	t.Run("HTTP contract auth strict JSON pagination CORS and tenant boundaries", func(t *testing.T) {
		q := db.New(pool)
		repo := identitypg.NewRepository(q)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := httpapi.NewHandler(httpapi.HandlerConfig{Products: cataloghttp.NewProductHandler(commands, queries, 10*time.Second, logger), Authenticate: identity.NewAuthenticate(settingsVerifier{}, repo), AuthorizeShop: identity.NewAuthorizeShopAction(repo, shopspg.NewResolver(q)), FrontendOrigin: "http://127.0.0.1:3000", Logger: logger})
		base := "/api/v1/admin/shops/" + slug + "/products"
		request := func(method, path string, actor uuid.UUID, body, key string) *httptest.ResponseRecorder {
			t.Helper()
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			if actor != uuid.Nil {
				r.Header.Set("Authorization", "Bearer "+actor.String())
			}
			if body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			if key != "" {
				r.Header.Set("Idempotency-Key", key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		for _, tc := range []struct {
			actor  uuid.UUID
			suffix string
			want   int
		}{{uuid.Nil, "", 401}, {outsider, "", 403}, {barber, "?include_inactive=true", 403}, {barber, "", 200}, {owner, "?limit=101", 400}} {
			if w := request("GET", base+tc.suffix, tc.actor, "", ""); w.Code != tc.want {
				t.Fatal("read authorization", w.Code, tc.want, w.Body.String())
			}
		}
		body := `{"name":"HTTP product","price_cents":3000,"initial_quantity":2,"minimum_quantity":1}`
		commandKey := key()
		for _, actor := range []uuid.UUID{barber, outsider} {
			if w := request("POST", base, actor, body, key()); w.Code != 403 {
				t.Fatal("unauthorized write", w.Code)
			}
		}
		for _, bad := range []string{"null", `{"name":"Missing price"}`, strings.TrimSuffix(body, "}") + `,"shop_id":"` + other.String() + `"}`, body + " {}"} {
			if w := request("POST", base, owner, bad, key()); w.Code != 400 {
				t.Fatal("invalid body accepted", w.Code)
			}
		}
		if w := request("POST", base, owner, body, ""); w.Code != 400 {
			t.Fatal("missing key accepted", w.Code)
		}
		first := request("POST", base, owner, body, commandKey)
		if first.Code != 201 {
			t.Fatal(first.Code, first.Body.String())
		}
		if w := request("POST", base, owner, body, commandKey); w.Code != 200 {
			t.Fatal("HTTP replay failed", w.Code)
		}
		var created struct {
			Product struct {
				ID           uuid.UUID `json:"id"`
				StockVersion int64     `json:"stock_version"`
			}
			Replayed bool `json:"replayed"`
		}
		if json.Unmarshal(first.Body.Bytes(), &created) != nil {
			t.Fatal("bad product response")
		}
		productPath := base + "/" + created.Product.ID.String()
		minimumBody, _ := json.Marshal(map[string]any{"minimum_quantity": 3, "expected_stock_version": created.Product.StockVersion})
		if w := request("PUT", productPath+"/stock-minimum", owner, string(minimumBody), ""); w.Code != 200 {
			t.Fatal("minimum update route", w.Code, w.Body.String())
		}
		if w := request("PUT", productPath+"/stock-minimum", owner, string(minimumBody), ""); w.Code != 409 {
			t.Fatal("stale stock version through HTTP", w.Code)
		}
		if w := request("POST", productPath+"/stock-movements", owner, `{"operation":"receipt","quantity_delta":2,"reason":"Delivery"}`, key()); w.Code != 201 {
			t.Fatal("stock movement route", w.Code, w.Body.String())
		}
		if w := request("POST", productPath+"/stock-movements", owner, `{"operation":"adjustment","quantity_delta":-5,"reason":"Count"}`, key()); w.Code != 409 {
			t.Fatal("HTTP negative stock", w.Code, w.Body.String())
		}
		if w := request("POST", productPath+"/stock-movements", owner, `{"operation":"sale","quantity_delta":-1,"reason":"Fake sale"}`, key()); w.Code != 422 {
			t.Fatal("manual API created sale movement", w.Code)
		}
		updateBody := `{"name":"HTTP edited","description":"","price_cents":3500,"active":false,"expected_version":1}`
		if w := request("PUT", productPath, owner, updateBody, ""); w.Code != 200 {
			t.Fatal("product update route", w.Code, w.Body.String())
		}
		if w := request("PUT", productPath, owner, updateBody, ""); w.Code != 409 {
			t.Fatal("stale product HTTP edit", w.Code)
		}
		assertBalance(created.Product.ID, 4)
		if w := request("GET", productPath+"/stock-movements", barber, "", ""); w.Code != 403 {
			t.Fatal("barber read audit")
		}
		if w := request("GET", productPath+"/stock-movements", owner, "", ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		foreign := request("POST", base+"/"+uuid.NewString()+"/stock-movements", owner, `{"operation":"receipt","quantity_delta":1,"reason":"Delivery"}`, key())
		if foreign.Code != 404 {
			t.Fatal("missing product", foreign.Code)
		}
		page := request("GET", base+"?limit=1", owner, "", "")
		var listed struct {
			Items []struct {
				ID uuid.UUID `json:"id"`
			} `json:"items"`
			Next *string `json:"next_cursor"`
		}
		if page.Code != 200 || json.Unmarshal(page.Body.Bytes(), &listed) != nil || listed.Next == nil || len(listed.Items) != 1 {
			t.Fatal("missing page", page.Code, page.Body.String())
		}
		second := request("GET", base+"?limit=1&cursor="+url.QueryEscape(*listed.Next), owner, "", "")
		if second.Code != 200 {
			t.Fatal(second.Code, second.Body.String())
		}
		var secondPage struct {
			Items []struct {
				ID uuid.UUID `json:"id"`
			} `json:"items"`
		}
		if json.Unmarshal(second.Body.Bytes(), &secondPage) != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID == listed.Items[0].ID {
			t.Fatal("pagination repeated a product")
		}
		if w := request("GET", base+"?search=changed&cursor="+url.QueryEscape(*listed.Next), owner, "", ""); w.Code != 400 {
			t.Fatal("cursor crossed filters", w.Code)
		}
		if !strings.Contains(page.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private product response cached")
		}
		r := httptest.NewRequest("OPTIONS", productPath+"/stock-movements", nil)
		r.Header.Set("Origin", "http://127.0.0.1:3000")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal("preflight rejected", w.Code)
		}
	})
	t.Run("SQL rejects quantities beyond its integer limit without partial history", func(t *testing.T) {
		p := fixture(domain.MaxStockQuantity)
		_, err := pool.Exec(ctx, "INSERT INTO product_stock_movements(id,shop_id,product_id,actor_id,kind,quantity_delta,reason) VALUES($1,$2,$3,$4,'receipt',1,'Overflow test')", uuid.New(), shop, p.Product.ID, owner)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "product_stock_limit" {
			t.Fatal("unbounded database quantity", err)
		}
		assertBalance(p.Product.ID, domain.MaxStockQuantity)
	})
	t.Run("existing foreign product and inactive barber cannot reuse a trusted scope", func(t *testing.T) {
		foreign, err := commands.Create(ctx, app.CreateProductInput{Scope: app.ProductScope{ActorID: outsider, ShopID: other}, Name: "Other unit product", PriceCents: 3000, InitialQuantity: 1, IdempotencyKey: key()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := commands.RecordStock(ctx, stockInput(foreign.Product.ID, 1)); !errors.Is(err, app.ErrProductNotFound) {
			t.Fatal("foreign product UUID resolved", err)
		}
		if _, err := queries.Movements(ctx, app.StockMovementListInput{Scope: scope, ProductID: foreign.Product.ID}); !errors.Is(err, app.ErrProductNotFound) {
			t.Fatal("foreign audit revealed", err)
		}
		exec("UPDATE professionals SET active=false WHERE id=$1", professional)
		defer exec("UPDATE professionals SET active=true WHERE id=$1", professional)
		if _, err := queries.List(ctx, app.ProductListInput{Scope: app.ProductScope{ActorID: barber, ShopID: shop}}); !errors.Is(err, app.ErrProductAccessDenied) {
			t.Fatal("inactive professional retained access", err)
		}
	})
}
