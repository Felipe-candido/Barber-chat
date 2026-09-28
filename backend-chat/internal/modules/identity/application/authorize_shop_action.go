package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type AuthorizeShopAction struct {
	repository IdentityRepository
	shops      ShopResolver
}

func NewAuthorizeShopAction(repository IdentityRepository, shops ShopResolver) *AuthorizeShopAction {
	return &AuthorizeShopAction{repository: repository, shops: shops}
}

func (uc *AuthorizeShopAction) Execute(ctx context.Context, input AuthorizeShopActionInput) (AuthorizedShopScope, error) {
	if input.UserID == uuid.Nil {
		return AuthorizedShopScope{}, ErrInvalidStaffIdentity
	}

	shopSlug := strings.TrimSpace(input.ShopSlug)
	if shopSlug == "" {
		return AuthorizedShopScope{}, ErrShopAccessDenied
	}

	// Recheck local state on each authorization, even after authentication.
	user, found, err := uc.repository.FindUserByID(ctx, input.UserID)
	if err != nil {
		return AuthorizedShopScope{}, fmt.Errorf("find local user: %w", err)
	}
	if !found {
		return AuthorizedShopScope{}, ErrUserNotProvisioned
	}
	if user.ID != input.UserID {
		return AuthorizedShopScope{}, fmt.Errorf("find local user: %w", ErrIdentityMismatch)
	}
	if !user.Active {
		return AuthorizedShopScope{}, ErrUserInactive
	}

	shopID, found, err := uc.shops.LookupActiveShop(ctx, shopSlug)
	if err != nil {
		return AuthorizedShopScope{}, fmt.Errorf("resolve active shop: %w", err)
	}
	if !found {
		return AuthorizedShopScope{}, ErrShopAccessDenied
	}
	if shopID == uuid.Nil {
		return AuthorizedShopScope{}, fmt.Errorf("resolve active shop: %w", ErrIdentityMismatch)
	}

	membership, found, err := uc.repository.FindMembership(ctx, user.ID, shopID)
	if err != nil {
		return AuthorizedShopScope{}, fmt.Errorf("find membership: %w", err)
	}
	if !found {
		return AuthorizedShopScope{}, ErrShopAccessDenied
	}
	if membership.UserID != user.ID || membership.ShopID != shopID {
		return AuthorizedShopScope{}, fmt.Errorf("find membership: %w", ErrIdentityMismatch)
	}
	if !membership.Active {
		return AuthorizedShopScope{}, ErrShopAccessDenied
	}

	return AuthorizedShopScope{UserID: user.ID, ShopID: shopID}, nil
}
