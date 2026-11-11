package middleware_test

import (
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedProxyIdentityAndHTTPS(t *testing.T) {
	p, err := middleware.NewClientNetworkPolicy("10.20.0.2/32,2001:db8::2/128")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, peer, xff, proto, want string
		status                       int
	}{
		{"direct spoof ignored", "198.51.100.2:9000", "203.0.113.9", "https", "198.51.100.2", 426},
		{"trusted client", "10.20.0.2:9000", "198.51.100.2", "https", "198.51.100.2", 200},
		{"untrusted middle stops traversal", "10.20.0.2:9000", "203.0.113.9, 198.51.100.2", "https", "198.51.100.2", 200},
		{"trusted chain", "10.20.0.2:9000", "198.51.100.2, 2001:db8::2", "https", "198.51.100.2", 200},
		{"IPv6", "[2001:db8::2]:9000", "2001:db8::123", "https", "2001:db8::123", 200},
		{"proxy cannot forge HTTPS through malformed list", "10.20.0.2:9000", "198.51.100.2", "https,http", "", 400},
		{"bad forwarded IP", "10.20.0.2:9000", "not-an-address", "https", "", 400},
		{"plain proxy transport rejected", "10.20.0.2:9000", "198.51.100.2", "http", "", 426},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := p.Middleware(middleware.SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if middleware.ClientAddress(r) != tc.want {
					t.Fatal(middleware.ClientAddress(r), tc.want)
				}
				w.WriteHeader(200)
			})))
			r := httptest.NewRequest("POST", "http://api.example/api/v1/onboarding/shops", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Forwarded-For", tc.xff)
			r.Header.Set("X-Forwarded-Proto", tc.proto)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			if w.Code == 200 && w.Header().Get("Strict-Transport-Security") == "" {
				t.Fatal("missing HSTS")
			}
		})
	}
	for _, raw := range []string{"0.0.0.0/0", "::/0", "not-a-cidr"} {
		if _, err := middleware.NewClientNetworkPolicy(raw); err == nil {
			t.Fatal("unsafe proxy network accepted", raw)
		}
	}
	health := p.Middleware(middleware.SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })))
	w := httptest.NewRecorder()
	health.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/ready", nil))
	if w.Code != 200 {
		t.Fatal("private readiness probe blocked")
	}
}

func TestAbuseLimitsDoNotTrustCallerHeadersOrAllowActorRotation(t *testing.T) {
	p, _ := middleware.NewClientNetworkPolicy("10.20.0.2/32")
	limits := middleware.NewAbuseLimits()
	actor := uuid.New()
	h := p.Middleware(limits.Guard("billing", 10, 2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })))
	request := func(client string, user uuid.UUID) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://api.example/checkout", nil)
		r.RemoteAddr = "10.20.0.2:9000"
		r.Header.Set("X-Forwarded-For", client)
		r = r.WithContext(requestctx.WithStaffIdentity(r.Context(), identity.StaffIdentityOutput{UserID: user}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if request("198.51.100.1", actor).Code != 200 || request("198.51.100.2", actor).Code != 200 {
		t.Fatal("initial requests rejected")
	}
	w := request("198.51.100.3", actor)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("actor escaped rate limit by changing IP", w.Code)
	}
	if request("198.51.100.4", uuid.New()).Code != 200 {
		t.Fatal("another actor was charged to the proxy")
	}
	direct, _ := middleware.NewClientNetworkPolicy("")
	ipOnly := direct.Middleware(limits.Guard("onboarding", 1, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })))
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "http://api.example/onboarding", nil)
		r.RemoteAddr = "198.51.100.20:1000"
		r.Header.Set("X-Forwarded-For", strings.Repeat("1", i+1))
		w := httptest.NewRecorder()
		ipOnly.ServeHTTP(w, r)
		if (i == 0 && w.Code != 200) || (i == 1 && w.Code != 429) {
			t.Fatal("untrusted XFF changed quota", w.Code)
		}
	}
}
