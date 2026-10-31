package application

import (
	"errors"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidWebhook            = errors.New("invalid Stripe webhook")
	ErrUnsupportedWebhookVersion = errors.New("unsupported Stripe webhook version")
	ErrWebhookNotConfigured      = errors.New("Stripe webhook is not configured")
	ErrWebhookRetry              = errors.New("Stripe webhook needs redelivery or reconciliation")
	ErrReconciliationConflict    = errors.New("billing reconciliation superseded or mismatched")
)

// VerifiedWebhook contains only a signed, validated envelope and resource ID.
// Raw bodies/customer contact/payment details are not persisted by this flow.
type VerifiedWebhook struct {
	ID, Type, APIVersion, Kind, ResourceID string
	CreatedAt                              time.Time
}

// References locate local associations; metadata is a hint, never authority.
type BillingReference struct {
	CustomerID, SubscriptionID string
	AttemptID, ShopID          uuid.UUID
}

type ReconciliationTarget struct {
	Event          VerifiedWebhook
	Account        CheckoutAccount
	Attempt        CheckoutAttempt
	SubscriptionID string
	Revision       int64
}

type ConfirmedInvoice struct {
	ID, PriceID string
	Eligible    bool
	Period      domain.Coverage
	AmountPaid  int64
}

type SubscriptionSnapshot struct {
	ID, CustomerID, PriceID string
	Status                  domain.SubscriptionStatus
	CancelAtPeriodEnd       bool
	Session                 CheckoutSession
	Invoices                []ConfirmedInvoice
}
