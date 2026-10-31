package postgres

import (
	"context"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/google/uuid"
)

type InitialOwnerStore struct{ queries *db.Queries }

var _ shops.InitialOwnerStore = (*InitialOwnerStore)(nil)

func NewInitialOwnerStore(queries *db.Queries) *InitialOwnerStore {
	return &InitialOwnerStore{queries: queries}
}

func (s *InitialOwnerStore) EnsureActiveUser(ctx context.Context, id uuid.UUID, name string) error {
	user, err := domain.NewUser(id, name)
	if err != nil {
		return err
	}
	if err := s.queries.InsertRegistrationUser(ctx, db.InsertRegistrationUserParams{ID: user.ID, DisplayName: user.DisplayName}); err != nil {
		return err
	}
	row, err := s.queries.LockRegistrationUser(ctx, id)
	if err != nil {
		return err
	}
	if row.ID != id {
		return shops.ErrCreationMismatch
	}
	if !row.Active {
		return shops.ErrOwnerInactive
	}
	return nil
}

func (s *InitialOwnerStore) CreateOwnerMembership(ctx context.Context, userID, shopID uuid.UUID) error {
	return s.queries.InsertInitialShopOwner(ctx, db.InsertInitialShopOwnerParams{UserID: userID, ShopID: shopID})
}

func (s *InitialOwnerStore) RequireActiveOwner(ctx context.Context, userID, shopID uuid.UUID) error {
	allowed, err := s.queries.HasActiveRegistrationOwner(ctx, db.HasActiveRegistrationOwnerParams{UserID: userID, ShopID: shopID})
	if err != nil {
		return err
	}
	if !allowed {
		return shops.ErrOwnershipDenied
	}
	return nil
}
