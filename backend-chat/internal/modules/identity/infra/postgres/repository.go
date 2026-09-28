package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository struct{ queries *db.Queries }

var _ application.IdentityRepository = (*Repository)(nil)

func NewRepository(queries *db.Queries) *Repository {
	return &Repository{queries: queries}
}

func toDomainUser(row db.GetUserByIDRow) domain.User {
	return domain.User{
		ID:          row.ID,
		DisplayName: row.DisplayName,
		Active:      row.Active,
	}
}

func toDomainMembership(row db.GetMembershipRow) domain.Membership {
	return domain.Membership{
		UserID: row.UserID,
		ShopID: row.ShopID,
		Active: row.Active,
	}
}

func (r *Repository) FindUserByID(
	ctx context.Context,
	id uuid.UUID,
) (domain.User, bool, error) {
	row, err := r.queries.GetUserByID(ctx, id)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return toDomainUser(row), true, nil
}

func (r *Repository) FindMembership(
	ctx context.Context,
	userID, shopID uuid.UUID,
) (domain.Membership, bool, error) {
	row, err := r.queries.GetMembership(ctx, db.GetMembershipParams{
		UserID: userID,
		ShopID: shopID,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Membership{}, false, nil
	}
	if err != nil {
		return domain.Membership{}, false, err
	}
	return toDomainMembership(row), true, nil
}

func (r *Repository) ListMembershipsByUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]domain.Membership, error) {
	rows, err := r.queries.ListMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	memberships := make([]domain.Membership, 0, len(rows))

	for _, row := range rows {
		memberships = append(memberships, domain.Membership{
			UserID: row.UserID,
			ShopID: row.ShopID,
			Active: row.Active,
		})
	}

	return memberships, nil
}
