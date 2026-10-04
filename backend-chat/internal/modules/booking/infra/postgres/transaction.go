package postgres

import (
	"context"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Composition supplies module-specific adapters bound to this transaction's
// Queries. The booking package does not construct other modules' repositories.
type Transactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.BookingTx
}

func NewTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.BookingTx) *Transactor {
	return &Transactor{pool, bind}
}

func (t *Transactor) WithinBookingTx(ctx context.Context, fn func(context.Context, application.BookingTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		// The request may already be cancelled; rollback still needs a bounded context.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
