package application

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Identity and shops implement their own projections on the same transaction.
// Locks protect the local authorization/reservation, never a network call.
type BillingOwnerAccess interface {
	RequireBillingOwner(context.Context, uuid.UUID, uuid.UUID) error
}

type CheckoutShopReader interface {
	FindCheckoutShop(context.Context, uuid.UUID) (CheckoutShop, bool, error)
}

type CheckoutStore interface {
	FindPortalAccount(context.Context, uuid.UUID) (CheckoutAccount, bool, error)
	ReadCheckoutOfferState(context.Context, uuid.UUID) (CheckoutOfferState, error)
	ReserveAccount(context.Context, uuid.UUID) (CheckoutAccount, error)
	HasCurrentSubscription(context.Context, uuid.UUID) (bool, error)
	FindPendingCheckout(context.Context, uuid.UUID) (CheckoutAttempt, bool, error)
	InsertCheckout(context.Context, CheckoutAttempt) error
	LinkCheckoutCustomer(context.Context, uuid.UUID, string) (bool, error)
	RecordCheckoutSession(context.Context, CheckoutAttempt, CheckoutSession, string) (bool, error)
}

// A portal session only delegates management of an existing tenant's Customer.
// Creating the session must not create a Customer, subscription or entitlement.
type PortalProvider interface {
	CreatePortalSession(context.Context, PortalRequest) (PortalSession, error)
}

type CheckoutTx struct {
	Access BillingOwnerAccess
	Shops  CheckoutShopReader
	Store  CheckoutStore
}

type CheckoutTransactor interface {
	WithinCheckoutTx(context.Context, func(context.Context, CheckoutTx) error) error
}

// Calls are outside SQL transactions. Creation must reuse the supplied durable
// keys/parameters. No automatic application-level retries or activation here.
type CheckoutProvider interface {
	ReadCheckoutPrice(context.Context, string) (CheckoutPrice, error)
	CreateCheckoutCustomer(context.Context, uuid.UUID) (string, error)
	CreateCheckoutSession(context.Context, CheckoutAttempt, string) (CheckoutSession, error)
	FindCheckoutSession(context.Context, string) (CheckoutSession, error)
}

type WebhookVerifier interface {
	VerifyWebhook([]byte, string) (VerifiedWebhook, error)
}

type WebhookProvider interface {
	ResolveBillingReference(context.Context, VerifiedWebhook) (BillingReference, error)
	ReadSubscriptionSnapshot(context.Context, ReconciliationTarget) (SubscriptionSnapshot, error)
}

type WebhookStore interface {
	// Returns terminal only after commit; collisions cannot overwrite an event.
	AcceptWebhook(context.Context, VerifiedWebhook) (terminal bool, err error)
	// nil target is a durably ignored/previously completed receipt. Claims fence
	// snapshots without holding any SQL transaction during Stripe requests.
	ClaimReconciliation(context.Context, VerifiedWebhook, BillingReference) (*ReconciliationTarget, error)
	ApplyReconciliation(context.Context, ReconciliationTarget, SubscriptionSnapshot) error
	NoteWebhookFailure(context.Context, string) error
}

// BillingStateReader returns one coherent local snapshot. Missing subscription
// is normal; found=false means the shop does not exist. Operational errors must
// remain errors, never become a "subscription required" result.
// A transaction-bound implementation is required when enforcing new bookings.
type BillingStateReader interface {
	FindShopBillingState(context.Context, uuid.UUID) (ShopBillingState, bool, error)
}

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }
