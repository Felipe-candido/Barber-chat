package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidShopID        = errors.New("invalid shop identity")
	ErrShopUnavailable      = errors.New("shop unavailable")
	ErrBillingStateMismatch = errors.New("billing state identity mismatch")
)

type GetShopEntitlements struct {
	reader BillingStateReader
	clock  Clock
}

func NewGetShopEntitlements(reader BillingStateReader, clock Clock) *GetShopEntitlements {
	return &GetShopEntitlements{reader: reader, clock: clock}
}

// CanOperateShop adapts the existing query to a narrow eligibility boundary.
// It grants neither a staff role nor publication intent.
func (uc *GetShopEntitlements) CanOperateShop(ctx context.Context, shopID uuid.UUID) (bool, error) {
	result, err := uc.Execute(ctx, GetShopEntitlementsInput{ShopID: shopID})
	if err != nil {
		return false, err
	}
	return result.Entitlement.Allowed, nil
}

func (uc *GetShopEntitlements) Execute(ctx context.Context, input GetShopEntitlementsInput) (ShopEntitlementsOutput, error) {
	if input.ShopID == uuid.Nil {
		return ShopEntitlementsOutput{}, ErrInvalidShopID
	}
	if err := ctx.Err(); err != nil {
		return ShopEntitlementsOutput{}, err
	}
	state, found, err := uc.reader.FindShopBillingState(ctx, input.ShopID)
	if err != nil {
		return ShopEntitlementsOutput{}, fmt.Errorf("read shop billing state: %w", err)
	}
	if !found {
		return ShopEntitlementsOutput{}, ErrShopUnavailable
	}
	if state.ShopID != input.ShopID || (state.Subscription != nil && state.Subscription.ShopID != input.ShopID) {
		return ShopEntitlementsOutput{}, ErrBillingStateMismatch
	}
	if !state.Active {
		return ShopEntitlementsOutput{}, ErrShopUnavailable
	}
	entitlement, err := domain.EvaluateEntitlement(state.Subscription, uc.clock.Now())
	if err != nil {
		return ShopEntitlementsOutput{}, fmt.Errorf("evaluate shop entitlement: %w", err)
	}
	return ShopEntitlementsOutput{ShopID: input.ShopID, Entitlement: entitlement}, nil
}
