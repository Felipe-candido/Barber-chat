package application

import "errors"

// Business failures for the first availability/creation workflows. Adapters must
// preserve context cancellation and operational causes instead of disguising them
// as absence, invalid input or a scheduling conflict. Never include contact data.
var (
	ErrShopNotFound            = errors.New("active shop not found")
	ErrBookingOptionNotFound   = errors.New("bookable service and professional selection not found")
	ErrInvalidAvailability     = errors.New("invalid availability request")
	ErrInvalidAppointment     = errors.New("invalid appointment request")
	ErrInvalidCustomer        = errors.New("invalid customer contact or consent")
	ErrSlotUnavailable        = errors.New("requested appointment interval is unavailable")
	ErrIdempotencyConflict    = errors.New("idempotency key already used with a different request")
	ErrInconsistentBookingData = errors.New("booking dependency returned inconsistent data")
)
