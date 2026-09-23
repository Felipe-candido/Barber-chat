// Package domain defines appointment and calendar rules.
//
// It owns appointments, their state transitions, immutable service snapshots,
// working hours, breaks, time off and calendar blocks. Availability combines
// those rules with tenant-scoped catalog data and the shop's IANA timezone.
//
// Customer contact records belong to customers. Service definitions belong to
// catalog. Delivery attempts belong to notifications. Booking owns the decision
// that an appointment was created, confirmed, cancelled or completed.
//
// The planned states are scheduled, confirmed, cancelled, completed and no_show.
// Overlap protection is a database invariant as well as a business rule; pure
// Go validation must never be presented as a double-booking guarantee.
//
// This package documents the domain boundary only. The sibling application package
// will orchestrate real workflows; no appointment behavior is implemented yet.
package domain
