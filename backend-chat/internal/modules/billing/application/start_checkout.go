package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

type StartCheckout struct {
	transactions CheckoutTransactor
	provider     CheckoutProvider
	clock        Clock
	plan         CheckoutPlan
}

func NewStartCheckout(transactions CheckoutTransactor, provider CheckoutProvider, clock Clock, plan CheckoutPlan) *StartCheckout {
	return &StartCheckout{transactions: transactions, provider: provider, clock: clock, plan: plan}
}

func (uc *StartCheckout) Execute(ctx context.Context, input StartCheckoutInput) (CheckoutOutput, error) {
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil || !input.Intent.Valid() {
		return CheckoutOutput{}, ErrInvalidCheckout
	}
	if uc.provider == nil || uc.plan.Key == "" || uc.plan.PriceID == "" || uc.plan.FrontendOrigin == "" {
		return CheckoutOutput{}, ErrBillingNotConfigured
	}
	if len(input.OfferID) != 64 || strings.Trim(input.OfferID, "0123456789abcdef") != "" {
		return CheckoutOutput{}, ErrInvalidCheckout
	}
	offer, err := uc.Offer(ctx, input.Scope)
	if err != nil {
		return CheckoutOutput{}, err
	}
	if input.OfferID != offer.OfferID {
		return CheckoutOutput{}, ErrCheckoutOfferChanged
	}
	var account CheckoutAccount
	var attempt CheckoutAttempt
	err = uc.transactions.WithinCheckoutTx(ctx, func(ctx context.Context, tx CheckoutTx) error {
		shop, err := checkoutOwner(ctx, tx, input.Scope)
		if err != nil {
			return err
		}
		account, err = tx.Store.ReserveAccount(ctx, shop.ShopID)
		if err != nil {
			return err
		}
		current, err := tx.Store.HasCurrentSubscription(ctx, shop.ShopID)
		if err != nil {
			return err
		}
		if current {
			return ErrSubscriptionExists
		}
		found := false
		attempt, found, err = tx.Store.FindPendingCheckout(ctx, shop.ShopID)
		if err != nil {
			return err
		}
		if found {
			if attempt.PriceID != offer.PriceID || attempt.PlanKey != offer.PlanKey {
				return ErrCheckoutOfferChanged
			}
			return nil
		}
		if uc.plan.PriceID != offer.PriceID || uc.plan.Key != offer.PlanKey {
			return ErrCheckoutOfferChanged
		}
		attempt = CheckoutAttempt{ID: uuid.New(), ShopID: shop.ShopID, Intent: input.Intent,
			PlanKey: uc.plan.Key, PriceID: uc.plan.PriceID,
			ReturnURL: uc.plan.FrontendOrigin + "/admin/" + url.PathEscape(shop.Slug) + "/configuracoes",
			State:     "preparing", CreatedAt: uc.clock.Now().UTC(), Protocol: 1}
		return tx.Store.InsertCheckout(ctx, attempt)
	})
	if err != nil {
		return CheckoutOutput{}, err
	}
	if attempt.Protocol != 1 || account.Mode != "test" {
		return CheckoutOutput{}, ErrCheckoutReview
	}
	if attempt.State == "review" {
		if attempt.SessionID != "" {
			return CheckoutOutput{}, ErrCheckoutPending
		}
		return CheckoutOutput{}, ErrCheckoutReview
	}
	var session CheckoutSession
	if attempt.SessionID != "" {
		// Never create a second Session because a known one is slow or expired
		// according to the local clock. Only the provider confirms its state.
		session, err = uc.provider.FindCheckoutSession(ctx, attempt.SessionID)
	} else {
		now := uc.clock.Now()
		if !domain.CanRecoverCheckout(attempt.CreatedAt, now) ||
			(account.CustomerID == "" && !domain.CanRecoverCheckout(account.CreatedAt, now)) {
			return CheckoutOutput{}, ErrCheckoutReview
		}
		if account.CustomerID == "" {
			account.CustomerID, err = uc.provider.CreateCheckoutCustomer(ctx, account.ShopID)
			if err != nil {
				return CheckoutOutput{}, fmt.Errorf("%w: %w", ErrCheckoutUnavailable, err)
			}
			err = uc.transactions.WithinCheckoutTx(ctx, func(ctx context.Context, tx CheckoutTx) error {
				if _, err := checkoutOwner(ctx, tx, input.Scope); err != nil {
					return err
				}
				linked, err := tx.Store.LinkCheckoutCustomer(ctx, account.ShopID, account.CustomerID)
				if err != nil {
					return err
				}
				if !linked {
					return ErrCheckoutReview
				}
				return nil
			})
			if err != nil {
				return CheckoutOutput{}, err
			}
		}
		session, err = uc.provider.CreateCheckoutSession(ctx, attempt, account.CustomerID)
	}
	if err != nil {
		return CheckoutOutput{}, fmt.Errorf("%w: %w", ErrCheckoutUnavailable, err)
	}
	if session.AttemptID != attempt.ID || session.ShopID != attempt.ShopID || session.CustomerID != account.CustomerID ||
		session.ID == "" || (attempt.SessionID != "" && session.ID != attempt.SessionID) || session.ExpiresAt.IsZero() {
		return CheckoutOutput{}, ErrCheckoutReview
	}
	// Dashboard-level currency localization/tax must not silently change the
	// reviewed fixed BRL amount. Do not change frozen Session creation parameters
	// for protocol-1 retries; refuse a mismatched open Session instead.
	if session.Status == "open" && (session.Currency != "brl" || session.AmountSubtotal != offer.AmountCents || session.AmountTotal != offer.AmountCents) {
		return CheckoutOutput{}, ErrCheckoutReview
	}
	state := ""
	switch session.Status {
	case "open":
		state = "open"
	case "complete":
		state = "review" // A completed Checkout is not confirmed paid coverage.
	case "expired":
		state = "expired"
	default:
		return CheckoutOutput{}, ErrCheckoutReview
	}
	err = uc.transactions.WithinCheckoutTx(ctx, func(ctx context.Context, tx CheckoutTx) error {
		if _, err := checkoutOwner(ctx, tx, input.Scope); err != nil {
			return err
		}
		linked, err := tx.Store.RecordCheckoutSession(ctx, attempt, session, state)
		if err != nil {
			return err
		}
		if !linked {
			return ErrCheckoutReview
		}
		return nil
	})
	if err != nil {
		return CheckoutOutput{}, err
	}
	if state == "review" {
		return CheckoutOutput{}, ErrCheckoutPending
	}
	if state == "expired" {
		return CheckoutOutput{}, ErrCheckoutExpired
	}
	if session.URL == "" || !session.ExpiresAt.After(uc.clock.Now().Add(30*time.Second)) {
		return CheckoutOutput{}, ErrCheckoutReview
	}
	return CheckoutOutput{AttemptID: attempt.ID, Intent: attempt.Intent, URL: session.URL, ExpiresAt: session.ExpiresAt.UTC()}, nil
}

func checkoutOwner(ctx context.Context, tx CheckoutTx, scope BillingScope) (CheckoutShop, error) {
	if err := tx.Access.RequireBillingOwner(ctx, scope.ActorID, scope.ShopID); err != nil {
		return CheckoutShop{}, err
	}
	shop, found, err := tx.Shops.FindCheckoutShop(ctx, scope.ShopID)
	if err != nil {
		return CheckoutShop{}, err
	}
	if !found || shop.ShopID != scope.ShopID {
		return CheckoutShop{}, ErrBillingAccessDenied
	}
	if !shop.SelfService {
		return CheckoutShop{}, ErrBillingNotRequired
	}
	return shop, nil
}
