// Package domain defines notification intent and delivery rules.
//
// It owns booking confirmations, reminder jobs, delivery attempts, retries,
// deduplication keys and provider result reconciliation. It does not own
// appointment state, customer identity or the meaning of completed revenue.
//
// A successful send does not mean the customer confirmed an appointment.
// Provider acceptance and final delivery are separate outcomes; an ambiguous
// timeout must not be reported as a definite rejection.
//
// This package documents the boundary only. The worker runtime and generic
// database/broker connections remain outside this business module. No provider,
// scheduler or message consumer is implemented yet.
package domain
