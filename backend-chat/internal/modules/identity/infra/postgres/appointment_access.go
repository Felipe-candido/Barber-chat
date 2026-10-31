package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) RequireAppointmentAccess(ctx context.Context, scope application.LifecycleScope, professional uuid.UUID) error {
	_, err := r.queries.LockStaffBookingAccess(ctx, db.LockStaffBookingAccessParams{UserID: scope.ActorID, ShopID: scope.ShopID, ProfessionalID: pgtype.UUID{Bytes: professional, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrAppointmentAccessDenied
	}
	if err != nil {
		return err
	}
	_, err = r.queries.LockActiveProductShop(ctx, scope.ShopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrAppointmentAccessDenied
	}
	return err
}
