// Package domain defines tenant-local customer contact and consent data.
//
// It owns customer names, normalized phone numbers and consent records with
// purpose, notice version, timestamp and revocation. Booking-message consent
// and marketing consent are separate. A customer has no login in the MVP.
//
// A supplied phone number is contact data, not proof of identity. Customer data
// and history are never shared across shops merely because a phone matches.
//
// This package currently documents the boundary only. No customer is persisted
// during the progressive public form; domain behavior will arrive with the
// first confirmed booking workflow.
package domain
