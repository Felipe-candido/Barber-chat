package domain_test

import (
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
)

func TestCheckoutRecoveryWindow(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		created time.Time
		allowed bool
	}{
		{now, true}, {now.Add(-19 * time.Hour), true},
		{now.Add(-20 * time.Hour), false}, {now.Add(-24 * time.Hour), false},
		{now.Add(time.Second), false}, {time.Time{}, false},
	} {
		if domain.CanRecoverCheckout(tc.created, now) != tc.allowed {
			t.Fatal(tc)
		}
	}
	if !domain.CheckoutPublish.Valid() || !domain.CheckoutManualBooking.Valid() || domain.CheckoutIntent("other").Valid() {
		t.Fatal("unexpected intents")
	}
}
