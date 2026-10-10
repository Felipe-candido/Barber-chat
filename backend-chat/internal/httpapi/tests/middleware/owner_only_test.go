package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/middleware"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

func TestOwnerOnly(t *testing.T) {
	userID, shopID := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name     string
		identity application.StaffIdentityOutput
		scope    application.AuthorizedShopScope
		want     int
	}{
		{"owner", application.StaffIdentityOutput{UserID: userID}, application.AuthorizedShopScope{UserID: userID, ShopID: shopID, Role: domain.RoleOwner}, 204},
		{"barber", application.StaffIdentityOutput{UserID: userID}, application.AuthorizedShopScope{UserID: userID, ShopID: shopID, Role: domain.RoleBarber, ProfessionalID: uuid.New()}, 403},
		{"missing identity", application.StaffIdentityOutput{}, application.AuthorizedShopScope{UserID: userID, ShopID: shopID, Role: domain.RoleOwner}, 403},
		{"missing scope", application.StaffIdentityOutput{UserID: userID}, application.AuthorizedShopScope{}, 403},
		{"other user", application.StaffIdentityOutput{UserID: uuid.New()}, application.AuthorizedShopScope{UserID: userID, ShopID: shopID, Role: domain.RoleOwner}, 403},
		{"missing shop", application.StaffIdentityOutput{UserID: userID}, application.AuthorizedShopScope{UserID: userID, Role: domain.RoleOwner}, 403},
		{"unknown role", application.StaffIdentityOutput{UserID: userID}, application.AuthorizedShopScope{UserID: userID, ShopID: shopID, Role: "admin"}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := requestctx.WithStaffIdentity(context.Background(), tc.identity)
			ctx = requestctx.WithShopScope(ctx, tc.scope)
			nextCalled := false
			h := middleware.OwnerOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { nextCalled = true; w.WriteHeader(204) }))
			r := httptest.NewRequest("POST", "/", nil).WithContext(ctx)
			// Browser-controlled hints must never replace the verified role.
			r.Header.Set("X-Role", "owner")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || nextCalled != (tc.want == 204) {
				t.Fatal("unexpected authorization", w.Code, nextCalled)
			}
		})
	}
}
