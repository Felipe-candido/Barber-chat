package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PublicationStore struct{ queries *db.Queries }

func NewPublicationStore(q *db.Queries) *PublicationStore { return &PublicationStore{queries: q} }

func (s *PublicationStore) LockPublicationShop(ctx context.Context, id uuid.UUID) error {
	_, err := s.queries.LockPublicationShop(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrSettingsNotFound
	}
	return err
}

func (s *PublicationStore) FindPublicationShop(ctx context.Context, id uuid.UUID) (application.PublicationShop, bool, error) {
	row, err := s.queries.GetPublicationShop(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.PublicationShop{}, false, nil
	}
	if err != nil {
		return application.PublicationShop{}, false, err
	}
	settings, found, err := NewSettingsStore(s.queries).FindSettings(ctx, id)
	if err != nil || !found {
		return application.PublicationShop{}, false, err
	}
	return application.PublicationShop{ShopID: row.ID, Slug: row.Slug, SelfService: row.ProvisioningSource == "self_service",
		Requested: row.PublicationRequested, Address: settings.Address}, true, nil
}

func (s *PublicationStore) SetPublicationRequested(ctx context.Context, id uuid.UUID, requested bool) error {
	n, err := s.queries.SetPublicationRequested(ctx, db.SetPublicationRequestedParams{ID: id, PublicationRequested: requested})
	if err != nil {
		return err
	}
	if n != 1 {
		return application.ErrSettingsNotFound
	}
	return nil
}

type PublicationTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.PublicationTx
}

func NewPublicationTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.PublicationTx) *PublicationTransactor {
	return &PublicationTransactor{pool: pool, bind: bind}
}

func (t *PublicationTransactor) WithinPublicationTx(ctx context.Context, fn func(context.Context, application.PublicationTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
