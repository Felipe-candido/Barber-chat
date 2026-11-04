package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/customers/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	customers "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	contactshttp "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra/http"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type contactsStore struct {
	scope   app.ContactScope
	rows    []domain.Contact
	denied  bool
	failure error
}

func (s *contactsStore) WithinContactTx(ctx context.Context, fn func(context.Context, app.ContactTx) error) error {
	if s.failure != nil {
		return s.failure
	}
	return fn(ctx, app.ContactTx{Access: s, Contacts: s})
}
func (s *contactsStore) RequireContactsAccess(_ context.Context, scope app.ContactScope) error {
	s.scope = scope
	if s.denied {
		return app.ErrContactAccessDenied
	}
	return nil
}
func (s *contactsStore) Create(_ context.Context, c domain.Contact) (domain.Contact, error) {
	for _, old := range s.rows {
		if old.ShopID == c.ShopID && old.Phone == c.Phone {
			return domain.Contact{}, app.ErrContactExists
		}
	}
	c.CreatedAt = time.Now().UTC()
	s.rows = append(s.rows, c)
	return c, nil
}
func (s *contactsStore) Find(_ context.Context, shop, id uuid.UUID) (domain.Contact, bool, error) {
	for _, c := range s.rows {
		if c.ShopID == shop && c.ID == id {
			return c, true, nil
		}
	}
	return domain.Contact{}, false, nil
}
func (s *contactsStore) List(_ context.Context, shop uuid.UUID, _ app.ContactQuery) ([]domain.Contact, error) {
	out := []domain.Contact{}
	for _, c := range s.rows {
		if c.ShopID == shop {
			out = append(out, c)
		}
	}
	return out, nil
}

func TestContactRoutes(t *testing.T) {
	f := newAccessFixture(t)
	store := &contactsStore{}
	var logs bytes.Buffer
	h := httpapi.NewHandler(httpapi.HandlerConfig{Contacts: contactshttp.NewContactHandler(app.NewManageContacts(store, customers.ContactNormalizer{}), time.Second, slog.New(slog.NewJSONHandler(&logs, nil))), Authenticate: identity.NewAuthenticate(f.verifier, f.store), AuthorizeShop: identity.NewAuthorizeShopAction(f.store, f.store), FrontendOrigin: browserOrigin})
	base := "/api/v1/admin/shops/shop-a/contacts"
	token := f.token(t, nil)
	if w := accessRequest(h, "GET", base, "", ""); w.Code != 401 {
		t.Fatal("anonymous access", w.Code)
	}
	if w := accessRequest(h, "GET", "/api/v1/admin/shops/shop-b/contacts", token, ""); w.Code != 403 {
		t.Fatal("foreign shop access", w.Code)
	}
	r := httptest.NewRequest("OPTIONS", base, nil)
	r.Header.Set("Origin", browserOrigin)
	r.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("missing preflight", w.Code)
	}
	body := `{"first_name":"Single","phone":"11912345678","phone_country":"BR"}`
	w = accessRequest(h, "POST", base, token, body)
	if w.Code != 201 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner creation", w.Code)
	}
	var c struct {
		ID       uuid.UUID `json:"id"`
		Name     string    `json:"name"`
		LastName string    `json:"last_name"`
		ShopID   uuid.UUID `json:"shop_id"`
	}
	if json.Unmarshal(w.Body.Bytes(), &c) != nil || c.Name != "Single" || c.LastName != "" || c.ShopID != f.store.shops["shop-a"] {
		t.Fatal("wrong contact representation")
	}
	membership := f.store.memberships[c.ShopID]
	membership.Role = identitydomain.RoleBarber
	membership.ProfessionalID = uuid.New()
	f.store.memberships[c.ShopID] = membership
	for _, path := range []string{base, base + "/" + c.ID.String()} {
		if w := accessRequest(h, "GET", path, token, ""); w.Code != 200 {
			t.Fatal("barber shared directory", w.Code)
		}
	}
	if w := accessRequest(h, "POST", base, token, strings.ReplaceAll(body, "11912345678", "11987654321")); w.Code != 201 {
		t.Fatal("barber manual create", w.Code)
	}
	if w := accessRequest(h, "POST", base, token, body); w.Code != 409 {
		t.Fatal("duplicate", w.Code)
	}
	if w := accessRequest(h, "GET", base+"/"+uuid.NewString(), token, ""); w.Code != 404 {
		t.Fatal("unknown contact", w.Code)
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", base, `null`, 400}, {"POST", base, `[]`, 400}, {"POST", base, `{"first_name":"X","shop_id":"` + uuid.NewString() + `"}`, 400},
		{"POST", base, `{"first_name":"","phone":"11912345678","phone_country":"BR"}`, 422},
		{"GET", base + "?limit=1&limit=2", "", 400}, {"GET", base + "?unknown=1", "", 400}, {"GET", base + "?cursor=bad", "", 400}, {"GET", base + "?phone=123&phone_country=BR", "", 400},
	} {
		if w := accessRequest(h, tc.method, tc.path, token, tc.body); w.Code != tc.status {
			t.Fatal("invalid request", tc.path, w.Code, tc.status)
		}
	}
	store.denied = true
	if w := accessRequest(h, "GET", base, token, ""); w.Code != 403 {
		t.Fatal("current access not revalidated", w.Code)
	}
	store.denied = false
	store.failure = errors.New("private-provider-contact-data")
	w = accessRequest(h, "GET", base, token, "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-provider") || strings.Contains(logs.String(), "private-provider") {
		t.Fatal("failure response leaked private data")
	}
}
