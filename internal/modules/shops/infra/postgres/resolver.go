package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Resolver struct{ queries *db.Queries }

func NewResolver(queries *db.Queries) *Resolver { return &Resolver{queries: queries} }
func (r *Resolver) LookupActiveShop(ctx context.Context, slug string) (uuid.UUID, bool, error) {
	id, err := r.queries.GetActiveShopIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}
