// Package application implements public availability, professional selection,
// appointment creation, receipt lookup and cancellation through consumer-owned
// ports. Domain and application never import HTTP, pgx or another business module.
//
// CreateAppointment claims idempotency, locks the professional, rechecks catalog
// and calendar, resolves the contact, inserts snapshots and stores an encrypted
// receipt in one transaction. Availability is an observation, never a reservation.
// The PostgreSQL exclusion constraint is the final protection against overlaps.
//
// Lookup/cancellation require a random code scoped to one appointment and shop,
// not a customer phone or appointment UUID. Cancellation is allowed before start;
// repeating a successful cancellation is harmless. WhatsApp is not implemented.
package application
