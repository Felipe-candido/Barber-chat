package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TimeOffTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) app.TimeOffTx
}

func NewTimeOffTransactor(pool *pgxpool.Pool, bind func(*db.Queries) app.TimeOffTx) *TimeOffTransactor {
	return &TimeOffTransactor{pool, bind}
}
func (t *TimeOffTransactor) WithinTimeOffTx(ctx context.Context, fn func(context.Context, app.TimeOffTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListTimeOffProfessionals(ctx context.Context, shop, professional uuid.UUID) ([]app.TimeOffProfessional, error) {
	rows, err := r.queries.ListTimeOffProfessionals(ctx, db.ListTimeOffProfessionalsParams{ShopID: shop, ProfessionalID: professional})
	if err != nil {
		return nil, err
	}
	out := make([]app.TimeOffProfessional, 0, len(rows))
	for _, v := range rows {
		out = append(out, app.TimeOffProfessional{ID: v.ID, Name: v.Name})
	}
	return out, nil
}
func (r *Repository) ListTimeOff(ctx context.Context, shop, professional uuid.UUID, now time.Time) ([]app.TimeOffBlock, error) {
	rows, err := r.queries.ListTimeOff(ctx, db.ListTimeOffParams{ShopID: shop, ProfessionalID: professional, NowAt: timestamp(now)})
	if err != nil {
		return nil, err
	}
	out := make([]app.TimeOffBlock, 0, len(rows))
	for _, v := range rows {
		out = append(out, app.TimeOffBlock{ID: v.ID, ProfessionalID: v.ProfessionalID, ProfessionalName: v.ProfessionalName, StartsAt: v.StartsAt.Time.UTC(), EndsAt: v.EndsAt.Time.UTC()})
	}
	return out, nil
}
func (r *Repository) FindTimeOff(ctx context.Context, shop, professional, id uuid.UUID) (app.TimeOffBlock, bool, error) {
	v, err := r.queries.FindTimeOff(ctx, db.FindTimeOffParams{ShopID: shop, ProfessionalID: professional, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.TimeOffBlock{}, false, nil
	}
	if err != nil {
		return app.TimeOffBlock{}, false, err
	}
	return app.TimeOffBlock{ID: v.ID, ProfessionalID: v.ProfessionalID, ProfessionalName: v.ProfessionalName, StartsAt: v.StartsAt.Time.UTC(), EndsAt: v.EndsAt.Time.UTC()}, true, nil
}

// Caller holds the same professional lock used by booking before every check.
// The database trigger independently checks appointment overlap on INSERT.
func (r *Repository) CreateTimeOff(ctx context.Context, shop uuid.UUID, b app.TimeOffBlock) error {
	occupied, err := r.queries.TimeOffHasAppointments(ctx, db.TimeOffHasAppointmentsParams{ShopID: shop, ProfessionalID: b.ProfessionalID, StartsAt: timestamp(b.StartsAt), EndsAt: timestamp(b.EndsAt)})
	if err != nil {
		return err
	}
	if occupied {
		return app.ErrTimeOffAppointments
	}
	blocked, err := r.queries.TimeOffHasBlocks(ctx, db.TimeOffHasBlocksParams{ShopID: shop, ProfessionalID: b.ProfessionalID, StartsAt: timestamp(b.StartsAt), EndsAt: timestamp(b.EndsAt)})
	if err != nil {
		return err
	}
	if blocked {
		return app.ErrTimeOffOverlap
	}
	n, err := r.queries.InsertTimeOff(ctx, db.InsertTimeOffParams{ID: b.ID, ShopID: shop, ProfessionalID: b.ProfessionalID, StartsAt: timestamp(b.StartsAt), EndsAt: timestamp(b.EndsAt)})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return app.ErrTimeOffAppointments
	}
	if err != nil {
		return err
	}
	if n != 1 {
		return app.ErrTimeOffConflict
	}
	return nil
}
func (r *Repository) DeleteTimeOff(ctx context.Context, shop, professional, id uuid.UUID) error {
	return r.queries.DeleteTimeOff(ctx, db.DeleteTimeOffParams{ShopID: shop, ProfessionalID: professional, ID: id})
}
