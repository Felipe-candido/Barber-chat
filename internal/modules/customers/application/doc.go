// Package application is the home of customer use cases.
//
// Planned workflows, not implemented yet:
//   - FindOrCreateCustomer: resolve a tenant-local contact at final booking.
//   - RecordConsent and RevokeConsent: keep purpose-specific consent history.
//   - ListCustomers and GetCustomer: authorized administrative reads.
//
// FindOrCreateCustomer participates in the caller's booking transaction and
// must not commit independently. Concurrent creation needs a tenant-scoped
// database uniqueness strategy; a prior lookup alone is insufficient.
//
// Phone normalization must validate country/number metadata, not just strip
// punctuation. Matching a phone must not reveal history or blindly overwrite
// an existing customer's identity with unverified public input.
//
// No public pre-registration route is planned. Inputs, outputs and narrow ports
// will be introduced with concrete workflows; none is executable in this step.
package application
