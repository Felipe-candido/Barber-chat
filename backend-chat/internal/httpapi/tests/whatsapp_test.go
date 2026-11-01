package httpapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/evolution"
	connectionhttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
	"github.com/google/uuid"
)

type whatsappRouteFixture struct {
	reads, calls, events int
	scope                app.ConnectionScope
	row                  domain.Connection
}

func (f *whatsappRouteFixture) ReadOwner(_ context.Context, s app.ConnectionScope) (domain.Connection, error) {
	f.reads++
	f.scope = s
	return f.row, nil
}
func (f *whatsappRouteFixture) ClaimOwner(context.Context, app.ConnectionScope, bool) (domain.Connection, error) {
	return f.row, nil
}
func (f *whatsappRouteFixture) ClaimDue(context.Context) (domain.Connection, bool, error) {
	return f.row, false, nil
}
func (f *whatsappRouteFixture) Complete(context.Context, domain.Connection, app.ConnectionObservation) (bool, error) {
	return true, nil
}
func (f *whatsappRouteFixture) MarkDirty(context.Context, string) error { f.events++; return nil }
func (f *whatsappRouteFixture) Connect(context.Context, string) (app.ConnectionChallenge, error) {
	f.calls++
	return app.ConnectionChallenge{Observation: app.ConnectionObservation{State: domain.ConnectionQRRequired}, QRBase64: "private-qr", DiscardAfter: time.Now().Add(20 * time.Second)}, nil
}
func (f *whatsappRouteFixture) Observe(context.Context, string) (app.ConnectionObservation, error) {
	return app.ConnectionObservation{State: domain.ConnectionConnected}, nil
}
func (f *whatsappRouteFixture) Disconnect(context.Context, string) (app.ConnectionObservation, error) {
	f.calls++
	return app.ConnectionObservation{State: domain.ConnectionDisconnected}, nil
}

func TestWhatsAppRoutesAreOwnerOnlyAndNeverExposeProviderIdentifiers(t *testing.T) {
	f := newAccessFixture(t)
	shop := f.store.shops["shop-a"]
	connections := &whatsappRouteFixture{row: domain.Connection{ShopID: shop, InstanceName: "bc_0123456789abcdef0123456789abcdef", Generation: 2, OperationToken: uuid.New(), State: domain.ConnectionConnected, VerifiedAt: time.Now(), ObservedAt: time.Now()}}
	secret := strings.Repeat("s", 32)
	uc := app.NewManageConnections(connections, connections)
	logger := slog.New(slog.NewJSONHandler(&f.logs, nil))
	h := httpapi.NewHandler(httpapi.HandlerConfig{WhatsApp: connectionhttp.NewConnectionHandler(uc, secret, time.Second), EvolutionWebhookEnabled: true, Authenticate: identity.NewAuthenticate(f.verifier, f.store), AuthorizeShop: identity.NewAuthorizeShopAction(f.store, f.store), FrontendOrigin: browserOrigin, Logger: logger})
	base := "/api/v1/admin/shops/shop-a/whatsapp"
	if w := accessRequest(h, "GET", base+"/connection", "", ""); w.Code != 401 || connections.reads != 0 {
		t.Fatal("anonymous reached connection data", w.Code)
	}
	token := f.token(t, nil)
	if w := accessRequest(h, "GET", strings.Replace(base, "shop-a", "shop-b", 1)+"/connection", token, ""); w.Code != 403 || connections.reads != 0 {
		t.Fatal("foreign tenant reached connection")
	}
	m := f.store.memberships[shop]
	m.Role = identitydomain.RoleBarber
	m.ProfessionalID = uuid.New()
	f.store.memberships[shop] = m
	for _, path := range []string{"connection", "connect", "disconnect"} {
		method := "POST"
		if path == "connection" {
			method = "GET"
		}
		if w := accessRequest(h, method, base+"/"+path, token, ""); w.Code != 403 || connections.reads != 0 || connections.calls != 0 {
			t.Fatal("barber reached owner connection controls", w.Code)
		}
	}
	m.Role = identitydomain.RoleOwner
	m.ProfessionalID = uuid.Nil
	f.store.memberships[shop] = m
	w := accessRequest(h, "GET", base+"/connection", token, "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), connections.row.InstanceName) || connections.scope.ActorID != f.store.user.ID || connections.scope.ShopID != shop {
		t.Fatal("unsafe connection response", w.Code)
	}
	w = accessRequest(h, "POST", base+"/connect", token, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "private-qr") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner challenge unavailable", w.Code)
	}
	for _, path := range []string{"connection", "connect", "disconnect"} {
		r := httptest.NewRequest("OPTIONS", base+"/"+path, nil)
		r.Header.Set("Origin", browserOrigin)
		method := "POST"
		if path == "connection" {
			method = "GET"
		}
		r.Header.Set("Access-Control-Request-Method", method)
		r.Header.Set("Access-Control-Request-Headers", "authorization")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		if out.Code != 204 {
			t.Fatal("missing preflight", out.Code)
		}
	}
	name := connections.row.InstanceName
	for _, valid := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/evolution", strings.NewReader(`{"event":"connection.update","instance":"`+name+`","apikey":"private-provider-key","data":{"state":"close"}}`))
		sig := "bad"
		if valid {
			sig = evolution.WebhookToken(secret, name)
		}
		r.Header.Set(evolution.WebhookHeader, sig)
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		expected := 401
		if valid {
			expected = 204
		}
		if out.Code != expected {
			t.Fatal("incorrect webhook authentication", out.Code)
		}
	}
	if connections.events != 1 || connections.row.State != domain.ConnectionConnected {
		t.Fatal("webhook changed state without active verification")
	}
	for _, sensitive := range []string{token, "private-qr", "private-provider-key", secret} {
		if strings.Contains(f.logs.String(), sensitive) {
			t.Fatal("logs exposed private connection data")
		}
	}
}
