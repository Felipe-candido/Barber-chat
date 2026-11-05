package postgres

import (
	"context"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CompletionSettingsTransactor struct {
	pool   *pgxpool.Pool
	access func(*db.Queries) app.CompletionSettingsAccess
}

func NewCompletionSettingsTransactor(pool *pgxpool.Pool, access func(*db.Queries) app.CompletionSettingsAccess) *CompletionSettingsTransactor {
	return &CompletionSettingsTransactor{pool, access}
}
func (t *CompletionSettingsTransactor) WithinCompletionSettingsTx(ctx context.Context, fn func(context.Context, app.CompletionSettingsTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err = fn(ctx, app.CompletionSettingsTx{Access: t.access(q), Store: &CompletionSettingsStore{q}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type CompletionSettingsStore struct{ q *db.Queries }

func (s *CompletionSettingsStore) Find(ctx context.Context, id uuid.UUID) (app.CompletionSettings, error) {
	r, err := s.q.GetShopAppointmentCompletion(ctx, id)
	if err != nil {
		return app.CompletionSettings{}, err
	}
	return app.CompletionSettings{ShopID: r.ShopID, Mode: domain.CompletionMode(r.Mode), Version: r.Version, AutomaticSince: r.AutomaticSince.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC()}, nil
}
func (s *CompletionSettingsStore) Replace(ctx context.Context, id uuid.UUID, version int64, mode domain.CompletionMode) (bool, error) {
	n, err := s.q.ReplaceShopAppointmentCompletion(ctx, db.ReplaceShopAppointmentCompletionParams{ShopID: id, ExpectedVersion: version, Mode: string(mode)})
	return n == 1, err
}
