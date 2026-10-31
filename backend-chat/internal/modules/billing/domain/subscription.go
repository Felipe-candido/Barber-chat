package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidSubscription = errors.New("invalid subscription")

// SubscriptionStatus describes local business state, not Stripe's SDK types.
// The future provider adapter must explicitly translate each external status.
type SubscriptionStatus string

const (
	SubscriptionPending   SubscriptionStatus = "pending"
	SubscriptionActive    SubscriptionStatus = "active"
	SubscriptionPastDue   SubscriptionStatus = "past_due"
	SubscriptionSuspended SubscriptionStatus = "suspended"
	SubscriptionCanceled  SubscriptionStatus = "canceled"
)

func (s SubscriptionStatus) Valid() bool {
	switch s {
	case SubscriptionPending, SubscriptionActive, SubscriptionPastDue, SubscriptionSuspended, SubscriptionCanceled:
		return true
	default:
		return false
	}
}

// Subscription is the local access projection. PaidFrom/PaidUntil must be based
// on confirmed coverage, never merely on a provider's next invoicing period.
// Zero timestamps mean no confirmed coverage. Intervals are [from, until).
// GraceUntil is an explicit deadline, not a duration restarted on each request.
type Subscription struct {
	ShopID            uuid.UUID
	Status            SubscriptionStatus
	PaidFrom          time.Time
	PaidUntil         time.Time
	GraceUntil        time.Time
	CancelAtPeriodEnd bool
	// Nil preserves the legacy single interval. Non-nil is authoritative,
	// including an empty list after invalidation of confirmed invoices.
	PaidPeriods []Coverage
}

func (s Subscription) Validate() error {
	for _, period := range s.PaidPeriods {
		if !period.Valid() {
			return ErrInvalidSubscription
		}
	}
	if s.ShopID == uuid.Nil || !s.Status.Valid() {
		return ErrInvalidSubscription
	}
	hasCoverage := !s.PaidFrom.IsZero()
	if hasCoverage != !s.PaidUntil.IsZero() || (hasCoverage && !s.PaidUntil.After(s.PaidFrom)) {
		return ErrInvalidSubscription
	}
	if s.Status == SubscriptionActive && !hasCoverage {
		return ErrInvalidSubscription
	}
	if s.Status == SubscriptionPending && (hasCoverage || s.CancelAtPeriodEnd) {
		return ErrInvalidSubscription
	}
	if !s.GraceUntil.IsZero() && (s.Status != SubscriptionPastDue || !hasCoverage || !s.GraceUntil.After(s.PaidUntil)) {
		return ErrInvalidSubscription
	}
	return nil
}
