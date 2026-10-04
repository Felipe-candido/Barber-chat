package postgres

import (
	"context"
	"errors"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Resolver) FindAgendaShop(ctx context.Context, shopID uuid.UUID) (booking.ShopContext, bool, error) {
	row, err := r.queries.GetActiveAgendaShop(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return booking.ShopContext{}, false, nil
	}
	if err != nil {
		return booking.ShopContext{}, false, err
	}
	return booking.ShopContext{ShopID: row.ID, Slug: row.Slug, Name: row.Name, Timezone: row.Timezone}, true, nil
}

// ResolveActiveShop implements booking's projection without exporting a shops
// domain entity or coupling either application's imports to the other module.
func (r *Resolver) ResolveActiveShop(ctx context.Context, slug string) (booking.ShopContext, bool, error) {
	row, err := r.queries.ResolveBookingShop(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return booking.ShopContext{}, false, nil
	}
	if err != nil {
		return booking.ShopContext{}, false, err
	}
	return booking.ShopContext{ShopID: row.ID, Slug: row.Slug, Name: row.Name, Timezone: row.Timezone}, true, nil
}
