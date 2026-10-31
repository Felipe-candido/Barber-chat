package domain

// Publication records the owner's intent, not a subscription or an active flag.
// Requested must only be set after the publication workflow validates readiness.
type Publication struct {
	Requested bool
}

// PublicBookingEnabled combines independent conditions. Paying never publishes
// automatically; losing entitlement hides booking without erasing owner intent.
func (p Publication) PublicBookingEnabled(shopActive, operationAllowed bool) bool {
	return p.Requested && shopActive && operationAllowed
}
