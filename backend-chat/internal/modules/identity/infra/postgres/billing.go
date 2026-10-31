package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	billing "github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BillingOwnerGuard struct{ queries *db.Queries }

var _ billing.BillingOwnerAccess = (*BillingOwnerGuard)(nil)

func NewBillingOwnerGuard(queries *db.Queries) *BillingOwnerGuard {
	return &BillingOwnerGuard{queries: queries}
}

func (g *BillingOwnerGuard) RequireBillingOwner(ctx context.Context, actorID, shopID uuid.UUID) error {
	_, err := g.queries.LockBillingOwner(ctx, db.LockBillingOwnerParams{UserID: actorID, ShopID: shopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.ErrBillingAccessDenied
	}
	return err
}
