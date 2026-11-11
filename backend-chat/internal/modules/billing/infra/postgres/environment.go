package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Bind before serving traffic, including reads of existing commercial coverage.
// Refusing a mode switch prevents sandbox invoices from unlocking a live shop.
func EnsureBillingEnvironment(ctx context.Context, pool *pgxpool.Pool, mode domain.BillingMode) error {
	if !mode.Valid() {
		return errors.New("invalid Stripe environment")
	}
	q := db.New(pool)
	if err := q.BindBillingEnvironment(ctx, string(mode)); err != nil {
		return errors.New("billing environment unavailable; apply migrations before starting the API")
	}
	actual, err := q.GetBillingEnvironment(ctx)
	if err != nil || actual != string(mode) {
		return errors.New("Stripe mode differs from the database environment; use an isolated database, never convert test coverage into live billing")
	}
	return nil
}
