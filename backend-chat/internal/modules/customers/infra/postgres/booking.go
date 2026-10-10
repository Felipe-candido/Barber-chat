package postgres

import (
	"context"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type BookingContacts struct{ queries *db.Queries }

func NewBookingContacts(queries *db.Queries) *BookingContacts { return &BookingContacts{queries} }

func (r *BookingContacts) ResolveForBooking(ctx context.Context, input booking.CustomerResolutionInput) (booking.CustomerResolution, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return booking.CustomerResolution{}, err
	}
	_, err = r.queries.InsertBookingCustomer(ctx, db.InsertBookingCustomerParams{ID: id, ShopID: input.ShopID, FirstName: input.Customer.FirstName,
		LastName: input.Customer.LastName, PhoneE164: input.Customer.Phone, CreatedAt: pgtype.Timestamptz{Time: input.RecordedAt, Valid: true}})
	if err != nil {
		return booking.CustomerResolution{}, err
	}
	// A separate statement sees a competing committed insert under READ COMMITTED.
	// Do not overwrite a previously registered contact from unverified input.
	id, err = r.queries.FindBookingCustomer(ctx, db.FindBookingCustomerParams{ShopID: input.ShopID, PhoneE164: input.Customer.Phone})
	if err != nil {
		return booking.CustomerResolution{}, err
	}
	return booking.CustomerResolution{CustomerID: id}, nil
}
