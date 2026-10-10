package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Called inside the booking transaction AFTER the professional calendar lock.
// This order matches account deactivation, avoiding membership/calendar inversion.
func (r *Repository) CanCreateBooking(ctx context.Context, userID, shopID, professionalID uuid.UUID) (bool, error) {
	_, err := r.queries.LockStaffBookingAccess(ctx, db.LockStaffBookingAccessParams{UserID: userID, ShopID: shopID, ProfessionalID: pgtype.UUID{Bytes: professionalID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
