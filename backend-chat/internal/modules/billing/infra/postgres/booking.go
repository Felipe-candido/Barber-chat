package postgres

import (
	"context"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/google/uuid"
)

// Queries must wrap the SAME transaction as the booking. The account lock is
// held through commit, serializing the decision with webhook reconciliation.
// Under READ COMMITTED the following SELECT sees a financial update committed
// while waiting for that lock. No Stripe request is made inside the transaction.
type BookingEligibility struct {
	queries      *db.Queries
	entitlements *application.GetShopEntitlements
}

func NewBookingEligibility(queries *db.Queries, clock application.Clock) *BookingEligibility {
	return &BookingEligibility{queries: queries, entitlements: application.NewGetShopEntitlements(NewReader(queries), clock)}
}

func (g *BookingEligibility) CanCreateStaffBooking(ctx context.Context, shopID uuid.UUID) (bool, error) {
	if _, err := g.queries.LockBillingOperationAccount(ctx, shopID); err != nil {
		return false, err
	}
	return g.entitlements.CanOperateShop(ctx, shopID)
}
