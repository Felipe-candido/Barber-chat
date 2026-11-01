package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/jackc/pgx/v5"
)

type ConnectionOwnerGuard struct{ queries *db.Queries }

func NewConnectionOwnerGuard(q *db.Queries) *ConnectionOwnerGuard { return &ConnectionOwnerGuard{q} }
func (g *ConnectionOwnerGuard) RequireConnectionOwner(ctx context.Context, s notifications.ConnectionScope) error {
	_, err := g.queries.LockWhatsAppConnectionOwner(ctx, db.LockWhatsAppConnectionOwnerParams{UserID: s.ActorID, ShopID: s.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return notifications.ErrConnectionAccessDenied
	}
	return err
}
