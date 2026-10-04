package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookingdomain "github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) FindBookableSelection(ctx context.Context, shopID, serviceID, professionalID uuid.UUID) (booking.BookableSelection, bool, error) {
	row, err := r.queries.FindBookableSelection(ctx, db.FindBookableSelectionParams{ShopID: shopID, ServiceID: serviceID, ProfessionalID: professionalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return booking.BookableSelection{}, false, nil
	}
	if err != nil {
		return booking.BookableSelection{}, false, err
	}
	return booking.BookableSelection{ShopID: row.ShopID, ServiceID: row.ServiceID, ProfessionalID: row.ProfessionalID, ProfessionalName: row.ProfessionalName,
		Service: bookingdomain.ServiceSnapshot{Name: row.ServiceName, DurationMinutes: int(row.DurationMinutes), PriceCents: row.PriceCents, Currency: row.Currency}}, true, nil
}

func (r *Repository) ListBookableProfessionals(ctx context.Context, shopID, serviceID uuid.UUID) ([]booking.Professional, error) {
	rows, err := r.queries.ListBookableProfessionals(ctx, db.ListBookableProfessionalsParams{ShopID: shopID, ServiceID: serviceID})
	if err != nil {
		return nil, err
	}
	items := make([]booking.Professional, 0, len(rows))
	for _, row := range rows {
		items = append(items, booking.Professional{ID: row.ID, Name: row.Name})
	}
	return items, nil
}
