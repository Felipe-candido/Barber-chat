package httpapi_test

import (
	"context"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	bookinghttp "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/http"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
	"net/http/httptest"
	"testing"
	"time"
)

type completionSettingsFake struct {
	value  app.CompletionSettings
	denied bool
	writes int
}

// These route tests isolate authorization and request validation from JWKS I/O.
// The access suite separately covers cryptographic token verification.
type completionTokenVerifier struct{ userID uuid.UUID }

func (v completionTokenVerifier) VerifyAccessToken(context.Context, string) (uuid.UUID, error) {
	return v.userID, nil
}

func (s *completionSettingsFake) WithinCompletionSettingsTx(ctx context.Context, fn func(context.Context, app.CompletionSettingsTx) error) error {
	return fn(ctx, app.CompletionSettingsTx{Access: s, Store: s})
}
func (s *completionSettingsFake) RequireCompletionSettingsAccess(_ context.Context, scope app.CompletionSettingsScope) error {
	if s.denied || scope.ShopID != s.value.ShopID {
		return app.ErrCompletionSettingsAccessDenied
	}
	return nil
}
func (s *completionSettingsFake) Find(context.Context, uuid.UUID) (app.CompletionSettings, error) {
	return s.value, nil
}
func (s *completionSettingsFake) Replace(_ context.Context, _ uuid.UUID, v int64, mode domain.CompletionMode) (bool, error) {
	if v != s.value.Version {
		return false, nil
	}
	s.writes++
	s.value.Version++
	s.value.Mode = mode
	if mode == domain.CompletionManual {
		s.value.AutomaticSince = time.Time{}
	} else {
		s.value.AutomaticSince = time.Now().UTC()
	}
	return true, nil
}
func TestCompletionSettingsRoutes(t *testing.T) {
	f := newAccessFixture(t)
	shop := f.store.shops["shop-a"]
	store := &completionSettingsFake{value: app.CompletionSettings{ShopID: shop, Mode: domain.CompletionAutomatic, Version: 1, AutomaticSince: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}
	h := httpapi.NewHandler(httpapi.HandlerConfig{CompletionSettings: bookinghttp.NewCompletionSettingsHandler(app.NewManageCompletionSettings(store), time.Second, nil), Authenticate: identity.NewAuthenticate(completionTokenVerifier{f.store.user.ID}, f.store), AuthorizeShop: identity.NewAuthorizeShopAction(f.store, f.store), FrontendOrigin: browserOrigin})
	base := "/api/v1/admin/shops/shop-a/appointment-completion"
	token := f.token(t, nil)
	for _, tc := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"GET", base, "", "", 401}, {"GET", "/api/v1/admin/shops/shop-b/appointment-completion", token, "", 403},
		{"GET", base, token, "", 200}, {"GET", base + "?shop_id=" + uuid.NewString(), token, "", 400},
		{"PUT", base, token, "null", 400}, {"PUT", base, token, `{"mode":"manual"}`, 400},
		{"PUT", base, token, `{"mode":"manual","version":1,"shop_id":"ignored"}`, 400},
		{"PUT", base, token, `{"mode":"invalid","version":1}`, 422}, {"PUT", base, token, `{"mode":"manual","version":0}`, 422},
		{"PUT", base, token, `{"mode":"manual","version":1}`, 200}, {"PUT", base, token, `{"mode":"automatic","version":1}`, 409},
	} {
		if w := accessRequest(h, tc.method, tc.path, tc.token, tc.body); w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	if store.writes != 1 {
		t.Fatal("invalid or stale write reached store", store.writes)
	}
	m := f.store.memberships[shop]
	m.Role = identitydomain.RoleBarber
	m.ProfessionalID = uuid.New()
	f.store.memberships[shop] = m
	if w := accessRequest(h, "PUT", base, token, `{"mode":"automatic","version":2}`); w.Code != 403 {
		t.Fatal("barber changed policy", w.Code, w.Body.String())
	}
	m.Role = identitydomain.RoleOwner
	m.ProfessionalID = uuid.Nil
	f.store.memberships[shop] = m
	store.denied = true
	if w := accessRequest(h, "PUT", base, token, `{"mode":"manual","version":3}`); w.Code != 403 {
		t.Fatal("revoked access", w.Code)
	}
	if store.writes != 1 {
		t.Fatal("revoked access wrote policy")
	}
	for _, method := range []string{"GET", "PUT"} {
		r := httptest.NewRequest("OPTIONS", base, nil)
		r.Header.Set("Origin", browserOrigin)
		r.Header.Set("Access-Control-Request-Method", method)
		r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal("preflight", method, w.Code)
		}
	}
	_, err := app.NewManageCompletionSettings(store).Read(context.Background(), app.CompletionSettingsScope{})
	if !errors.Is(err, app.ErrCompletionSettingsAccessDenied) {
		t.Fatal("empty context accepted", err)
	}
}
