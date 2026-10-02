package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ListMyShops lists eligible shops for an already authenticated user.
// It neither chooses a shop nor authorizes subsequent administrative actions.
type ListMyShops struct {
	repository IdentityRepository
	shops      AccessibleShopReader
}

func NewListMyShops(
	repository IdentityRepository,
	shops AccessibleShopReader,
) *ListMyShops {
	return &ListMyShops{
		repository: repository,
		shops:      shops,
	}
}

func (uc *ListMyShops) Execute(
	ctx context.Context,
	input ListMyShopsInput,
) ([]AccessibleShopOutput, error) {
	if input.UserID == uuid.Nil {
		return nil, ErrInvalidStaffIdentity
	}

	// Recheck local state even after successful authentication.
	user, found, err := uc.repository.FindUserByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("find local user: %w", err)
	}
	if !found {
		return nil, ErrUserNotProvisioned
	}
	if user.ID != input.UserID {
		return nil, fmt.Errorf("find local user: %w", ErrIdentityMismatch)
	}
	if !user.Active {
		return nil, ErrUserInactive
	}

	// The reader filters by this user and the current user/membership/shop states.
	shops, err := uc.shops.ListActiveShopsByUser(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list accessible shops: %w", err)
	}

	for _, shop := range shops {
		if shop.ShopID == uuid.Nil {
			return nil, fmt.Errorf("list accessible shops: %w", ErrIdentityMismatch)
		}
	}

	// A valid user without eligible memberships is not an authentication failure.
	if len(shops) == 0 {
		return []AccessibleShopOutput{}, nil
	}

	return shops, nil
}
