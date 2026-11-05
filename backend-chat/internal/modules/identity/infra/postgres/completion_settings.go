package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/jackc/pgx/v5"
)

// Membership belongs to identity; booking consumes only this access port.
type CompletionSettingsAccess struct{ q *db.Queries }

func NewCompletionSettingsAccess(q *db.Queries) *CompletionSettingsAccess {
	return &CompletionSettingsAccess{q}
}
func (g *CompletionSettingsAccess) RequireCompletionSettingsAccess(ctx context.Context, s booking.CompletionSettingsScope) error {
	_, err := g.q.LockAppointmentCompletionAccess(ctx, db.LockAppointmentCompletionAccessParams{UserID: s.ActorID, ShopID: s.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return booking.ErrCompletionSettingsAccessDenied
	}
	return err
}
