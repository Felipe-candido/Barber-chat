package domain

import "time"

type CheckoutIntent string

const (
	CheckoutPublish       CheckoutIntent = "publish"
	CheckoutManualBooking CheckoutIntent = "manual_booking"
)

func (i CheckoutIntent) Valid() bool {
	return i == CheckoutPublish || i == CheckoutManualBooking
}

// Stripe may prune idempotency keys after 24 hours. Stop well before that
// boundary if an external result was never linked; time alone is not evidence
// that no Customer, Session or Subscription was created.
const CheckoutRecoveryWindow = 20 * time.Hour

func CanRecoverCheckout(createdAt, now time.Time) bool {
	return !createdAt.IsZero() && !now.Before(createdAt) && now.Sub(createdAt) < CheckoutRecoveryWindow
}
