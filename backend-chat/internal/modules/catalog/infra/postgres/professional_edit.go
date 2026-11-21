package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) LockProfessional(ctx context.Context, shop, id uuid.UUID) error {
	_, err := r.queries.LockEditableProfessional(ctx, db.LockEditableProfessionalParams{ShopID: shop, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrProfessionalNotFound
	}
	return err
}
func (r *Repository) ReadProfessional(ctx context.Context, shop, id uuid.UUID) (app.ProfessionalOutput, error) {
	p, err := r.queries.ReadEditableProfessional(ctx, db.ReadEditableProfessionalParams{ShopID: shop, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ProfessionalOutput{}, app.ErrProfessionalNotFound
	}
	if err != nil {
		return app.ProfessionalOutput{}, err
	}
	ids := p.ServiceIds
	if ids == nil {
		ids = make([]uuid.UUID, 0)
	}
	return app.ProfessionalOutput{ID: p.ID, Name: p.Name, Active: p.Active, ServiceIDs: ids}, nil
}
func (r *Repository) ReplaceProfessional(ctx context.Context, shop uuid.UUID, p app.ProfessionalOutput) error {
	rows, err := r.queries.LockEditableProfessionalServices(ctx, db.LockEditableProfessionalServicesParams{ShopID: shop, ProfessionalID: p.ID, ServiceIds: p.ServiceIDs})
	if err != nil {
		return err
	}
	if len(rows) != len(p.ServiceIDs) {
		return app.ErrInvalidProfessionalServices
	}
	if err := r.queries.UpdateProfessionalProfile(ctx, db.UpdateProfessionalProfileParams{ShopID: shop, ID: p.ID, Name: p.Name, Active: p.Active}); err != nil {
		return err
	}
	if err := r.queries.RemoveProfessionalAssignments(ctx, db.RemoveProfessionalAssignmentsParams{ShopID: shop, ProfessionalID: p.ID}); err != nil {
		return err
	}
	return r.queries.AssignProfessionalServices(ctx, db.AssignProfessionalServicesParams{ShopID: shop, ProfessionalID: p.ID, ServiceIds: p.ServiceIDs})
}
