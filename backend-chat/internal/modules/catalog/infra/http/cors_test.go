package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestLocalBrowserAccess(t *testing.T) {
	const origin = "http://127.0.0.1:3000"
	for _, tc := range []struct {
		name, method, path, origin, remote, host, requestedMethod, requestedHeaders string
		status                                                                      int
		cors                                                                        bool
	}{
		{"create", "POST", "/api/v1/admin/services", origin, "127.0.0.1:1234", "127.0.0.1:8080", "", "", 201, true},
		{"public list", "GET", "/api/v1/public/shops/shop/services", origin, "127.0.0.1:1234", "127.0.0.1:8080", "", "", 200, true},
		{"preflight", "OPTIONS", "/api/v1/admin/services", origin, "127.0.0.1:1234", "127.0.0.1:8080", "POST", "content-type", 204, true},
		{"different port", "POST", "/api/v1/admin/services", "http://127.0.0.1:3001", "127.0.0.1:1234", "127.0.0.1:8080", "", "", 403, false},
		{"remote peer", "POST", "/api/v1/admin/services", origin, "192.0.2.1:1234", "127.0.0.1:8080", "", "", 403, false},
		{"untrusted host", "POST", "/api/v1/admin/services", origin, "127.0.0.1:1234", "evil.example", "", "", 403, false},
		{"untrusted origin", "OPTIONS", "/api/v1/admin/services", "https://evil.example", "127.0.0.1:1234", "127.0.0.1:8080", "POST", "content-type", 403, false},
		{"unsupported method", "OPTIONS", "/api/v1/admin/services", origin, "127.0.0.1:1234", "127.0.0.1:8080", "DELETE", "content-type", 403, true},
		{"unsupported header", "OPTIONS", "/api/v1/admin/services", origin, "127.0.0.1:1234", "127.0.0.1:8080", "POST", "x-shop-id", 403, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{shop: uuid.New()}
			handler := NewHandler(application.NewCreateService(store, store), application.NewListServices(store, store), "shop", time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)), origin)
			router := chi.NewRouter()
			RegisterRoutes(router, handler)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(validBody))
			request.RemoteAddr = tc.remote
			request.Host = tc.host
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Access-Control-Request-Method", tc.requestedMethod)
			request.Header.Set("Access-Control-Request-Headers", tc.requestedHeaders)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Access-Control-Allow-Origin"); (got == origin) != tc.cors {
				t.Fatalf("unexpected CORS origin %q", got)
			}
			if response.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("credentials must not be enabled")
			}
			if tc.status == http.StatusCreated && store.created != 1 {
				t.Fatal("service was not created")
			}
			if tc.status != http.StatusCreated && store.created != 0 {
				t.Fatal("rejected request created data")
			}
		})
	}
}
