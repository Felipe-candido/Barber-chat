package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (r *Repository) CreateProfessional(ctx context.Context, p domain.Professional, services []uuid.UUID) error {
	if _, err := r.queries.LockActiveProfessionalShop(ctx, p.ShopID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrShopNotFound
		}
		return err
	}
	locked, err := r.queries.LockProfessionalServices(ctx, db.LockProfessionalServicesParams{ShopID: p.ShopID, ServiceIds: services})
	if err != nil {
		return err
	}
	// The use case rejects duplicate IDs. A missing row means at least one
	// service is inactive, invalid or belongs to another shop; never insert a subset.
	if len(locked) != len(services) {
		return application.ErrInvalidProfessionalServices
	}
	if err := r.queries.InsertProfessional(ctx, db.InsertProfessionalParams{ID: p.ID, ShopID: p.ShopID, Name: p.Name, Active: p.Active}); err != nil {
		return err
	}
	return r.queries.AssignProfessionalServices(ctx, db.AssignProfessionalServicesParams{ShopID: p.ShopID, ProfessionalID: p.ID, ServiceIds: services})
}

func (r *Repository) ListProfessionals(ctx context.Context, shopID uuid.UUID) ([]application.ProfessionalOutput, error) {
	rows, err := r.queries.ListShopProfessionals(ctx, shopID)
	if err != nil {
		return nil, err
	}
	items := make([]application.ProfessionalOutput, 0, len(rows))
	for _, row := range rows {
		services := row.ServiceIds
		if services == nil {
			services = make([]uuid.UUID, 0)
		}
		items = append(items, application.ProfessionalOutput{ID: row.ID, Name: row.Name, Active: row.Active, ServiceIDs: services})
	}
	return items, nil
}

type ProfessionalTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.ProfessionalTx
}

func NewProfessionalTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.ProfessionalTx) *ProfessionalTransactor {
	return &ProfessionalTransactor{pool, bind}
}
func (t *ProfessionalTransactor) WithinProfessionalTx(ctx context.Context, fn func(context.Context, application.ProfessionalTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
