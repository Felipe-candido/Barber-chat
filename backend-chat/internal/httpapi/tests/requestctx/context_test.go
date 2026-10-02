package requestctx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
)

func TestMissingIdentityAndScope(t *testing.T) {
	ctx := t.Context()
	if identity, ok := requestctx.StaffIdentityFromContext(ctx); ok || identity != (identityapp.StaffIdentityOutput{}) {
		t.Fatalf("missing identity returned (%v, %v)", identity, ok)
	}
	if scope, ok := requestctx.ShopScopeFromContext(ctx); ok || scope != (identityapp.AuthorizedShopScope{}) {
		t.Fatalf("missing shop scope returned (%v, %v)", scope, ok)
	}
}

func TestIdentityDoesNotGrantShopScope(t *testing.T) {
	identity := identityapp.StaffIdentityOutput{UserID: uuid.New(), DisplayName: "Staff"}
	ctx := requestctx.WithStaffIdentity(t.Context(), identity)
	if _, ok := requestctx.ShopScopeFromContext(ctx); ok {
		t.Fatal("authentication alone must not create a shop scope")
	}
}

func TestRequestValuesReachHandlerWithoutLeakingToOtherRequests(t *testing.T) {
	parent := t.Context()
	first := identityapp.StaffIdentityOutput{UserID: uuid.New(), DisplayName: "First"}
	second := identityapp.StaffIdentityOutput{UserID: uuid.New(), DisplayName: "Second"}
	firstScope := identityapp.AuthorizedShopScope{UserID: first.UserID, ShopID: uuid.New()}
	secondScope := identityapp.AuthorizedShopScope{UserID: second.UserID, ShopID: uuid.New()}

	for _, tc := range []struct {
		name     string
		identity identityapp.StaffIdentityOutput
		scope    identityapp.AuthorizedShopScope
	}{
		{"first request", first, firstScope},
		{"second request", second, secondScope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity, ok := requestctx.StaffIdentityFromContext(r.Context())
				if !ok || identity != tc.identity {
					t.Fatalf("handler received identity (%v, %v), want %v", identity, ok, tc.identity)
				}
				scope, ok := requestctx.ShopScopeFromContext(r.Context())
				if !ok || scope != tc.scope {
					t.Fatalf("handler received shop scope (%v, %v), want %v", scope, ok, tc.scope)
				}
				w.WriteHeader(http.StatusNoContent)
			})
			// Model the hand-off after the use cases have verified these results.
			middleware := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := requestctx.WithStaffIdentity(r.Context(), tc.identity)
				ctx = requestctx.WithShopScope(ctx, tc.scope)
				handler.ServeHTTP(w, r.WithContext(ctx))
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)
			recorder := httptest.NewRecorder()
			middleware.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("unexpected response: %d", recorder.Code)
			}
			if _, ok := requestctx.StaffIdentityFromContext(request.Context()); ok {
				t.Fatal("identity leaked back into the original request")
			}
			if _, ok := requestctx.ShopScopeFromContext(request.Context()); ok {
				t.Fatal("shop scope leaked back into the original request")
			}
		})
	}
}

func TestRequestValuesPreserveDeadlineAndCancellation(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	identity := identityapp.StaffIdentityOutput{UserID: uuid.New(), DisplayName: "Staff"}
	scope := identityapp.AuthorizedShopScope{UserID: identity.UserID, ShopID: uuid.New()}
	ctx := requestctx.WithStaffIdentity(parent, identity)
	ctx = requestctx.WithShopScope(ctx, scope)

	if got, ok := ctx.Deadline(); !ok || !got.Equal(deadline) {
		t.Fatalf("deadline lost: (%v, %v)", got, ok)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("request canceled prematurely: %v", err)
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("parent cancellation was not propagated: %v", ctx.Err())
	}
}
