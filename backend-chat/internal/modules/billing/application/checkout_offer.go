package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

// This binds confirmation to server-selected conditions without storing a quote
// or accepting price/amount from the browser. POST always recomputes it.
func (o CheckoutOffer) ConfirmationID() string {
	data, _ := json.Marshal([]any{o.ShopID, o.PlanKey, o.PriceID, o.AmountCents, "BRL", "month", 1, false})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (uc *StartCheckout) Offer(ctx context.Context, scope BillingScope) (CheckoutOffer, error) {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil {
		return CheckoutOffer{}, ErrBillingAccessDenied
	}
	if uc.provider == nil || uc.plan.Key == "" || uc.plan.PriceID == "" || uc.plan.FrontendOrigin == "" {
		return CheckoutOffer{}, ErrBillingNotConfigured
	}
	selected, err := uc.offerSelection(ctx, scope)
	if err != nil {
		return CheckoutOffer{}, err
	}
	price, err := uc.provider.ReadCheckoutPrice(ctx, selected.PriceID)
	if err != nil {
		return CheckoutOffer{}, fmt.Errorf("%w: %w", ErrCheckoutUnavailable, err)
	}
	if price.ID != selected.PriceID || price.AmountCents <= 0 || price.AmountCents > 9007199254740991 {
		return CheckoutOffer{}, ErrCheckoutUnavailable
	}
	// Auth and the selected local attempt can change during provider I/O.
	current, err := uc.offerSelection(ctx, scope)
	if err != nil {
		return CheckoutOffer{}, err
	}
	if current.PriceID != selected.PriceID || current.PlanKey != selected.PlanKey {
		return CheckoutOffer{}, ErrCheckoutOfferChanged
	}
	current.AmountCents = price.AmountCents
	current.OfferID = current.ConfirmationID()
	return current, nil
}

func (uc *StartCheckout) offerSelection(ctx context.Context, scope BillingScope) (CheckoutOffer, error) {
	var result CheckoutOffer
	err := uc.transactions.WithinCheckoutTx(ctx, func(ctx context.Context, tx CheckoutTx) error {
		shop, err := checkoutOwner(ctx, tx, scope)
		if err != nil {
			return err
		}
		state, err := tx.Store.ReadCheckoutOfferState(ctx, shop.ShopID)
		if err != nil {
			return err
		}
		if state.HasSubscription {
			return ErrSubscriptionExists
		}
		if state.Account != nil && (state.Account.ShopID != shop.ShopID || state.Account.Mode != "test") {
			return ErrCheckoutReview
		}
		result = CheckoutOffer{ShopID: shop.ShopID, PlanKey: uc.plan.Key, PriceID: uc.plan.PriceID}
		if a := state.Attempt; a != nil {
			if a.ShopID != shop.ShopID || a.Protocol != 1 || a.PriceID == "" || a.PlanKey == "" {
				return ErrCheckoutReview
			}
			if a.State == "review" {
				if a.SessionID != "" {
					return ErrCheckoutPending
				}
				return ErrCheckoutReview
			}
			if a.SessionID == "" && !domain.CanRecoverCheckout(a.CreatedAt, uc.clock.Now()) {
				return ErrCheckoutReview
			}
			result.PlanKey, result.PriceID, result.Recovering = a.PlanKey, a.PriceID, true
		}
		if state.Account != nil && state.Account.CustomerID == "" && !domain.CanRecoverCheckout(state.Account.CreatedAt, uc.clock.Now()) {
			return ErrCheckoutReview
		}
		return nil
	})
	return result, err
}
