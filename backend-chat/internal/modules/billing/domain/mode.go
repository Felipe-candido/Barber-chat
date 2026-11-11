package domain

// BillingMode separates real money from sandbox resources. The zero value keeps
// existing test-only callers safe; live mode must always be explicitly selected.
type BillingMode string

const (
	BillingTest BillingMode = "test"
	BillingLive BillingMode = "live"
)

func (m BillingMode) Effective() BillingMode {
	if m == "" {
		return BillingTest
	}
	return m
}
func (m BillingMode) Valid() bool { return m == BillingTest || m == BillingLive }
func (m BillingMode) Live() bool  { return m == BillingLive }
