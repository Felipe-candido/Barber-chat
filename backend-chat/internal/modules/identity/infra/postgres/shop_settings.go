package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Identity owns membership checks; shops consumes only the narrow access port.
type SettingsOwnerGuard struct{ queries *db.Queries }

var _ shops.SettingsOwnerAccess = (*SettingsOwnerGuard)(nil)

func NewSettingsOwnerGuard(queries *db.Queries) *SettingsOwnerGuard {
	return &SettingsOwnerGuard{queries: queries}
}

func (g *SettingsOwnerGuard) RequireSettingsOwner(ctx context.Context, actorID, shopID uuid.UUID) error {
	_, err := g.queries.LockShopSettingsOwner(ctx, db.LockShopSettingsOwnerParams{UserID: actorID, ShopID: shopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return shops.ErrSettingsAccessDenied
	}
	return err
}
