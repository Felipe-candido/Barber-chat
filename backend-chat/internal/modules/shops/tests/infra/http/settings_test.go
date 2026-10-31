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

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	shopshttp "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/http"
	"github.com/google/uuid"
)

type settingsStore struct {
	shop    uuid.UUID
	version int64
	writes  int
	err     error
	address *domain.Address
}

func (s *settingsStore) RequireSettingsOwner(context.Context, uuid.UUID, uuid.UUID) error {
	return s.err
}
func (s *settingsStore) FindSettings(context.Context, uuid.UUID) (app.ShopSettings, bool, error) {
	return app.ShopSettings{ShopID: s.shop, Name: "Shop", Slug: "my-shop", Timezone: "America/Sao_Paulo", Version: s.version, Address: s.address}, true, nil
}
func (s *settingsStore) ReplaceSettings(_ context.Context, _ uuid.UUID, _ int64, settings domain.BasicSettings) (bool, error) {
	s.writes++
	s.version++
	s.address = &settings.Address
	return true, nil
}
func (s *settingsStore) WithinShopSettingsTx(ctx context.Context, fn func(context.Context, app.ShopSettingsTx) error) error {
	return fn(ctx, app.ShopSettingsTx{Access: s, Shops: s})
}

func TestSettingsHTTPBoundary(t *testing.T) {
	valid := `{"version":1,"name":"Shop","phone":"","description":"","address":{"postal_code":"01310100","street":"Avenida Paulista","number":"100","district":"Bela Vista","city":"São Paulo","state":"SP"}}`
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"valid", valid, 200},
		{"empty contact is explicit", strings.Replace(valid, `"phone":"",`, "", 1), 422},
		{"address required", strings.Replace(valid, `"address":{`, `"address":null,"ignored":{`, 1), 400},
		{"empty address", `{"version":1,"name":"Shop","phone":"","description":"","address":{}}`, 422},
		{"null address", `{"version":1,"name":"Shop","phone":"","description":"","address":null}`, 422},
		{"missing version", strings.Replace(valid, `"version":1,`, "", 1), 422},
		{"invalid version", strings.Replace(valid, `"version":1`, `"version":0`, 1), 422},
		{"fractional version", strings.Replace(valid, `"version":1`, `"version":1.5`, 1), 400},
		{"tenant injection", strings.Replace(valid, `"version":1`, `"shop_id":"`+uuid.NewString()+`","version":1`, 1), 400},
		{"cannot publish", strings.Replace(valid, `"version":1`, `"publication_requested":true,"version":1`, 1), 400},
		{"cannot change slug", strings.Replace(valid, `"version":1`, `"slug":"new-link","version":1`, 1), 400},
		{"cannot change timezone", strings.Replace(valid, `"version":1`, `"timezone":"UTC","version":1`, 1), 400},
		{"extra object", valid + ` {}`, 400}, {"null body", "null", 400},
		{"oversized", `{"description":"` + strings.Repeat("x", 9000) + `"}`, 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			actor, shop := uuid.New(), uuid.New()
			s := &settingsStore{shop: shop, version: 1}
			h := shopshttp.NewSettingsHandler(app.NewGetShopSettings(s), app.NewUpdateShopSettings(s), 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("PUT", "/", strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/json")
			ctx := requestctx.WithStaffIdentity(r.Context(), identity.StaffIdentityOutput{UserID: actor})
			ctx = requestctx.WithShopScope(ctx, identity.AuthorizedShopScope{UserID: actor, ShopID: shop, Role: identitydomain.RoleOwner})
			w := httptest.NewRecorder()
			h.Update(w, r.WithContext(ctx))
			if w.Code != test.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if test.status != 200 && s.writes != 0 {
				t.Fatal("rejected input wrote data")
			}
			if test.status == 200 {
				var result map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &result)
				if result["version"] != float64(2) || result["address"] == nil || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal(result, w.Header())
				}
			}
		})
	}
}

func TestSettingsHTTPAccessAndFailures(t *testing.T) {
	s := &settingsStore{shop: uuid.New(), version: 1}
	h := shopshttp.NewSettingsHandler(app.NewGetShopSettings(s), app.NewUpdateShopSettings(s), 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	actor := uuid.New()
	for _, role := range []identitydomain.Role{"", identitydomain.RoleBarber, identitydomain.RoleOwner} {
		for _, method := range []string{"GET", "PUT"} {
			r := httptest.NewRequest(method, "/", nil)
			ctx := requestctx.WithStaffIdentity(r.Context(), identity.StaffIdentityOutput{UserID: actor})
			ctx = requestctx.WithShopScope(ctx, identity.AuthorizedShopScope{UserID: actor, ShopID: s.shop, Role: role})
			w := httptest.NewRecorder()
			if method == "GET" {
				h.Get(w, r.WithContext(ctx))
			} else {
				h.Update(w, r.WithContext(ctx))
			}
			want := 403
			if role == identitydomain.RoleOwner {
				want = 200
				if method == "PUT" {
					want = 415
				}
			}
			if w.Code != want {
				t.Fatal(role, method, w.Code)
			}
		}
	}
	for _, test := range []struct {
		err    error
		status int
	}{
		{app.ErrSettingsAccessDenied, 403}, {app.ErrSettingsConflict, 409},
		{app.ErrSettingsNotFound, 404}, {errors.New("sensitive SQL phone secret"), 503},
	} {
		s.err = test.err
		r := httptest.NewRequest("GET", "/", nil)
		ctx := requestctx.WithStaffIdentity(r.Context(), identity.StaffIdentityOutput{UserID: actor})
		ctx = requestctx.WithShopScope(ctx, identity.AuthorizedShopScope{UserID: actor, ShopID: s.shop, Role: identitydomain.RoleOwner})
		w := httptest.NewRecorder()
		h.Get(w, r.WithContext(ctx))
		if w.Code != test.status || strings.Contains(w.Body.String(), "sensitive") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	handler := httpapi.NewHandler(httpapi.HandlerConfig{ShopSettings: h, FrontendOrigin: "http://127.0.0.1:3000"})
	r := httptest.NewRequest("OPTIONS", "/api/v1/admin/shops/my-shop/settings", nil)
	r.Header.Set("Origin", "http://127.0.0.1:3000")
	r.Header.Set("Access-Control-Request-Method", "PUT")
	r.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatal(w.Code, w.Header())
	}
}
