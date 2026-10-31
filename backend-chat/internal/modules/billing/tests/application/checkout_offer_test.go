package application_test

import (
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/google/uuid"
)

func TestOfferConfirmationBindsTenantAndReviewedConditions(t *testing.T) {
	original := application.CheckoutOffer{ShopID: uuid.New(), PlanKey: "monthly", PriceID: "price_original", AmountCents: 9900}
	id := original.ConfirmationID()
	if len(id) != 64 || id != original.ConfirmationID() {
		t.Fatal("confirmation must be stable")
	}
	for _, field := range []string{"tenant", "plan", "price", "amount"} {
		changed := original
		switch field {
		case "tenant":
			changed.ShopID = uuid.New()
		case "plan":
			changed.PlanKey = "another_monthly"
		case "price":
			changed.PriceID = "price_new"
		case "amount":
			changed.AmountCents++
		}
		if changed.ConfirmationID() == id {
			t.Fatal("confirmation ignored", field)
		}
	}
}
