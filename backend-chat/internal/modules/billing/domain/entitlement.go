package domain

import (
	"errors"
	"time"
)

var ErrInvalidEvaluationTime = errors.New("invalid entitlement evaluation time")

type AccessReason string

const (
	AccessSubscribed           AccessReason = "subscribed"
	AccessGracePeriod          AccessReason = "grace_period"
	AccessSubscriptionRequired AccessReason = "subscription_required"
	AccessPaymentPending       AccessReason = "payment_pending"
	AccessPaymentRequired      AccessReason = "payment_required"
	AccessSuspended            AccessReason = "subscription_suspended"
	AccessCanceled             AccessReason = "subscription_canceled"
)

// Entitlement governs new bookings and eligibility to publish, not login,
// configuration, billing management or handling existing appointments.
// Until is exclusive and is nonzero only when operation is allowed.
type Entitlement struct {
	Allowed bool
	Reason  AccessReason
	Until   time.Time
}

// EvaluateEntitlement is pure: the caller supplies the clock and the persisted
// snapshot. It never calls Stripe and never grants indefinite access from status.
func EvaluateEntitlement(subscription *Subscription, now time.Time) (Entitlement, error) {
	if now.IsZero() {
		return Entitlement{}, ErrInvalidEvaluationTime
	}
	if subscription == nil {
		return Entitlement{Reason: AccessSubscriptionRequired}, nil
	}
	if err := subscription.Validate(); err != nil {
		return Entitlement{}, err
	}
	s := *subscription
	switch s.Status {
	case SubscriptionPending:
		return Entitlement{Reason: AccessPaymentPending}, nil
	case SubscriptionSuspended:
		return Entitlement{Reason: AccessSuspended}, nil
	case SubscriptionCanceled:
		return Entitlement{Reason: AccessCanceled}, nil
	}
	if s.PaidPeriods != nil {
		if until := CoveredUntil(s.PaidPeriods, now); !until.IsZero() {
			return Entitlement{Allowed: true, Reason: AccessSubscribed, Until: until}, nil
		}
		return Entitlement{Reason: AccessPaymentRequired}, nil
	}
	if !s.PaidFrom.IsZero() && !now.Before(s.PaidFrom) {
		if now.Before(s.PaidUntil) {
			return Entitlement{Allowed: true, Reason: AccessSubscribed, Until: s.PaidUntil.UTC()}, nil
		}
		if s.Status == SubscriptionPastDue && now.Before(s.GraceUntil) {
			return Entitlement{Allowed: true, Reason: AccessGracePeriod, Until: s.GraceUntil.UTC()}, nil
		}
	}
	return Entitlement{Reason: AccessPaymentRequired}, nil
}
