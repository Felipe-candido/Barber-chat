package application

import (
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

// ShopContext is the booking-owned projection of an active shop, not its domain
// entity. The named timezone must be valid before calendar calculations begin.
type ShopContext struct {
	ShopID   uuid.UUID
	Slug     string
	Name     string
	Timezone string
}

// BookableSelection is the effective catalog data for exactly one shop/service/
// professional pair. Readers must verify all three resources are active and the
// professional is enabled to perform that service in the same shop.
type BookableSelection struct {
	ShopID           uuid.UUID
	ServiceID        uuid.UUID
	ProfessionalID   uuid.UUID
	ProfessionalName string
	Service          domain.ServiceSnapshot
}

// AvailabilityQuery is built after resolving the shop and validating selection.
// Its tenant ID and timezone come from the server, not public query parameters.
// Readers return a consistent observation of working hours, exceptions and
// occupied intervals, but do not create or hold an appointment.
type AvailabilityQuery struct {
	ShopID         uuid.UUID
	ServiceID      uuid.UUID
	ProfessionalID uuid.UUID
	LocalDate      string
	ShopTimezone   string
}

// ClaimedReceipt stores the original response, including its one-appointment
// capability. Infrastructure encrypts it with the high-entropy request key;
// neither that key nor the plaintext code is persisted.
type ClaimedReceipt struct {
	Appointment AppointmentReceipt
	Code        string
}

// CustomerResolutionInput is submitted only after validating final booking data.
// Resolving contacts must share the booking transaction.
// RecordedAt is supplied by the server clock. Customer data must not be logged.
type CustomerResolutionInput struct {
	ShopID     uuid.UUID
	Customer   CustomerInput
	RecordedAt time.Time
}

// CustomerResolution never exposes an existing customer's profile or history.
type CustomerResolution struct {
	CustomerID uuid.UUID
}

// IdempotencyClaim reserves the create-appointment operation's tenant-scoped key.
// RequestFingerprint is a hash of the canonical validated caller input, excluding
// generated IDs, clock values and mutable catalog prices. Never retain a raw body.
// RequestedAt/ExpiresAt come from the server; retention is an explicit policy.
type IdempotencyClaim struct {
	ShopID             uuid.UUID
	Key                string
	RequestFingerprint [32]byte
	RequestedAt        time.Time
	ExpiresAt          time.Time
}
