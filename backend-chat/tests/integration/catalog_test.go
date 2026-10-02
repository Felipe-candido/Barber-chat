//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	shopspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func catalogTransaction(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := postgres.Open(ctx, requiredEnv(t, "DATABASE_URL"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("could not start PostgreSQL test transaction")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error("test transaction rollback failed")
		}
	})
	return ctx, tx
}

func insertTestShop(t *testing.T, ctx context.Context, tx pgx.Tx) (uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	slug := "catalog-test-" + id.String()
	_, err := tx.Exec(ctx, "INSERT INTO shops (id,name,slug) VALUES ($1,$2,$3)", id, "Catalog integration test", slug)
	if err != nil {
		t.Fatal(err)
	}
	return id, slug
}

// JWT cryptography is covered by the HTTP access tests. This fixture exercises
// the real database and membership checks, without an external Supabase project.
// Use plain development PostgreSQL with the portable migrations, not a hosted
// database whose users table requires real auth.users identities.
type catalogTokenVerifier struct{ userID uuid.UUID }

func (v catalogTokenVerifier) VerifyAccessToken(_ context.Context, token string) (uuid.UUID, error) {
	if token != "integration-token" {
		return uuid.Nil, identityapp.ErrInvalidAccessToken
	}
	return v.userID, nil
}

func TestCatalogHTTPPersistenceAndIsolation(t *testing.T) {
	ctx, tx := catalogTransaction(t)
	shopA, slugA := insertTestShop(t, ctx, tx)
	shopB, slugB := insertTestShop(t, ctx, tx)
	queries := db.New(tx)
	repo := catalogpostgres.NewRepository(queries)
	resolver := shopspostgres.NewResolver(queries)
	create := application.NewCreateService(repo)
	list := application.NewListServices(repo, resolver)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := cataloghttp.NewHandler(create, list, application.NewListAuthorizedServices(repo), 3*time.Second, logger)
	userID := uuid.New()
	if _, err := tx.Exec(ctx, "INSERT INTO public.users (id,display_name) VALUES ($1,'HTTP integration test')", userID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.shop_memberships (user_id,shop_id) VALUES ($1,$2)", userID, shopA); err != nil {
		t.Fatal(err)
	}
	identityRepo := identitypostgres.NewRepository(queries)
	router := httpapi.NewHandler(httpapi.HandlerConfig{
		Catalog: handler, Authenticate: identityapp.NewAuthenticate(catalogTokenVerifier{userID}, identityRepo),
		AuthorizeShop: identityapp.NewAuthorizeShopAction(identityRepo, resolver), Logger: logger,
		ListMyShops:    identityapp.NewListMyShops(identityRepo, identityRepo),
		RequestTimeout: 3 * time.Second,
	})

	request := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/admin/shops/"+slugA+"/services",
		strings.NewReader(`{"name":" Corte ","description":" Tesoura ","duration_minutes":30,"price_cents":3500}`))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer integration-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", response.Code, response.Body.String())
	}
	var created struct {
		ID       uuid.UUID `json:"id"`
		Currency string    `json:"currency"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	var storedShop uuid.UUID
	var name, currency string
	var price int64
	if err := tx.QueryRow(ctx, "SELECT shop_id,name,price_cents,currency FROM services WHERE id=$1", created.ID).Scan(&storedShop, &name, &price, &currency); err != nil {
		t.Fatal(err)
	}
	if storedShop != shopA || name != "Corte" || price != 3500 || currency != "BRL" || created.Currency != "BRL" {
		t.Fatal("persisted values differ from the expected tenant/service")
	}

	denied := httptest.NewRequest("POST", "/api/v1/admin/shops/"+slugB+"/services", strings.NewReader(`{"name":"Forbidden","duration_minutes":30,"price_cents":3500}`))
	denied.Header.Set("Authorization", "Bearer integration-token")
	denied.Header.Set("Content-Type", "application/json")
	deniedResponse := httptest.NewRecorder()
	router.ServeHTTP(deniedResponse, denied)
	if deniedResponse.Code != http.StatusForbidden {
		t.Fatal("another shop's membership was accepted")
	}
	var deniedWrites int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM services WHERE shop_id=$1", shopB).Scan(&deniedWrites); err != nil || deniedWrites != 0 {
		t.Fatal("unauthorized request wrote data", err)
	}

	_, err := create.Execute(ctx, application.CreateServiceInput{ShopID: shopB, Name: "Other shop service", DurationMinutes: 45, PriceCents: 5000})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := create.Execute(ctx, application.CreateServiceInput{ShopID: shopA, Name: "Inactive service", DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE services SET active=false WHERE shop_id=$1 AND id=$2", shopA, hidden.ID); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/public/shops/"+slugA+"/services", nil))
	var services []struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &services); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(services) != 1 || services[0].ID != created.ID {
		t.Fatal("list leaked another tenant or an inactive service")
	}

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/v1/admin/shops/" + slugA + "/services", 200},
		{"/api/v1/admin/shops/" + slugB + "/services", 403},
		{"/api/v1/admin/shops", 200},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Authorization", "Bearer integration-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s returned %d: %s", tc.path, w.Code, w.Body)
		}
		if tc.want == 200 && strings.Contains(w.Body.String(), slugB) {
			t.Fatal("accessible shops leaked another user's unit")
		}
		if tc.path == "/api/v1/admin/shops/"+slugA+"/services" {
			if err := json.Unmarshal(w.Body.Bytes(), &services); err != nil || len(services) != 1 || services[0].ID != created.ID {
				t.Fatal("administrative list crossed tenant or activity boundaries", err)
			}
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE shop_memberships SET active=false WHERE user_id=$1 AND shop_id=$2", userID, shopA); err != nil {
		t.Fatal(err)
	}
	revoked := httptest.NewRequest("GET", "/api/v1/admin/shops/"+slugA+"/services", nil)
	revoked.Header.Set("Authorization", "Bearer integration-token")
	revokedResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedResponse, revoked)
	if revokedResponse.Code != 403 {
		t.Fatal("revoked database membership remained authorized")
	}

	if _, err := tx.Exec(ctx, "UPDATE shops SET active=false WHERE id=$1", shopB); err != nil {
		t.Fatal(err)
	}
	if _, err := list.Execute(ctx, slugB); !errors.Is(err, application.ErrShopNotFound) {
		t.Fatal("inactive shop remained public", err)
	}
	if _, err := create.Execute(ctx, application.CreateServiceInput{ShopID: shopB, Name: "Blocked", DurationMinutes: 30}); !errors.Is(err, application.ErrShopNotFound) {
		t.Fatal("inactive shop accepted creation", err)
	}
	if _, err := list.Execute(ctx, "missing-"+uuid.NewString()); !errors.Is(err, application.ErrShopNotFound) {
		t.Fatal("unknown shop did not return not-found", err)
	}

	before := 0
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM services WHERE shop_id=$1", shopA).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := create.Execute(ctx, application.CreateServiceInput{ShopID: shopA, Name: " ", DurationMinutes: 30}); err == nil {
		t.Fatal("invalid service accepted")
	}
	var after int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM services WHERE shop_id=$1", shopA).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("invalid service was persisted")
	}
}

func TestCatalogDatabaseConstraints(t *testing.T) {
	ctx, tx := catalogTransaction(t)
	shop, slug := insertTestShop(t, ctx, tx)
	for _, tc := range []struct {
		name     string
		shop     uuid.UUID
		title    string
		duration int32
		price    int64
		currency string
	}{
		{"missing tenant", uuid.New(), "Corte", 30, 3500, "BRL"},
		{"empty name", shop, " ", 30, 3500, "BRL"},
		{"invalid duration", shop, "Corte", 0, 3500, "BRL"},
		{"invalid price", shop, "Corte", 30, -1, "BRL"},
		{"invalid currency", shop, "Corte", 30, 3500, "USD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tx.Exec(ctx, "SAVEPOINT invalid_service"); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(ctx, "INSERT INTO services (id,shop_id,name,duration_minutes,price_cents,currency) VALUES ($1,$2,$3,$4,$5,$6)",
				uuid.New(), tc.shop, tc.title, tc.duration, tc.price, tc.currency)
			if err == nil {
				t.Error("database accepted an invalid service")
			}
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT invalid_service"); err != nil {
				t.Fatal(err)
			}
		})
	}
	queries := db.New(tx)
	repo := catalogpostgres.NewRepository(queries)
	list := application.NewListServices(repo, shopspostgres.NewResolver(queries))
	result, err := list.Execute(ctx, slug)
	if err != nil || result == nil || len(result) != 0 {
		t.Fatal("empty shop should have a non-nil empty catalog", err)
	}
	// Even a caller that resolved the tenant earlier cannot insert after deactivation.
	if _, err := tx.Exec(ctx, "UPDATE shops SET active=false WHERE id=$1", shop); err != nil {
		t.Fatal(err)
	}
	_, err = queries.CreateService(ctx, db.CreateServiceParams{ID: uuid.New(), ShopID: shop, Name: "Blocked", DurationMinutes: 30, PriceCents: 3500, Currency: "BRL", Active: true})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("INSERT did not recheck shop activity", err)
	}
}
