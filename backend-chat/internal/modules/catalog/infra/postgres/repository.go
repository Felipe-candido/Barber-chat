package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository struct{ queries *db.Queries }

func NewRepository(queries *db.Queries) *Repository { return &Repository{queries: queries} }

func toDomain(row db.Service) domain.Service {
	return domain.Service{ID: row.ID, ShopID: row.ShopID, Name: row.Name, Description: row.Description,
		DurationMinutes: int(row.DurationMinutes), PriceCents: row.PriceCents, Currency: row.Currency, Active: row.Active}
}

func (r *Repository) Create(ctx context.Context, service domain.Service) (domain.Service, error) {
	row, err := r.queries.CreateService(ctx, db.CreateServiceParams{
		ID:              service.ID,
		ShopID:          service.ShopID,
		Name:            service.Name,
		Description:     service.Description,
		DurationMinutes: int32(service.DurationMinutes),
		PriceCents:      service.PriceCents,
		Currency:        service.Currency,
		Active:          service.Active,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Service{}, application.ErrShopNotFound
	}
	if err != nil {
		return domain.Service{}, err
	}
	return toDomain(row), nil
}
func (r *Repository) ListActiveByShop(ctx context.Context, shopID uuid.UUID) ([]domain.Service, error) {
	rows, err := r.queries.ListActiveServicesByShopID(ctx, shopID)
	if err != nil {
		return nil, err
	}
	services := make([]domain.Service, 0, len(rows))
	for _, row := range rows {
		services = append(services, toDomain(row))
	}
	return services, nil
}
