package httpapi_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	catalogapp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	catalogdomain "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/supabase"
	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const serviceBody = `{"name":"Corte","duration_minutes":30,"price_cents":3500}`
const browserOrigin = "http://localhost:3000"

type accessStore struct {
	user            identitydomain.User
	userFound       bool
	memberships     map[uuid.UUID]identitydomain.Membership
	shops           map[string]uuid.UUID
	created         []catalogdomain.Service
	listedShopIDs   []uuid.UUID
	userCalls       int
	err             error
	recheckInactive bool
}

func (s *accessStore) FindUserByID(context.Context, uuid.UUID) (identitydomain.User, bool, error) {
	s.userCalls++
	user := s.user
	if s.recheckInactive && s.userCalls > 1 {
		user.Active = false
	}
	return user, s.userFound, s.err
}
func (s *accessStore) FindMembership(_ context.Context, userID, shopID uuid.UUID) (identitydomain.Membership, bool, error) {
	membership, found := s.memberships[shopID]
	return membership, found, s.err
}
func (s *accessStore) ListMembershipsByUser(context.Context, uuid.UUID) ([]identitydomain.Membership, error) {
	return nil, errors.New("HTTP access must use the specific user/shop membership")
}
func (s *accessStore) LookupActiveShop(_ context.Context, slug string) (uuid.UUID, bool, error) {
	id, found := s.shops[slug]
	return id, found, s.err
}
func (s *accessStore) Create(_ context.Context, service catalogdomain.Service) (catalogdomain.Service, error) {
	s.created = append(s.created, service)
	return service, s.err
}
func (s *accessStore) ListActiveByShop(_ context.Context, shopID uuid.UUID) ([]catalogdomain.Service, error) {
	s.listedShopIDs = append(s.listedShopIDs, shopID)
	return []catalogdomain.Service{}, s.err
}

type accessFixture struct {
	store      *accessStore
	verifier   *supabase.Verifier
	key        *ecdsa.PrivateKey
	origin     string
	jwksCalls  atomic.Int32
	jwksStatus atomic.Int32
	logs       bytes.Buffer
}

// Real ES256 signatures, JWKS HTTP requests, verifier and use cases; only
// persistence is substituted. PostgreSQL behavior has separate integration tests.
func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &accessFixture{key: key}
	f.jwksStatus.Store(200)
	body, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "ES256", Use: "sig"}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.jwksCalls.Add(1)
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" {
			t.Errorf("unexpected JWKS path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("access token was sent to JWKS")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(f.jwksStatus.Load()))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	f.origin = server.URL
	f.verifier, err = supabase.NewVerifier(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	userID, shopID := uuid.New(), uuid.New()
	f.store = &accessStore{
		user: identitydomain.User{ID: userID, DisplayName: "Felipe", Active: true}, userFound: true,
		shops:       map[string]uuid.UUID{"shop-a": shopID, "shop-b": uuid.New()},
		memberships: map[uuid.UUID]identitydomain.Membership{shopID: {UserID: userID, ShopID: shopID, Active: true}},
	}
	return f
}

func (f *accessFixture) handler() http.Handler {
	logger := slog.New(slog.NewJSONHandler(&f.logs, nil))
	catalog := cataloghttp.NewHandler(catalogapp.NewCreateService(f.store), catalogapp.NewListServices(f.store, f.store), catalogapp.NewListAuthorizedServices(f.store), time.Second, logger)
	return httpapi.NewHandler(httpapi.HandlerConfig{
		Catalog: catalog, Authenticate: identityapp.NewAuthenticate(f.verifier, f.store),
		AuthorizeShop: identityapp.NewAuthorizeShopAction(f.store, f.store),
		Logger:        logger, FrontendOrigin: browserOrigin,
		RequestTimeout: time.Second, CheckDB: func(context.Context) error { return nil },
	})
}

func (f *accessFixture) token(t *testing.T, change func(jwt.MapClaims)) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": f.store.user.ID.String(), "iss": f.origin + "/auth/v1", "aud": "authenticated",
		"exp": now.Add(time.Hour).Unix(), "iat": now.Add(-time.Minute).Unix(),
		"role": "authenticated", "is_anonymous": false,
	}
	if change != nil {
		change(claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func accessRequest(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMeReturnsVerifiedLocalIdentityAndReusesJWKS(t *testing.T) {
	f := newAccessFixture(t)
	h := f.handler()
	token := f.token(t, nil)
	for range 2 {
		w := accessRequest(h, "GET", "/api/v1/admin/me", token, "")
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || len(body) != 2 || body["user_id"] != f.store.user.ID.String() || body["display_name"] != "Felipe" {
			t.Fatalf("identity mismatch: %d %s", w.Code, w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), token) {
			t.Fatal("identity response leaked or cached credentials")
		}
	}
	if f.jwksCalls.Load() != 1 || f.store.userCalls != 2 {
		t.Fatal("JWKS must be cached, but local user state must be reread")
	}
}

func TestMeRejectsInvalidJWTBeforeDatabase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(jwt.MapClaims)
	}{
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"issuer", func(c jwt.MapClaims) { c["iss"] = "https://evil.example/auth/v1" }},
		{"audience", func(c jwt.MapClaims) { c["aud"] = "anon" }},
		{"anonymous", func(c jwt.MapClaims) { c["is_anonymous"] = true }},
		{"service role", func(c jwt.MapClaims) { c["role"] = "service_role" }},
		{"missing expiration", func(c jwt.MapClaims) { delete(c, "exp") }},
		{"invalid subject", func(c jwt.MapClaims) { c["sub"] = "not-a-uuid" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccessFixture(t)
			w := accessRequest(f.handler(), "GET", "/api/v1/admin/me", f.token(t, tc.change), "")
			if w.Code != 401 || f.store.userCalls != 0 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "invalid_token") {
				t.Fatalf("status=%d database calls=%d", w.Code, f.store.userCalls)
			}
		})
	}
	f := newAccessFixture(t)
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.key = otherKey
	if w := accessRequest(f.handler(), "GET", "/api/v1/admin/me", f.token(t, nil), ""); w.Code != 401 || f.store.userCalls != 0 {
		t.Fatal("wrong signature accepted")
	}
}

func TestMeRejectsMalformedAuthorization(t *testing.T) {
	f := newAccessFixture(t)
	h := f.handler()
	for _, values := range [][]string{
		nil, {""}, {"Basic secret"}, {"Bearer"}, {"Bearer "}, {"Bearer a b"}, {"Bearer a,b"}, {"Bearer\ttoken"}, {"Bearer " + strings.Repeat("x", 33*1024)}, {"Bearer one", "Bearer two"},
	} {
		r := httptest.NewRequest("GET", "/api/v1/admin/me", nil)
		for _, value := range values {
			r.Header.Add("Authorization", value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("malformed header accepted: %d", w.Code)
		}
	}
	if f.jwksCalls.Load() != 0 || f.store.userCalls != 0 {
		t.Fatal("malformed credentials reached dependencies")
	}
	// Scheme is case insensitive and RFC 6750 permits more than one space.
	r := httptest.NewRequest("GET", "/api/v1/admin/me", nil)
	r.Header.Set("Authorization", "bEaReR   "+f.token(t, nil))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("valid bearer rejected: %d", w.Code)
	}
}

func TestCredentialsInURLDoNotAuthenticate(t *testing.T) {
	f := newAccessFixture(t)
	w := accessRequest(f.handler(), "GET", "/api/v1/admin/me?access_token="+f.token(t, nil), "", "")
	if w.Code != 401 || f.jwksCalls.Load() != 0 || f.store.userCalls != 0 {
		t.Fatal("URL credentials authenticated")
	}
}

func TestMissingAuthenticationDependencyFailsClosed(t *testing.T) {
	h := httpapi.NewHandler(httpapi.HandlerConfig{Logger: testLogger()})
	w := accessRequest(h, "GET", "/api/v1/admin/me", "opaque-token", "")
	if w.Code != 503 {
		t.Fatal("missing authentication dependency granted access")
	}
}

func TestMeDistinguishesAccessDenialFromDependencyFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*accessFixture)
		want  int
	}{
		{"unprovisioned", func(f *accessFixture) { f.store.userFound = false }, 403},
		{"inactive", func(f *accessFixture) { f.store.user.Active = false }, 403},
		{"repository identity mismatch", func(f *accessFixture) { f.store.user.ID = uuid.New() }, 500},
		{"database failure", func(f *accessFixture) { f.store.err = errors.New("postgres://user:secret@host") }, 500},
		{"database deadline", func(f *accessFixture) { f.store.err = context.DeadlineExceeded }, 503},
		{"JWKS failure", func(f *accessFixture) { f.jwksStatus.Store(503) }, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccessFixture(t)
			token := f.token(t, nil)
			tc.setup(f)
			w := accessRequest(f.handler(), "GET", "/api/v1/admin/me", token, "")
			if w.Code != tc.want || strings.Contains(w.Body.String()+f.logs.String(), "secret") || strings.Contains(w.Body.String()+f.logs.String(), token) {
				t.Fatalf("unexpected access response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestShopAccessScopesWritesAndRejectsOtherUnits(t *testing.T) {
	for _, tc := range []struct {
		name, slug string
		setup      func(*accessFixture)
		want       int
	}{
		{"active membership", "shop-a", nil, 201},
		{"other shop", "shop-b", nil, 403},
		{"unknown shop", "missing", nil, 403},
		{"inactive shop", "shop-a", func(f *accessFixture) { delete(f.store.shops, "shop-a") }, 403},
		{"missing membership", "shop-a", func(f *accessFixture) { delete(f.store.memberships, f.store.shops["shop-a"]) }, 403},
		{"inactive membership", "shop-a", func(f *accessFixture) {
			id := f.store.shops["shop-a"]
			m := f.store.memberships[id]
			m.Active = false
			f.store.memberships[id] = m
		}, 403},
		{"user deactivated after authentication", "shop-a", func(f *accessFixture) { f.store.recheckInactive = true }, 403},
		{"membership identity mismatch", "shop-a", func(f *accessFixture) {
			id := f.store.shops["shop-a"]
			m := f.store.memberships[id]
			m.UserID = uuid.New()
			f.store.memberships[id] = m
		}, 500},
		{"membership shop mismatch", "shop-a", func(f *accessFixture) {
			id := f.store.shops["shop-a"]
			m := f.store.memberships[id]
			m.ShopID = uuid.New()
			f.store.memberships[id] = m
		}, 500},
		{"second active membership", "shop-b", func(f *accessFixture) {
			id := f.store.shops["shop-b"]
			f.store.memberships[id] = identitydomain.Membership{UserID: f.store.user.ID, ShopID: id, Active: true}
		}, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccessFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			w := accessRequest(f.handler(), "POST", "/api/v1/admin/shops/"+tc.slug+"/services", f.token(t, nil), serviceBody)
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if tc.want == 201 {
				if len(f.store.created) != 1 || f.store.created[0].ShopID != f.store.shops[tc.slug] {
					t.Fatal("write was not scoped to the authorized shop")
				}
			} else if len(f.store.created) != 0 {
				t.Fatal("unauthorized write reached persistence")
			}
		})
	}
}

func TestAdminServiceReadRequiresCurrentMembership(t *testing.T) {
	for _, tc := range []struct {
		name, slug string
		setup      func(*accessFixture)
		want       int
	}{
		{"own shop", "shop-a", nil, 200},
		{"other shop", "shop-b", nil, 403},
		{"unknown shop", "missing", nil, 403},
		{"revoked membership", "shop-a", func(f *accessFixture) {
			delete(f.store.memberships, f.store.shops["shop-a"])
		}, 403},
		{"deactivated user", "shop-a", func(f *accessFixture) { f.store.recheckInactive = true }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccessFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			w := accessRequest(f.handler(), "GET", "/api/v1/admin/shops/"+tc.slug+"/services", f.token(t, nil), "")
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if tc.want == 200 {
				if len(f.store.listedShopIDs) != 1 || f.store.listedShopIDs[0] != f.store.shops[tc.slug] {
					t.Fatal("read did not use the authorized tenant")
				}
			} else if len(f.store.listedShopIDs) != 0 {
				t.Fatal("denied read reached the catalog")
			}
		})
	}
}

func TestUnscopedServiceEndpointWasRemoved(t *testing.T) {
	f := newAccessFixture(t)
	for _, method := range []string{"GET", "POST", "OPTIONS"} {
		w := accessRequest(f.handler(), method, "/api/v1/admin/services", f.token(t, nil), serviceBody)
		if w.Code != 404 || len(f.store.created) != 0 || f.store.userCalls != 0 {
			t.Fatalf("unscoped endpoint remains active: %d", w.Code)
		}
	}
}

func TestPublicEndpointsAndPreflightDoNotAuthenticate(t *testing.T) {
	f := newAccessFixture(t)
	h := f.handler()
	for _, path := range []string{"/health", "/ready", "/api/v1/public/shops/shop-a/services"} {
		if w := accessRequest(h, "GET", path, "", ""); w.Code != 200 {
			t.Fatalf("public endpoint rejected: %s %d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/admin/me", "/api/v1/admin/shops", "/api/v1/admin/shops/shop-a/services"} {
		r := httptest.NewRequest("OPTIONS", path, nil)
		r.Header.Set("Origin", browserOrigin)
		method := "POST"
		if strings.HasSuffix(path, "/me") || path == "/api/v1/admin/shops" {
			method = "GET"
		}
		r.Header.Set("Access-Control-Request-Method", method)
		r.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != browserOrigin {
			t.Fatalf("preflight failed: %d %s", w.Code, w.Body)
		}
	}
	if f.jwksCalls.Load() != 0 || f.store.userCalls != 0 {
		t.Fatal("public/preflight request authenticated")
	}
	r := httptest.NewRequest("GET", "/api/v1/admin/me", nil)
	r.Header.Set("Origin", browserOrigin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("Access-Control-Allow-Origin") != browserOrigin {
		t.Fatal("browser cannot read the authentication error")
	}
}

type waitingVerifier struct{}

func (waitingVerifier) VerifyAccessToken(ctx context.Context, _ string) (uuid.UUID, error) {
	<-ctx.Done()
	return uuid.Nil, ctx.Err()
}

func TestRequestDeadlineIncludesAuthentication(t *testing.T) {
	f := newAccessFixture(t)
	h := httpapi.NewHandler(httpapi.HandlerConfig{
		Authenticate:   identityapp.NewAuthenticate(waitingVerifier{}, f.store),
		RequestTimeout: 10 * time.Millisecond, Logger: testLogger(),
	})
	w := accessRequest(h, "GET", "/api/v1/admin/me", "opaque-token", "")
	if w.Code != 503 || f.store.userCalls != 0 {
		t.Fatal("authentication ignored the request deadline")
	}
}
