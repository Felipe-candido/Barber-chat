package domain_test

import (
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

func TestConfirmedCoverageNeverBridgesGaps(t *testing.T) {
	now := time.Now().UTC()
	periods := []domain.Coverage{{From: now.Add(time.Hour), Until: now.Add(2 * time.Hour)}, {From: now.Add(-time.Hour), Until: now.Add(30 * time.Minute)}}
	sub := domain.Subscription{ShopID: uuid.New(), Status: domain.SubscriptionActive, PaidFrom: now.Add(-time.Hour), PaidUntil: now.Add(2 * time.Hour), PaidPeriods: periods}
	got, err := domain.EvaluateEntitlement(&sub, now.Add(45*time.Minute))
	if err != nil || got.Allowed {
		t.Fatal("unpaid gap granted access", got, err)
	}
	got, err = domain.EvaluateEntitlement(&sub, now)
	if err != nil || !got.Allowed || !got.Until.Equal(now.Add(30*time.Minute)) {
		t.Fatal(got, err)
	}
	if !periods[0].From.Equal(now.Add(time.Hour)) {
		t.Fatal("evaluation mutated shared state")
	}
	sub.PaidPeriods = append(sub.PaidPeriods, domain.Coverage{From: now.Add(30 * time.Minute), Until: now.Add(time.Hour)})
	got, err = domain.EvaluateEntitlement(&sub, now)
	if err != nil || !got.Until.Equal(now.Add(2*time.Hour)) {
		t.Fatal("touching periods not coalesced", got, err)
	}
	got, err = domain.EvaluateEntitlement(&sub, now.Add(2*time.Hour))
	if err != nil || got.Allowed {
		t.Fatal("exclusive end granted access")
	}
	sub.PaidPeriods = []domain.Coverage{}
	got, err = domain.EvaluateEntitlement(&sub, now)
	if err != nil || got.Allowed {
		t.Fatal("invalidated invoices fell back to legacy interval")
	}
}
