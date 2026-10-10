// Package infra contains HTTP, PostgreSQL and cryptographic booking adapters.
//
// HTTP translates validated JSON to use-case inputs and sanitized errors to
// responses. PostgreSQL binds all BookingTx capabilities to one transaction,
// serializes calendar operations by professional and maps exclusion violations
// to slot conflicts. The bootstrap supplies adapters of the resource-owning
// modules using the same sqlc transaction, not independently committed writes.
//
// Security generates 160-bit random consultation codes, stores their SHA-256
// hashes and encrypts replay receipts with a key derived from the caller's
// 256-bit random Idempotency-Key. Neither secret belongs in URLs or logs.
//
// The HTTP rate limiter is process-local, uses RemoteAddr and does not trust
// arbitrary forwarding headers. Multi-instance deployment needs edge limits.
// No message provider is invoked by the booking transaction.
package infra
