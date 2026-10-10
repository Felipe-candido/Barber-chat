package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	identityhttp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/http"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestAccountInviteRequiresOwnerAndStrictBody(t *testing.T) {
	// Missing adapters are intentional: rejected requests must never reach them.
	h := identityhttp.NewAccountHandler(app.NewStaffAccounts(nil, nil), time.Second, nil)
	for _, tc := range []struct {
		name, body, media string
		role              domain.Role
		authenticated     bool
		want              int
	}{
		{"anonymous", `{}`, "application/json", domain.RoleOwner, false, 403},
		{"barber", `{}`, "application/json", domain.RoleBarber, true, 403},
		{"unknown role", `{}`, "application/json", domain.Role("admin"), true, 403},
		{"role injection", `{"professional_id":"22222222-2222-4222-8222-222222222222","email":"test@example.com","role":"owner"}`, "application/json", domain.RoleOwner, true, 400},
		{"extra object", `{} {}`, "application/json", domain.RoleOwner, true, 400},
		{"null", `null`, "application/json", domain.RoleOwner, true, 400},
		{"wrong type", `{}`, "text/plain", domain.RoleOwner, true, 415},
		{"oversized", `{"email":"` + strings.Repeat("a", 8192) + `"}`, "application/json", domain.RoleOwner, true, 413},
		{"no provider", `{"professional_id":"22222222-2222-4222-8222-222222222222","email":"test@example.com"}`, "application/json", domain.RoleOwner, true, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			if tc.authenticated {
				id := uuid.New()
				ctx := requestctx.WithStaffIdentity(context.Background(), app.StaffIdentityOutput{UserID: id})
				ctx = requestctx.WithShopScope(ctx, app.AuthorizedShopScope{UserID: id, ShopID: uuid.New(), Role: tc.role})
				r = r.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			h.Invite(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

type activeReplacementStore struct{ app.StaffAccountStore }

func (activeReplacementStore) ReserveReplacement(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (app.StaffAccount, error) {
	return app.StaffAccount{}, app.ErrAccountMustBeInactive
}

type forbiddenInviteProvider struct{ calls int }

func (p *forbiddenInviteProvider) Invite(context.Context, string) (app.ProviderUser, error) {
	p.calls++
	return app.ProviderUser{}, app.ErrAccountProviderUnavailable
}
func (p *forbiddenInviteProvider) FindUser(context.Context, uuid.UUID) (app.ProviderUser, error) {
	p.calls++
	return app.ProviderUser{}, app.ErrAccountProviderUnavailable
}

func TestAccountReplaceValidatesBeforeInviting(t *testing.T) {
	provider := &forbiddenInviteProvider{}
	h := identityhttp.NewAccountHandler(app.NewStaffAccounts(activeReplacementStore{}, provider), time.Second, nil)
	for _, tc := range []struct {
		name, body, id string
		want           int
	}{
		{"null", `null`, uuid.NewString(), 400},
		{"tenant injection", `{"email":"correct@example.com","shop_id":"anything"}`, uuid.NewString(), 400},
		{"professional injection", `{"email":"correct@example.com","professional_id":"anything"}`, uuid.NewString(), 400},
		{"extra object", `{"email":"correct@example.com"} {}`, uuid.NewString(), 400},
		{"empty email", `{}`, uuid.NewString(), 422},
		{"invalid UUID", `{"email":"correct@example.com"}`, "broken", 422},
		{"active access", `{"email":"correct@example.com"}`, uuid.NewString(), 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.New()
			ctx := requestctx.WithStaffIdentity(context.Background(), app.StaffIdentityOutput{UserID: id})
			ctx = requestctx.WithShopScope(ctx, app.AuthorizedShopScope{UserID: id, ShopID: uuid.New(), Role: domain.RoleOwner})
			route := chi.NewRouteContext()
			route.URLParams.Add("accountID", tc.id)
			ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.Replace(w, r)
			if w.Code != tc.want || provider.calls != 0 {
				t.Fatal(w.Code, w.Body.String(), provider.calls)
			}
			if tc.want == 409 && !strings.Contains(w.Body.String(), "staff_account_must_be_inactive") {
				t.Fatal(w.Body.String())
			}
		})
	}
}
