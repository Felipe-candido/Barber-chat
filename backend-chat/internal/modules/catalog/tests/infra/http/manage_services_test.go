package http_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
)

func TestManageServiceUsesAuthorizedShopAndResourceID(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			store := &testStore{shop: uuid.New()}
			userID, serviceID := uuid.New(), uuid.New()
			ctx := requestctx.WithStaffIdentity(context.Background(), identityapp.StaffIdentityOutput{UserID: userID})
			ctx = requestctx.WithShopScope(ctx, identityapp.AuthorizedShopScope{UserID: userID, ShopID: store.shop})
			r := httptest.NewRequest(method, "/api/v1/admin/shops/ignored/services/"+serviceID.String()+"?shop_id="+uuid.NewString(), strings.NewReader(validBody)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			testRouter(store).ServeHTTP(w, r)
			if method == "PUT" {
				if w.Code != 200 || store.updated.ID != serviceID || store.updated.ShopID != store.shop || store.updated.Name != "Corte" || !json.Valid(w.Body.Bytes()) {
					t.Fatalf("update: %d %s %+v", w.Code, w.Body, store.updated)
				}
			} else if w.Code != 204 || w.Body.Len() != 0 || store.deletedID != serviceID || store.createdShop != store.shop {
				t.Fatalf("delete: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestManageServiceRejectsInvalidInputAndMissingService(t *testing.T) {
	for _, tc := range []struct {
		name, method, id, body string
		want                   int
		missing                bool
	}{
		{"invalid ID", "PUT", "invalid", validBody, 422, false},
		{"zero ID", "DELETE", uuid.Nil.String(), "", 422, false},
		{"unknown fields", "PUT", uuid.NewString(), strings.TrimSuffix(validBody, "}") + `,"active":true}`, 400, false},
		{"missing price", "PUT", uuid.NewString(), `{"name":"Cut","duration_minutes":30}`, 422, false},
		{"invalid name", "PUT", uuid.NewString(), strings.Replace(validBody, " Corte ", " ", 1), 422, false},
		{"missing update", "PUT", uuid.NewString(), validBody, 404, true},
		{"missing delete", "DELETE", uuid.NewString(), "", 404, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{shop: uuid.New()}
			if tc.missing {
				store.err = application.ErrServiceNotFound
			}
			userID := uuid.New()
			ctx := requestctx.WithStaffIdentity(context.Background(), identityapp.StaffIdentityOutput{UserID: userID})
			ctx = requestctx.WithShopScope(ctx, identityapp.AuthorizedShopScope{UserID: userID, ShopID: store.shop})
			r := httptest.NewRequest(tc.method, "/api/v1/admin/shops/shop/services/"+tc.id, strings.NewReader(tc.body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			testRouter(store).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body)
			}
			if !tc.missing && (store.updated.ID != uuid.Nil || store.deletedID != uuid.Nil) {
				t.Fatal("invalid input reached persistence")
			}
		})
	}
}
