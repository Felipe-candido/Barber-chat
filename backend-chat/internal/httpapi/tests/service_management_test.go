package httpapi_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	identitydomain "github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

func TestServiceChangesRequireOwnerOfTheShop(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE"} {
		for _, role := range []identitydomain.Role{identitydomain.RoleOwner, identitydomain.RoleBarber} {
			for _, slug := range []string{"shop-a", "shop-b"} {
				t.Run(method+"/"+string(role)+"/"+slug, func(t *testing.T) {
					f := newAccessFixture(t)
					shopID := f.store.shops["shop-a"]
					membership := f.store.memberships[shopID]
					membership.Role = role
					if role == identitydomain.RoleBarber {
						membership.ProfessionalID = uuid.New()
					}
					f.store.memberships[shopID] = membership
					w := accessRequest(f.handler(), method, "/api/v1/admin/shops/"+slug+"/services/"+uuid.NewString(), f.token(t, nil), serviceBody)
					allowed := role == identitydomain.RoleOwner && slug == "shop-a"
					if allowed {
						want := 200
						if method == "DELETE" {
							want = 204
						}
						if w.Code != want {
							t.Fatalf("%d: %s", w.Code, w.Body)
						}
					} else if w.Code != 403 || len(f.store.updated)+len(f.store.deleted) != 0 {
						t.Fatalf("unauthorized mutation: %d %s", w.Code, w.Body)
					}
				})
			}
		}
	}
}

func TestServiceMutationPreflight(t *testing.T) {
	f := newAccessFixture(t)
	for _, method := range []string{"PUT", "DELETE"} {
		r := httptest.NewRequest("OPTIONS", "/api/v1/admin/shops/shop-a/services/"+uuid.NewString(), nil)
		r.Header.Set("Origin", browserOrigin)
		r.Header.Set("Access-Control-Request-Method", method)
		r.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
		w := httptest.NewRecorder()
		f.handler().ServeHTTP(w, r)
		if w.Code != 204 || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), method) {
			t.Fatalf("%s: %d %s", method, w.Code, w.Body)
		}
	}
}
