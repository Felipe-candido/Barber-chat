package httpapi_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	notificationhttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
	"github.com/google/uuid"
)

type notificationSettingsFixture struct{ reads, writes int }

func (*notificationSettingsFixture) WorkerAvailable(context.Context) (bool, error) { return false, nil }

func (f *notificationSettingsFixture) Read(context.Context, app.ConnectionScope) (app.NotificationSettings, error) {
	f.reads++
	return app.NotificationSettings{DeliveryMode: "request", ReminderLeadMinutes: 1440, Version: 1}, nil
}
func (f *notificationSettingsFixture) Save(_ context.Context, _ app.ConnectionScope, s app.NotificationSettings) (app.NotificationSettings, error) {
	f.writes++
	s.Version++
	return s, nil
}
func (*notificationSettingsFixture) WhatsAppEnabled(context.Context, string) (bool, error) {
	return true, nil
}
func TestNotificationSettingsRoutesAndPreflight(t *testing.T) {
	f := newAccessFixture(t)
	store := &notificationSettingsFixture{}
	handler := notificationhttp.NewMessageHandler(app.NewManageNotificationSettings(store, true, false), nil, store, true, time.Second)
	h := httpapi.NewHandler(httpapi.HandlerConfig{Notifications: handler, Authenticate: identity.NewAuthenticate(f.verifier, f.store), AuthorizeShop: identity.NewAuthorizeShopAction(f.store, f.store), FrontendOrigin: browserOrigin})
	base := "/api/v1/admin/shops/shop-a"
	token := f.token(t, nil)
	if w := accessRequest(h, "GET", base+"/notifications/settings", "", ""); w.Code != 401 {
		t.Fatal("anonymous settings access", w.Code)
	}
	if w := accessRequest(h, "GET", "/api/v1/admin/shops/shop-b/notifications/settings", token, ""); w.Code != 403 {
		t.Fatal("foreign settings access", w.Code)
	}
	shop := f.store.shops["shop-a"]
	membership := f.store.memberships[shop]
	membership.Role = identitydomain.RoleBarber
	membership.ProfessionalID = uuid.New()
	f.store.memberships[shop] = membership
	if w := accessRequest(h, "GET", base+"/notifications/settings", token, ""); w.Code != 403 || store.reads != 0 {
		t.Fatal("barber read owner settings", w.Code)
	}
	if w := accessRequest(h, "GET", base+"/notifications/options", token, ""); w.Code != 200 {
		t.Fatal("barber cannot read consent availability", w.Code)
	}
	membership.Role = identitydomain.RoleOwner
	membership.ProfessionalID = uuid.Nil
	f.store.memberships[shop] = membership
	if w := accessRequest(h, "GET", base+"/notifications/settings", token, ""); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner settings unavailable", w.Code)
	}
	for _, body := range []string{`{}`, `null`, `{"enabled":true,"delivery_mode":"request","reminder_lead_minutes":1440,"version":1,"instance":"untrusted"}`} {
		if w := accessRequest(h, "PUT", base+"/notifications/settings", token, body); w.Code != 400 {
			t.Fatal("invalid settings body accepted", w.Code)
		}
	}
	if w := accessRequest(h, "PUT", base+"/notifications/settings", token, `{"enabled":true,"delivery_mode":"request","reminder_lead_minutes":1440,"version":1}`); w.Code != 200 || store.writes != 1 {
		t.Fatal("valid owner write failed", w.Code)
	}
	routes := map[string]string{"/notifications/settings": "PUT", "/notifications/options": "GET", "/appointments/" + uuid.NewString() + "/notifications": "GET", "/appointments/" + uuid.NewString() + "/notifications/confirmation": "POST", "/appointments/" + uuid.NewString() + "/notifications/reminder": "POST"}
	for path, method := range routes {
		r := httptest.NewRequest("OPTIONS", base+path, nil)
		r.Header.Set("Origin", browserOrigin)
		r.Header.Set("Access-Control-Request-Method", method)
		r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal("missing notification preflight", path, w.Code)
		}
	}
}
