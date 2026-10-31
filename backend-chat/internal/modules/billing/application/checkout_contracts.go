package application

import (
	"errors"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidCheckout      = errors.New("invalid checkout input")
	ErrBillingAccessDenied  = errors.New("billing owner access required")
	ErrBillingNotConfigured = errors.New("billing is not configured")
	ErrBillingNotRequired   = errors.New("manual shop billing rollout is not enabled")
	ErrSubscriptionExists   = errors.New("a current subscription already exists")
	ErrCheckoutUnavailable  = errors.New("checkout could not be confirmed")
	ErrCheckoutReview       = errors.New("checkout requires reconciliation")
	ErrCheckoutPending      = errors.New("checkout awaits payment confirmation")
	ErrCheckoutExpired      = errors.New("checkout expiration confirmed by provider")
	ErrCheckoutOfferChanged = errors.New("checkout offer changed; review the current conditions")
)

type BillingScope struct{ ActorID, ShopID uuid.UUID }

// ProviderFailure carries only sanitized diagnostics for server logs, never the
// provider's raw message, body, resource identities or request credentials.
type ProviderFailure struct {
	HTTPStatus                 int
	Code, Parameter, RequestID string
}

func (e *ProviderFailure) Error() string { return ErrCheckoutUnavailable.Error() }
func (e *ProviderFailure) Unwrap() error { return ErrCheckoutUnavailable }

type StartCheckoutInput struct {
	Scope   BillingScope
	Intent  domain.CheckoutIntent
	OfferID string
}

type CheckoutPrice struct {
	ID          string
	AmountCents int64
}

// OfferID is a version checksum, NOT authorization or a browser-selected price.
// HTTP exposes only the safe fields below, never the provider price identity.
type CheckoutOffer struct {
	ShopID                    uuid.UUID
	PlanKey, PriceID, OfferID string
	AmountCents               int64
	Recovering                bool
}

type CheckoutOfferState struct {
	HasSubscription bool
	Account         *CheckoutAccount
	Attempt         *CheckoutAttempt
}

// Only server configuration selects the price and return origin. The price is
// checked against Stripe before creating a Session: monthly BRL, no trial.
type CheckoutPlan struct{ Key, PriceID, FrontendOrigin string }

type CheckoutShop struct {
	ShopID      uuid.UUID
	Slug        string
	SelfService bool
}

type CheckoutAccount struct {
	ShopID     uuid.UUID
	CustomerID string
	Mode       string
	CreatedAt  time.Time
}

// Frozen provider parameters survive configuration changes and process crashes.
// Protocol 0 is a legacy record and must not be guessed/backfilled for retries.
type CheckoutAttempt struct {
	ID, ShopID       uuid.UUID
	Intent           domain.CheckoutIntent
	PlanKey, PriceID string
	ReturnURL        string
	State, SessionID string
	CreatedAt        time.Time
	Protocol         int16
}

// A provider-neutral projection; transport-specific JSON stays in the adapter.
type CheckoutSession struct {
	ID, CustomerID, URL, Status string
	SubscriptionID              string
	AttemptID, ShopID           uuid.UUID
	ExpiresAt                   time.Time
	Currency                    string
	AmountSubtotal, AmountTotal int64
}

type CheckoutOutput struct {
	AttemptID uuid.UUID
	Intent    domain.CheckoutIntent
	URL       string
	ExpiresAt time.Time
}
