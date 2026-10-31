package postgres

import (
	"context"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries, time.Time) application.ActivityReadTx
}

func NewTransactor(pool *pgxpool.Pool, bind func(*db.Queries, time.Time) application.ActivityReadTx) *Transactor {
	return &Transactor{pool: pool, bind: bind}
}

var _ application.ActivityReadTransactor = (*Transactor)(nil)

func (t *Transactor) WithinActivityRead(ctx context.Context, fn func(context.Context, application.ActivityReadTx) error) error {
	if t.pool == nil || t.bind == nil || fn == nil {
		return application.ErrInconsistentData
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	at, err := q.ReadReportSnapshotTime(ctx)
	if err != nil {
		return err
	}
	if !at.Valid || at.InfinityModifier != pgtype.Finite || at.Time.IsZero() {
		return application.ErrInconsistentData
	}
	bound := t.bind(q, at.Time.UTC())
	bound.ObservedAt = at.Time.UTC()
	if err := fn(ctx, bound); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
