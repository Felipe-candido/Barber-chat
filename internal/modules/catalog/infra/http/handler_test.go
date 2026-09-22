package http

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testStore struct {
	shop    uuid.UUID
	created int
	err     error
	slug    string
}

func (s *testStore) LookupActiveShop(_ context.Context, slug string) (uuid.UUID, bool, error) {
	s.slug = slug
	return s.shop, slug != "missing", s.err
}
func (s *testStore) Create(_ context.Context, v domain.Service) (domain.Service, error) {
	s.created++
	return v, s.err
}
func (s *testStore) ListActiveByShop(context.Context, uuid.UUID) ([]domain.Service, error) {
	return nil, s.err
}
func testRouter(store *testStore, slug string) http.Handler {
	h := NewHandler(application.NewCreateService(store, store), application.NewListServices(store, store), slug, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := chi.NewRouter()
	RegisterRoutes(router, h)
	return router
}

const validBody = `{"name":" Corte ","description":"Tesoura","duration_minutes":30,"price_cents":3500}`

func TestCreateHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, body, slug, remote, host, origin, contentType string
		want                                                int
	}{
		{"valid", validBody, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 201},
		{"disabled", validBody, "", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 403},
		{"remote", validBody, "configured-shop", "192.0.2.1:1234", "127.0.0.1:8080", "", "application/json", 403},
		{"untrusted host", validBody, "configured-shop", "127.0.0.1:1234", "attacker.example", "", "application/json", 403},
		{"browser origin", validBody, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "https://attacker.example", "application/json", 403},
		{"unknown shop", validBody, "missing", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 404},
		{"client tenant", strings.TrimSuffix(validBody, "}") + `,"shop_id":"other"}`, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 400},
		{"client slug", strings.TrimSuffix(validBody, "}") + `,"shop_slug":"other"}`, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 400},
		{"multiple objects", validBody + " {}", "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 400},
		{"trailing garbage", validBody + " x", "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 400},
		{"null", "null", "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 400},
		{"missing price", `{"name":"Corte","duration_minutes":30}`, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 422},
		{"invalid duration", strings.Replace(validBody, ":30", ":0", 1), "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 422},
		{"wrong content type", validBody, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "text/plain", 415},
		{"large body", `{"name":"` + strings.Repeat("x", maxBodyBytes) + `"}`, "configured-shop", "127.0.0.1:1234", "127.0.0.1:8080", "", "application/json", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{shop: uuid.New()}
			req := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/admin/services", strings.NewReader(tc.body))
			req.RemoteAddr = tc.remote
			req.Host = tc.host
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			testRouter(store, tc.slug).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatal("response is not JSON")
			}
			if tc.want == 201 {
				var response serviceResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.ID == "" || response.Currency != "BRL" || response.Name != "Corte" || store.slug != tc.slug || store.created != 1 {
					t.Fatal("invalid creation response or tenant")
				}
			} else if store.created != 0 {
				t.Fatal("rejected request was persisted")
			}
		})
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
		testRouter(store, "").ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/shops/"+tc.slug+"/services", nil))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("unexpected response %d %s", w.Code, w.Body.String())
		}
		if tc.status == 200 && strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatal("empty list must be []")
		}
	}
}
