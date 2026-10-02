package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type eligibleShopReaderFunc func(context.Context, uuid.UUID) ([]identityapp.AccessibleShopOutput, error)

func (f eligibleShopReaderFunc) ListActiveShopsByUser(ctx context.Context, id uuid.UUID) ([]identityapp.AccessibleShopOutput, error) {
	return f(ctx, id)
}

func myShopsHandler(f *accessFixture, reader eligibleShopReaderFunc, dbTimeout, requestTimeout time.Duration) http.Handler {
	return httpapi.NewHandler(httpapi.HandlerConfig{
		Authenticate: identityapp.NewAuthenticate(f.verifier, f.store),
		ListMyShops:  identityapp.NewListMyShops(f.store, reader),
		Logger:       slog.New(slog.NewJSONHandler(&f.logs, nil)), FrontendOrigin: browserOrigin,
		DBTimeout: dbTimeout, RequestTimeout: requestTimeout,
	})
}

func TestMyShopsUsesAuthenticatedIdentityAndSharesVerifierWithMe(t *testing.T) {
	f := newAccessFixture(t)
	shopID := f.store.shops["shop-a"]
	readerCalls := 0
	reader := eligibleShopReaderFunc(func(ctx context.Context, userID uuid.UUID) ([]identityapp.AccessibleShopOutput, error) {
		readerCalls++
		if userID != f.store.user.ID {
			t.Fatal("shop selection trusted a client-supplied user ID")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("shop selection has no operation deadline")
		}
		return []identityapp.AccessibleShopOutput{{ShopID: shopID, Name: "Shop A", Slug: "shop-a"}}, nil
	})
	h := myShopsHandler(f, reader, time.Second, time.Second)
	token := f.token(t, nil)
	if w := accessRequest(h, "GET", "/api/v1/admin/me", token, ""); w.Code != 200 {
		t.Fatalf("me failed: %d %s", w.Code, w.Body)
	}
	w := accessRequest(h, "GET", "/api/v1/admin/shops?user_id="+uuid.NewString(), token, "")
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(items) != 1 || len(items[0]) != 3 || items[0]["shop_id"] != shopID.String() || items[0]["name"] != "Shop A" || items[0]["slug"] != "shop-a" {
		t.Fatalf("unexpected shop selection: %d %s", w.Code, w.Body)
	}
	if readerCalls != 1 || f.store.userCalls != 3 || f.jwksCalls.Load() != 1 {
		t.Fatal("identity must be reread while the verifier/JWKS cache is shared")
	}
	if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String()+f.logs.String(), token) {
		t.Fatal("shop selection cached or exposed the access token")
	}
}

func TestMyShopsWithoutMembershipsReturnsEmptyList(t *testing.T) {
	f := newAccessFixture(t)
	f.store.memberships = nil
	reader := eligibleShopReaderFunc(func(context.Context, uuid.UUID) ([]identityapp.AccessibleShopOutput, error) { return nil, nil })
	w := accessRequest(myShopsHandler(f, reader, 0, 0), "GET", "/api/v1/admin/shops", f.token(t, nil), "")
	if w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatalf("user without memberships: %d %s", w.Code, w.Body)
	}
}

func TestMyShopsDenialsAndFailures(t *testing.T) {
	for _, tt := range []struct {
		name        string
		setup       func(*accessFixture)
		tokenChange func(jwt.MapClaims)
		readerErr   error
		want        int
		wantReader  bool
	}{
		{name: "expired token", tokenChange: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, want: 401},
		{name: "unprovisioned", setup: func(f *accessFixture) { f.store.userFound = false }, want: 403},
		{name: "inactive", setup: func(f *accessFixture) { f.store.user.Active = false }, want: 403},
		{name: "deactivated after authentication", setup: func(f *accessFixture) { f.store.recheckInactive = true }, want: 403},
		{name: "JWKS unavailable", setup: func(f *accessFixture) { f.jwksStatus.Store(503) }, want: 503},
		{name: "reader failure", readerErr: errors.New("postgres://user:private-database-password@host"), want: 500, wantReader: true},
		{name: "reader deadline", readerErr: context.DeadlineExceeded, want: 503, wantReader: true},
		{name: "reader canceled", readerErr: context.Canceled, want: 503, wantReader: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAccessFixture(t)
			token := f.token(t, tt.tokenChange)
			if tt.setup != nil {
				tt.setup(f)
			}
			readerCalls := 0
			reader := eligibleShopReaderFunc(func(context.Context, uuid.UUID) ([]identityapp.AccessibleShopOutput, error) {
				readerCalls++
				return nil, tt.readerErr
			})
			w := accessRequest(myShopsHandler(f, reader, 0, 0), "GET", "/api/v1/admin/shops", token, "")
			if w.Code != tt.want || (readerCalls == 1) != tt.wantReader {
				t.Fatalf("response=%d reader calls=%d, want %d and called=%v", w.Code, readerCalls, tt.want, tt.wantReader)
			}
			if strings.Contains(w.Body.String()+f.logs.String(), "private-database-password") || strings.Contains(w.Body.String()+f.logs.String(), token) {
				t.Fatal("shop selection exposed credentials in errors or logs")
			}
		})
	}
}

func TestMyShopsPreflightAndMissingCredentials(t *testing.T) {
	f := newAccessFixture(t)
	reader := eligibleShopReaderFunc(func(context.Context, uuid.UUID) ([]identityapp.AccessibleShopOutput, error) {
		t.Fatal("preflight or unauthenticated request reached shop selection")
		return nil, nil
	})
	h := myShopsHandler(f, reader, 0, 0)
	r := httptest.NewRequest("OPTIONS", "/api/v1/admin/shops", nil)
	r.Header.Set("Origin", browserOrigin)
	r.Header.Set("Access-Control-Request-Method", "GET")
	r.Header.Set("Access-Control-Request-Headers", "authorization")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Methods") != "GET" {
		t.Fatalf("preflight failed: %d %s", w.Code, w.Body)
	}
	if w := accessRequest(h, "GET", "/api/v1/admin/shops", "", ""); w.Code != 401 {
		t.Fatalf("missing credentials: %d", w.Code)
	}
	if f.store.userCalls != 0 || f.jwksCalls.Load() != 0 {
		t.Fatal("preflight/missing credentials reached dependencies")
	}
	if w := accessRequest(h, "POST", "/api/v1/admin/shops", "", ""); w.Code != 405 {
		t.Fatalf("unexpected method accepted: %d", w.Code)
	}
}

func TestMyShopsMissingDependencyFailsClosed(t *testing.T) {
	f := newAccessFixture(t)
	w := accessRequest(f.handler(), "GET", "/api/v1/admin/shops", f.token(t, nil), "")
	if w.Code != 503 {
		t.Fatalf("missing use case granted access: %d", w.Code)
	}
	for _, validIdentity := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/api/v1/admin/shops", nil)
		if validIdentity {
			r = r.WithContext(requestctx.WithStaffIdentity(r.Context(), identityapp.StaffIdentityOutput{UserID: uuid.New()}))
		}
		w := httptest.NewRecorder()
		identityhttp.ListShops(nil, 0, testLogger()).ServeHTTP(w, r)
		want := 401
		if validIdentity {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("direct handler status=%d, want %d", w.Code, want)
		}
	}
}

func TestMyShopsHonorsOperationAndRequestDeadlines(t *testing.T) {
	for _, operationDeadline := range []bool{false, true} {
		f := newAccessFixture(t)
		token := f.token(t, nil)
		if _, err := f.verifier.VerifyAccessToken(context.Background(), token); err != nil {
			t.Fatal(err)
		}
		entered := false
		reader := eligibleShopReaderFunc(func(ctx context.Context, _ uuid.UUID) ([]identityapp.AccessibleShopOutput, error) {
			entered = true
			<-ctx.Done()
			return nil, ctx.Err()
		})
		dbTimeout, requestTimeout := time.Second, 10*time.Millisecond
		if operationDeadline {
			dbTimeout, requestTimeout = requestTimeout, dbTimeout
		}
		w := accessRequest(myShopsHandler(f, reader, dbTimeout, requestTimeout), "GET", "/api/v1/admin/shops", token, "")
		if !entered || w.Code != 503 {
			t.Fatalf("deadline not enforced: reader entered=%v status=%d", entered, w.Code)
		}
	}
}
