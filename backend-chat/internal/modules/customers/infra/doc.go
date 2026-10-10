// Package infra implements contact normalization and transactional persistence.
//
// ContactNormalizer validates names and uses phone metadata to normalize E.164.
// A valid phone number is not verified ownership. BookingContacts finds or creates
// a same-shop contact inside the booking transaction, without overwriting an
// existing person's profile using unverified public input.
//
// No incomplete-customer endpoint, contact history lookup, consent collection or
// WhatsApp delivery is implemented. Customer data must not appear in public
// receipts, logs or persistent frontend booking state.
package infra
