package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) RequireProfessionalOwner(ctx context.Context, actor, shop uuid.UUID) error {
	_, err := r.queries.LockShopSettingsOwner(ctx, db.LockShopSettingsOwnerParams{UserID: actor, ShopID: shop})
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrProfessionalAccessDenied
	}
	return err
}
