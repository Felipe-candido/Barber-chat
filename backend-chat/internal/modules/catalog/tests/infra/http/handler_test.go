package http_test

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

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type testStore struct {
	shop        uuid.UUID
	created     int
	err         error
	createdShop uuid.UUID
	listedShop  uuid.UUID
}

func (s *testStore) LookupActiveShop(_ context.Context, slug string) (uuid.UUID, bool, error) {
	return s.shop, slug != "missing", s.err
}
func (s *testStore) Create(_ context.Context, v domain.Service) (domain.Service, error) {
	s.created++
	s.createdShop = v.ShopID
	return v, s.err
}
func (s *testStore) ListActiveByShop(_ context.Context, shopID uuid.UUID) ([]domain.Service, error) {
	s.listedShop = shopID
	return nil, s.err
}
func testRouter(store *testStore) http.Handler {
	h := cataloghttp.NewHandler(application.NewCreateService(store), application.NewListServices(store, store), application.NewListAuthorizedServices(store), time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := chi.NewRouter()
	cataloghttp.RegisterPublicRoutes(router, h)
	// Authentication and authorization are tested together in httpapi/tests.
	router.Post("/api/v1/admin/shops/{slug}/services", h.CreateService)
	router.Get("/api/v1/admin/shops/{slug}/services", h.ListShopServices)
	return router
}

const validBody = `{"name":" Corte ","description":"Tesoura","duration_minutes":30,"price_cents":3500}`

func TestCreateHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		want                    int
	}{
		{"valid", validBody, "application/json", 201},
		{"client tenant", strings.TrimSuffix(validBody, "}") + `,"shop_id":"other"}`, "application/json", 400},
		{"client slug", strings.TrimSuffix(validBody, "}") + `,"shop_slug":"other"}`, "application/json", 400},
		{"client user", strings.TrimSuffix(validBody, "}") + `,"user_id":"other"}`, "application/json", 400},
		{"multiple objects", validBody + " {}", "application/json", 400},
		{"trailing garbage", validBody + " x", "application/json", 400},
		{"null", "null", "application/json", 400},
		{"missing price", `{"name":"Corte","duration_minutes":30}`, "application/json", 422},
		{"invalid duration", strings.Replace(validBody, ":30", ":0", 1), "application/json", 422},
		{"wrong content type", validBody, "text/plain", 415},
		{"large body", `{"name":"` + strings.Repeat("x", (64<<10)) + `"}`, "application/json", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{shop: uuid.New()}
			req := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/admin/shops/shop/services", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			// Supply the already verified boundary values, not client identifiers.
			userID := uuid.New()
			ctx := requestctx.WithStaffIdentity(req.Context(), identityapp.StaffIdentityOutput{UserID: userID})
			ctx = requestctx.WithShopScope(ctx, identityapp.AuthorizedShopScope{UserID: userID, ShopID: store.shop})
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			testRouter(store).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatal("response is not JSON")
			}
			if tc.want == 201 {
				var response struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Currency string `json:"currency"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.ID == "" || response.Currency != "BRL" || response.Name != "Corte" || store.createdShop != store.shop || store.created != 1 {
					t.Fatal("invalid creation response or tenant")
				}
			} else if store.created != 0 {
				t.Fatal("rejected request was persisted")
			}
		})
	}
}
func TestCreateRequiresMatchingIdentityAndScope(t *testing.T) {
	userID, shopID := uuid.New(), uuid.New()
	identity := identityapp.StaffIdentityOutput{UserID: userID}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want int
	}{
		{"missing identity", context.Background(), 401},
		{"empty identity", requestctx.WithStaffIdentity(context.Background(), identityapp.StaffIdentityOutput{}), 401},
		{"missing scope", requestctx.WithStaffIdentity(context.Background(), identity), 403},
		{"empty shop", requestctx.WithShopScope(requestctx.WithStaffIdentity(context.Background(), identity), identityapp.AuthorizedShopScope{UserID: userID}), 403},
		{"different user", requestctx.WithShopScope(requestctx.WithStaffIdentity(context.Background(), identity), identityapp.AuthorizedShopScope{UserID: uuid.New(), ShopID: shopID}), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{shop: shopID}
			for _, method := range []string{"GET", "POST"} {
				r := httptest.NewRequest(method, "/api/v1/admin/shops/shop/services", strings.NewReader(validBody)).WithContext(tc.ctx)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				testRouter(store).ServeHTTP(w, r)
				if w.Code != tc.want || store.created != 0 || store.listedShop != uuid.Nil {
					t.Fatalf("method=%s status=%d writes=%d read=%s", method, w.Code, store.created, store.listedShop)
				}
			}
		})
	}
}

func TestAdminListUsesOnlyAuthorizedScope(t *testing.T) {
	store := &testStore{shop: uuid.New()}
	userID := uuid.New()
	ctx := requestctx.WithStaffIdentity(context.Background(), identityapp.StaffIdentityOutput{UserID: userID})
	ctx = requestctx.WithShopScope(ctx, identityapp.AuthorizedShopScope{UserID: userID, ShopID: store.shop})
	// Even conflicting identifiers cannot override the middleware's scope.
	r := httptest.NewRequest("GET", "/api/v1/admin/shops/ignored/services?shop_id="+uuid.NewString(), nil).WithContext(ctx)
	w := httptest.NewRecorder()
	testRouter(store).ServeHTTP(w, r)
	if w.Code != 200 || store.listedShop != store.shop || strings.TrimSpace(w.Body.String()) != "[]" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d shop=%s body=%s", w.Code, store.listedShop, w.Body)
	}
}
func TestPublicListAndSanitizedFailure(t *testing.T) {
	store := &testStore{shop: uuid.New()}
	for _, tc := range []struct {
		slug   string
		err    error
		status int
	}{
		{"shop", nil, 200}, {"missing", nil, 404}, {"shop", errors.New("postgres://secret@host"), 500}, {"shop", context.DeadlineExceeded, 503},
	} {
		store.err = tc.err
		w := httptest.NewRecorder()
		testRouter(store).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/shops/"+tc.slug+"/services", nil))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("unexpected response %d %s", w.Code, w.Body.String())
		}
		if tc.status == 200 && strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatal("empty list must be []")
		}
	}
}
