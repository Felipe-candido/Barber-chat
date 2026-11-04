package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	contacts "github.com/Felipe-candido/Barber-chat/internal/modules/customers/application"
	"github.com/jackc/pgx/v5"
)

type ContactAccess struct{ q *db.Queries }

func NewContactAccess(q *db.Queries) *ContactAccess { return &ContactAccess{q} }
func (g *ContactAccess) RequireContactsAccess(ctx context.Context, s contacts.ContactScope) error {
	_, err := g.q.LockCustomerContactsAccess(ctx, db.LockCustomerContactsAccessParams{UserID: s.ActorID, ShopID: s.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contacts.ErrContactAccessDenied
	}
	return err
}
