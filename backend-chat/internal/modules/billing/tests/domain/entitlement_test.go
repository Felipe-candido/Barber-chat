package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

func TestEvaluateEntitlement(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	grace := end.Add(48 * time.Hour)
	for _, test := range []struct {
		name                         string
		status                       domain.SubscriptionStatus
		now                          time.Time
		paid, tolerance, cancelAtEnd bool
		allowed                      bool
		reason                       domain.AccessReason
		until                        time.Time
	}{
		{"pending is not paid", domain.SubscriptionPending, start, false, false, false, false, domain.AccessPaymentPending, time.Time{}},
		{"before coverage", domain.SubscriptionActive, start.Add(-time.Nanosecond), true, false, false, false, domain.AccessPaymentRequired, time.Time{}},
		{"coverage starts inclusively", domain.SubscriptionActive, start, true, false, false, true, domain.AccessSubscribed, end},
		{"covered", domain.SubscriptionActive, end.Add(-time.Nanosecond), true, false, false, true, domain.AccessSubscribed, end},
		{"coverage ends exclusively", domain.SubscriptionActive, end, true, false, false, false, domain.AccessPaymentRequired, time.Time{}},
		{"stale active status expires", domain.SubscriptionActive, grace, true, false, false, false, domain.AccessPaymentRequired, time.Time{}},
		{"scheduled cancellation keeps coverage", domain.SubscriptionActive, start, true, false, true, true, domain.AccessSubscribed, end},
		{"scheduled cancellation does not extend coverage", domain.SubscriptionActive, end, true, false, true, false, domain.AccessPaymentRequired, time.Time{}},
		{"past due retains paid interval", domain.SubscriptionPastDue, start, true, true, false, true, domain.AccessSubscribed, end},
		{"past due with explicit grace", domain.SubscriptionPastDue, end, true, true, false, true, domain.AccessGracePeriod, grace},
		{"last moment of grace", domain.SubscriptionPastDue, grace.Add(-time.Nanosecond), true, true, false, true, domain.AccessGracePeriod, grace},
		{"grace ends exclusively", domain.SubscriptionPastDue, grace, true, true, false, false, domain.AccessPaymentRequired, time.Time{}},
		{"no implicit grace", domain.SubscriptionPastDue, end, true, false, false, false, domain.AccessPaymentRequired, time.Time{}},
		{"unpaid initial subscription has no access", domain.SubscriptionPastDue, start, false, false, false, false, domain.AccessPaymentRequired, time.Time{}},
		{"suspension revokes even with coverage", domain.SubscriptionSuspended, start, true, false, false, false, domain.AccessSuspended, time.Time{}},
		{"effective cancellation revokes", domain.SubscriptionCanceled, start, true, false, false, false, domain.AccessCanceled, time.Time{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := domain.Subscription{ShopID: uuid.New(), Status: test.status, CancelAtPeriodEnd: test.cancelAtEnd}
			if test.paid {
				s.PaidFrom, s.PaidUntil = start, end
			}
			if test.tolerance {
				s.GraceUntil = grace
			}
			got, err := domain.EvaluateEntitlement(&s, test.now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Allowed != test.allowed || got.Reason != test.reason || !got.Until.Equal(test.until) {
				t.Fatalf("got %+v; want allowed=%v reason=%s until=%s", got, test.allowed, test.reason, test.until)
			}
		})
	}
	t.Run("missing subscription", func(t *testing.T) {
		got, err := domain.EvaluateEntitlement(nil, start)
		if err != nil || got.Allowed || got.Reason != domain.AccessSubscriptionRequired || !got.Until.IsZero() {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("invalid clock", func(t *testing.T) {
		if _, err := domain.EvaluateEntitlement(nil, time.Time{}); !errors.Is(err, domain.ErrInvalidEvaluationTime) {
			t.Fatal(err)
		}
	})
	t.Run("same instant in another timezone", func(t *testing.T) {
		s := domain.Subscription{ShopID: uuid.New(), Status: domain.SubscriptionActive, PaidFrom: start, PaidUntil: end}
		got, err := domain.EvaluateEntitlement(&s, start.In(time.FixedZone("client", -3*3600)))
		if err != nil || !got.Allowed || got.Until.Location() != time.UTC {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
}

func TestInvalidSubscriptionNeverGrantsAccess(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*domain.Subscription)
	}{
		{"nil tenant", func(s *domain.Subscription) { s.ShopID = uuid.Nil }},
		{"unknown status", func(s *domain.Subscription) { s.Status = "trialing" }},
		{"empty status", func(s *domain.Subscription) { s.Status = "" }},
		{"missing start", func(s *domain.Subscription) { s.PaidFrom = time.Time{} }},
		{"missing end", func(s *domain.Subscription) { s.PaidUntil = time.Time{} }},
		{"zero interval", func(s *domain.Subscription) { s.PaidUntil = s.PaidFrom }},
		{"reversed interval", func(s *domain.Subscription) { s.PaidUntil = s.PaidFrom.Add(-time.Second) }},
		{"active without coverage", func(s *domain.Subscription) { s.PaidFrom, s.PaidUntil = time.Time{}, time.Time{} }},
		{"pending with paid coverage", func(s *domain.Subscription) { s.Status = domain.SubscriptionPending }},
		{"pending with scheduled cancellation", func(s *domain.Subscription) {
			s.Status = domain.SubscriptionPending
			s.PaidFrom, s.PaidUntil = time.Time{}, time.Time{}
			s.CancelAtPeriodEnd = true
		}},
		{"grace on active", func(s *domain.Subscription) { s.GraceUntil = s.PaidUntil.Add(time.Hour) }},
		{"grace without previous payment", func(s *domain.Subscription) {
			s.Status = domain.SubscriptionPastDue
			s.PaidFrom, s.PaidUntil = time.Time{}, time.Time{}
			s.GraceUntil = now.Add(time.Hour)
		}},
		{"grace ending at payment", func(s *domain.Subscription) { s.Status = domain.SubscriptionPastDue; s.GraceUntil = s.PaidUntil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := domain.Subscription{ShopID: uuid.New(), Status: domain.SubscriptionActive, PaidFrom: now.Add(-time.Hour), PaidUntil: now.Add(time.Hour)}
			test.mutate(&s)
			got, err := domain.EvaluateEntitlement(&s, now)
			if !errors.Is(err, domain.ErrInvalidSubscription) || got.Allowed {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}
