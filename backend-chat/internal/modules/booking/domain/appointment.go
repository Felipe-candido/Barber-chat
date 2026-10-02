package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status describes the appointment lifecycle, not message delivery.
// Constructors and transition rules will validate these values in the next step.
type Status string

const (
	StatusScheduled Status = "scheduled"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
	StatusCompleted Status = "completed"
	StatusNoShow    Status = "no_show"
)

// ServiceSnapshot preserves the effective service terms at booking time.
// Its values come from the tenant-scoped catalog, never from the public form.
// Catalog changes must not rewrite an existing appointment's snapshot.
type ServiceSnapshot struct {
	Name            string
	DurationMinutes int
	PriceCents      int64
	Currency        string
}

// Appointment is the booking-owned record shared with its persistence port.
// It is a data contract only: these declarations do not implement validation,
// state transitions, availability checks or database overlap protection.
// New appointments will start scheduled, with version 1 and server-generated IDs.
type Appointment struct {
	ID             uuid.UUID
	ShopID         uuid.UUID
	CustomerID     uuid.UUID
	ServiceID      uuid.UUID
	ProfessionalID uuid.UUID

	Service          ServiceSnapshot
	ProfessionalName string
	// ShopTimezone preserves the named timezone used when the booking was made.
	ShopTimezone string

	// Instants must be normalized to UTC. The interval is [StartsAt, EndsAt),
	// so an appointment ending at 10:00 may be followed by another at 10:00.
	StartsAt time.Time
	EndsAt   time.Time
	Status   Status
	// Version supports future audited changes and invalidation of stale reminders.
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
