package application

import (
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

// ShopID must come from successful shop authorization, never a trusted JSON ID.
// This query does not replace staff authorization or disclose payment details.
type GetShopEntitlementsInput struct {
	ShopID uuid.UUID
}

type ShopBillingState struct {
	ShopID       uuid.UUID
	PlanKey      string
	Active       bool
	Subscription *domain.Subscription
}

// Eligibility does not publish a shop. Shops also checks publication intent and
// configuration readiness. Entitlement does not grant an administrative role.
type ShopEntitlementsOutput struct {
	ShopID      uuid.UUID
	Entitlement domain.Entitlement
}
