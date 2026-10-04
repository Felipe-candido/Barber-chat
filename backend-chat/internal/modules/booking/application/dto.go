package application

import (
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

// CustomerInput is unverified contact data, not a logged-in user or proof that
// the caller owns a phone number. Keep it in memory until final submission.
// Customers owns metadata-based phone validation and normalization; PhoneCountry
// is an ISO 3166-1 alpha-2 country code for numbers supplied in national format.
type CustomerInput struct {
	FirstName    string
	LastName     string
	Phone        string
	PhoneCountry string
}

// CreateAppointmentInput describes only what the public caller may request.
// ShopSlug comes from the route; it must be resolved to an active shop server-side.
// There is intentionally no ShopID, CustomerID, status, price, duration or EndsAt.
// IdempotencyKey comes from the header and must be kept unchanged during retries.
// The use case will validate these fields before making transactional changes.
type CreateAppointmentInput struct {
	ShopSlug       string
	ServiceID      uuid.UUID
	ProfessionalID uuid.UUID
	StartsAt       time.Time
	Customer       CustomerInput
	IdempotencyKey string
}

// ListAvailabilityInput asks about a civil date in the shop's named timezone.
// LocalDate uses YYYY-MM-DD; it is not midnight UTC or the browser's timezone.
// A returned slot is an observation, not a hold or an authorization to reserve it.
type ListAvailabilityInput struct {
	ShopSlug       string
	ServiceID      uuid.UUID
	ProfessionalID uuid.UUID
	LocalDate      string
}

// AvailabilitySlot contains UTC instants for the RESERVED [StartsAt, EndsAt).
// EndsAt includes SlotPolicy padding/rounding, not just the service's duration.
type AvailabilitySlot struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// AvailabilityOutput is safe for public display; an empty result uses [] slots.
type AvailabilityOutput struct {
	ShopTimezone string
	Slots        []AvailabilitySlot
}

// AppointmentReceipt is the public result retained for idempotent retries.
// It excludes customer contact data and internal customer IDs. Knowing an
// appointment ID does not authorize reading, cancelling or confirming it.
// EndsAt is the reserved calendar end; Service.DurationMinutes is the estimate.
type AppointmentReceipt struct {
	ShopName         string
	AppointmentID    uuid.UUID
	Status           domain.Status
	Service          domain.ServiceSnapshot
	ProfessionalID   uuid.UUID
	ProfessionalName string
	StartsAt         time.Time
	EndsAt           time.Time
	ShopTimezone     string
}

// CreateAppointmentOutput separates transport metadata from the stable receipt.
// Replayed must not change the receipt stored for the first successful request.
// HTTP/JSON mapping belongs to infra, so these contracts have no JSON tags.
type CreateAppointmentOutput struct {
	Appointment AppointmentReceipt
	Code        string
	Replayed    bool
}

type Professional struct {
	ID   uuid.UUID
	Name string
}

type ProfessionalsOutput struct {
	ShopName        string
	ShopTimezone    string
	Today           string
	LastBookingDate string
	Professionals   []Professional
}

type AppointmentCodeInput struct{ ShopSlug, Code string }
type AppointmentDetails struct {
	Appointment AppointmentReceipt
	CanCancel   bool
}
