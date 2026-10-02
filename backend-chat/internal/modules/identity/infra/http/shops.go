package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
)

type accessibleShopResponse struct {
	ShopID uuid.UUID `json:"shop_id"`
	Name   string    `json:"name"`
	Slug   string    `json:"slug"`
}

// ListShops returns a handler bound to the shared use case and operation timeout.
// User identity comes only from authentication, never from client parameters.
func ListShops(uc *application.ListMyShops, timeout time.Duration, logger *slog.Logger) http.HandlerFunc {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := requestctx.StaffIdentityFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
			response.Error(w, http.StatusUnauthorized, "authentication_required", "authentication is required")
			return
		}
		if uc == nil {
			response.Error(w, http.StatusServiceUnavailable, "temporarily_unavailable", "shop selection is unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		shops, err := uc.Execute(ctx, application.ListMyShopsInput{UserID: identity.UserID})
		if err != nil {
			WriteAccessError(w, r.WithContext(ctx), logger, "list_my_shops", err)
			return
		}

		items := make([]accessibleShopResponse, 0, len(shops))
		for _, shop := range shops {
			items = append(items, accessibleShopResponse{ShopID: shop.ShopID, Name: shop.Name, Slug: shop.Slug})
		}
		response.JSON(w, http.StatusOK, items)
	}
}
