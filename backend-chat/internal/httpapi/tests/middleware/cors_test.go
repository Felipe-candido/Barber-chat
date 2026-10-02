package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
)

func TestCORS(t *testing.T) {
	const origin = "http://localhost:3000"
	for _, tc := range []struct {
		name, origin, method, requestedMethod, requestedHeaders string
		want                                                    int
		cors                                                    bool
	}{
		{"allowed", origin, "GET", "", "", 401, true},
		{"no origin", "", "GET", "", "", 401, false},
		{"untrusted", "https://evil.example", "GET", "", "", 403, false},
		{"different port", "http://localhost:3001", "GET", "", "", 403, false},
		{"preflight", origin, "OPTIONS", "GET", "authorization, Content-Type", 204, true},
		{"untrusted preflight", "https://evil.example", "OPTIONS", "GET", "authorization", 403, false},
		{"wrong method", origin, "OPTIONS", "DELETE", "authorization", 403, true},
		{"wrong header", origin, "OPTIONS", "GET", "x-shop-id", 403, true},
		{"preflight without origin", "", "OPTIONS", "GET", "authorization", 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := chi.NewRouter()
			router.Use(middleware.CORS(origin))
			router.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.WriteHeader(401)
			})
			router.Options("/me", middleware.Preflight(http.MethodGet))
			r := httptest.NewRequest(tc.method, "/me", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("Access-Control-Request-Method", tc.requestedMethod)
			r.Header.Set("Access-Control-Request-Headers", tc.requestedHeaders)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.want || (w.Header().Get("Access-Control-Allow-Origin") == origin) != tc.cors {
				t.Fatalf("status=%d headers=%v", w.Code, w.Header())
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("cookie credentials must not be enabled")
			}
			if tc.cors && w.Header().Get("Access-Control-Expose-Headers") != "WWW-Authenticate" {
				t.Fatal("authentication errors must be readable by the browser")
			}
		})
	}
}

func TestCORSRejectsAmbiguousOrUnconfiguredOrigin(t *testing.T) {
	for _, configured := range []string{"", "http://localhost:3000"} {
		h := middleware.CORS(configured)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("request must be rejected") }))
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Add("Origin", "http://localhost:3000")
		if configured != "" {
			r.Header.Add("Origin", "http://localhost:3000")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
}
