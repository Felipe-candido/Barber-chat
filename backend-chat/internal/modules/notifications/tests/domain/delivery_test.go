package domain_test

import (
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
)

func TestDeliveryRecoveryBoundary(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	if !domain.DeliveryLeased.ReclaimableAt(now, now) || domain.DeliveryLeased.ReclaimableAt(now, now.Add(time.Microsecond)) {
		t.Fatal("lease boundary")
	}
	for _, s := range []domain.DeliveryState{domain.DeliverySending, domain.DeliveryAccepted, domain.DeliveryUnknown, domain.DeliveryFailed, domain.DeliveryInvalidated, domain.DeliveryExpired} {
		if s.ReclaimableAt(now, now.Add(-time.Hour)) {
			t.Fatal("external/terminal work became claimable", s)
		}
	}
	if !domain.DeliverySending.StartedLeaseExpired(now, now) || domain.DeliveryLeased.StartedLeaseExpired(now, now) {
		t.Fatal("unsafe recovery classification")
	}
}
func TestSendOutcomeFailsClosed(t *testing.T) {
	for _, o := range []domain.SendOutcome{
		{}, {Disposition: domain.SendAccepted}, {Disposition: domain.SendAccepted, ProviderMessageID: "secret\nbody"},
		{Disposition: domain.SendAccepted, ProviderMessageID: "id", Reason: domain.ReasonTransportUnknown},
		{Disposition: domain.SendNotSent, Reason: "raw provider error"},
		{Disposition: domain.SendNotSent, ProviderMessageID: "id", Reason: domain.ReasonProviderRejected},
	} {
		n := o.Normalize()
		if n.State() != domain.DeliveryUnknown || n.ProviderMessageID != "" || n.Reason != domain.ReasonInvalidResponse {
			t.Fatal("invalid response asserted a definite result")
		}
	}
	o := domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: "ABC_123:xyz-456"}
	if o.Normalize() != o {
		t.Fatal("accepted evidence lost")
	}
}
