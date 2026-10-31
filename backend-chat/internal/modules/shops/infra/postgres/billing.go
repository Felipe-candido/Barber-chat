package postgres

import (
	"context"
	"errors"

	billing "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Shops owns the active tenant/slug/provisioning projection, not billing.
func (r *Resolver) FindCheckoutShop(ctx context.Context, shopID uuid.UUID) (billing.CheckoutShop, bool, error) {
	row, err := r.queries.LockBillingCheckoutShop(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.CheckoutShop{}, false, nil
	}
	if err != nil {
		return billing.CheckoutShop{}, false, err
	}
	return billing.CheckoutShop{ShopID: row.ID, Slug: row.Slug, SelfService: row.ProvisioningSource == "self_service"}, true, nil
}
