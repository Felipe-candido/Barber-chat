package application

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

var (
	ErrPortalNotConfigured = errors.New("customer portal is not configured")
	ErrPortalNotAvailable  = errors.New("no reconciled subscription is available for management")
	ErrPortalUnavailable   = errors.New("customer portal could not be opened")
)

type PortalRequest struct {
	ShopID                                 uuid.UUID
	CustomerID, ConfigurationID, ReturnURL string
}

type PortalSession struct {
	URL       string
	CreatedAt time.Time
}

type PortalOutput struct {
	LiveMode bool
	ShopID   uuid.UUID
	URL      string
	// An application opening window, not Stripe's session expiry.
	OpenUntil time.Time
}

type OpenPortal struct {
	mode                            domain.BillingMode
	transactions                    CheckoutTransactor
	provider                        PortalProvider
	clock                           Clock
	configurationID, frontendOrigin string
}

func NewOpenPortal(tx CheckoutTransactor, provider PortalProvider, clock Clock, configurationID, frontendOrigin string) *OpenPortal {
	return NewOpenPortalForMode(tx, provider, clock, configurationID, frontendOrigin, domain.BillingTest)
}

func NewOpenPortalForMode(tx CheckoutTransactor, provider PortalProvider, clock Clock, configurationID, frontendOrigin string, mode domain.BillingMode) *OpenPortal {
	return &OpenPortal{mode: mode.Effective(), transactions: tx, provider: provider, clock: clock, configurationID: configurationID, frontendOrigin: frontendOrigin}
}

func (uc *OpenPortal) Execute(ctx context.Context, scope BillingScope) (PortalOutput, error) {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil {
		return PortalOutput{}, ErrBillingAccessDenied
	}
	if uc.provider == nil || !uc.mode.Valid() || uc.configurationID == "" || uc.frontendOrigin == "" {
		return PortalOutput{}, ErrPortalNotConfigured
	}
	var target PortalRequest
	err := uc.transactions.WithinCheckoutTx(ctx, func(ctx context.Context, tx CheckoutTx) error {
		shop, err := checkoutOwner(ctx, tx, scope)
		if err != nil {
			return err
		}
		account, found, err := tx.Store.FindPortalAccount(ctx, shop.ShopID)
		if err != nil {
			return err
		}
		if !found || account.ShopID != shop.ShopID || account.Mode != string(uc.mode) || account.CustomerID == "" {
			return ErrPortalNotAvailable
		}
		target = PortalRequest{ShopID: shop.ShopID, CustomerID: account.CustomerID, ConfigurationID: uc.configurationID,
			ReturnURL: uc.frontendOrigin + "/admin/" + url.PathEscape(shop.Slug) + "/configuracoes?portal=returned#publication"}
		return nil
	})
	if err != nil {
		return PortalOutput{}, err
	}
	// Stripe I/O never holds the authorization/database locks.
	session, err := uc.provider.CreatePortalSession(ctx, target)
	if err != nil {
		return PortalOutput{}, errors.Join(ErrPortalUnavailable, err)
	}
	now := uc.clock.Now()
	openUntil := session.CreatedAt.Add(4 * time.Minute)
	if session.URL == "" || session.CreatedAt.After(now.Add(30*time.Second)) || !openUntil.After(now.Add(30*time.Second)) {
		return PortalOutput{}, ErrPortalUnavailable
	}
	// Do not disclose the bearer URL if access changed while Stripe responded.
	err = uc.transactions.WithinCheckoutTx(ctx, func(ctx context.Context, tx CheckoutTx) error {
		shop, err := checkoutOwner(ctx, tx, scope)
		if err != nil {
			return err
		}
		account, found, err := tx.Store.FindPortalAccount(ctx, shop.ShopID)
		if err != nil {
			return err
		}
		if !found || account.ShopID != target.ShopID || account.Mode != string(uc.mode) || account.CustomerID != target.CustomerID ||
			target.ReturnURL != uc.frontendOrigin+"/admin/"+url.PathEscape(shop.Slug)+"/configuracoes?portal=returned#publication" {
			return ErrPortalNotAvailable
		}
		return nil
	})
	if err != nil {
		return PortalOutput{}, err
	}
	return PortalOutput{ShopID: scope.ShopID, URL: session.URL, OpenUntil: openUntil, LiveMode: uc.mode.Live()}, nil
}
