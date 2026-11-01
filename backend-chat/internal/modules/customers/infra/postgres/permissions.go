package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type BookingPermissions struct{ queries *db.Queries }

func NewBookingPermissions(q *db.Queries) *BookingPermissions { return &BookingPermissions{q} }

func (s *BookingPermissions) RecordBookingPermission(ctx context.Context, p notifications.BookingPermission) error {
	source := "public_booking"
	if p.ActorID != uuid.Nil {
		source = "staff_booking"
	}
	return s.queries.InsertBookingMessagePermission(ctx, db.InsertBookingMessagePermissionParams{
		ShopID: p.ShopID, AppointmentID: p.AppointmentID, CustomerID: p.CustomerID, Allowed: p.Allowed,
		Source: source, ActorID: pgtype.UUID{Bytes: p.ActorID, Valid: p.ActorID != uuid.Nil},
		RecordedAt: pgtype.Timestamptz{Time: p.RecordedAt.UTC(), Valid: true},
	})
}

func (s *BookingPermissions) LockDeliveryPermission(ctx context.Context, shop, appointment uuid.UUID) (notifications.DeliveryPermission, error) {
	r, err := s.queries.LockNotificationDeliveryPermission(ctx, db.LockNotificationDeliveryPermissionParams{ShopID: shop, AppointmentID: appointment})
	if errors.Is(err, pgx.ErrNoRows) {
		return notifications.DeliveryPermission{}, nil
	}
	return notifications.DeliveryPermission{CustomerID: r.CustomerID, Allowed: r.Allowed && !r.RevokedAt.Valid, Phone: r.PhoneE164}, err
}
