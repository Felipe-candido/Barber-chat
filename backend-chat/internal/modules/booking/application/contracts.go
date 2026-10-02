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

// ScheduleCheck is recomputed from the requested start and effective duration.
// The professional's common agenda lock must already be held in the transaction.
type ScheduleCheck struct {
	ShopID         uuid.UUID
	ProfessionalID uuid.UUID
	StartsAt       time.Time
	EndsAt         time.Time
}

// CustomerResolutionInput is submitted only after validating final booking data.
// Resolving contacts and recording consent must share the booking transaction.
// RecordedAt is supplied by the server clock. Customer data must not be logged.
type CustomerResolutionInput struct {
	ShopID     uuid.UUID
	Customer   CustomerInput
	Consent    ConsentInput
	RecordedAt time.Time
}

// CustomerResolution never exposes an existing customer's profile or history.
// BookingMessagesAllowed applies to this request's validated consent, not an
// assumed opt-in based on an old record or the mere presence of a phone number.
type CustomerResolution struct {
	CustomerID             uuid.UUID
	BookingMessagesAllowed bool
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

// AppointmentNotificationPlan contains references, not phone numbers or broker
// messages. Notifications owns reminder policy, intent/job/outbox persistence
// and later delivery. This request's allowed messaging consent is explicit.
type AppointmentNotificationPlan struct {
	ShopID                 uuid.UUID
	AppointmentID          uuid.UUID
	AppointmentVersion     int64
	CustomerID             uuid.UUID
	StartsAt               time.Time
	ShopTimezone           string
	RecordedAt             time.Time
	BookingMessagesAllowed bool
}
