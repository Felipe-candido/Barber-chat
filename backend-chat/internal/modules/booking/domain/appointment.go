package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status describes the appointment lifecycle, not message delivery.
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
// DurationMinutes remains the estimated service duration, not rounded occupancy.
type ServiceSnapshot struct {
	Name            string
	DurationMinutes int
	PriceCents      int64
	Currency        string
}

// Appointment is the booking-owned record shared with its persistence port.
// Creation and cancellation validate local invariants; the transaction and
// PostgreSQL exclusion constraint enforce availability across appointments.
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

	// Instants must be normalized to UTC. EndsAt is the end of the RESERVED
	// interval, computed using SlotPolicy, not just Service.DurationMinutes.
	// For a 40-minute service on a 60-minute grid, 09:30 reserves through 10:30.
	// The interval is [StartsAt, EndsAt), allowing another start exactly at EndsAt.
	StartsAt time.Time
	EndsAt   time.Time
	Status   Status
	// Version supports future audited changes and invalidation of stale reminders.
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrInvalidAppointment = errors.New("invalid appointment")
	ErrCancellationClosed = errors.New("appointment can no longer be cancelled")
)

// NewAppointment receives server-owned snapshots and an already calculated
// reserved interval. It does not generate IDs or read the wall clock itself.
func NewAppointment(a Appointment, now time.Time) (Appointment, error) {
	if a.ID == uuid.Nil || a.ShopID == uuid.Nil || a.CustomerID == uuid.Nil ||
		a.ServiceID == uuid.Nil || a.ProfessionalID == uuid.Nil || now.IsZero() ||
		strings.TrimSpace(a.Service.Name) == "" || strings.TrimSpace(a.ProfessionalName) == "" ||
		a.Service.DurationMinutes < 1 || a.Service.DurationMinutes > 1440 ||
		a.Service.PriceCents < 0 || a.Service.Currency != "BRL" ||
		!a.StartsAt.After(now) || a.StartsAt.Second() != 0 || a.StartsAt.Nanosecond() != 0 ||
		!a.EndsAt.After(a.StartsAt) || a.EndsAt.Sub(a.StartsAt) < time.Duration(a.Service.DurationMinutes)*time.Minute {
		return Appointment{}, ErrInvalidAppointment
	}
	if a.ShopTimezone == "" {
		return Appointment{}, ErrInvalidAppointment
	}
	if _, err := time.LoadLocation(a.ShopTimezone); err != nil {
		return Appointment{}, ErrInvalidAppointment
	}
	a.StartsAt, a.EndsAt = a.StartsAt.UTC(), a.EndsAt.UTC()
	a.Status, a.Version = StatusScheduled, 1
	a.CreatedAt, a.UpdatedAt = now.UTC(), now.UTC()
	return a, nil
}

func (a Appointment) CanCancel(now time.Time) bool {
	return !now.IsZero() && now.Before(a.StartsAt) &&
		(a.Status == StatusScheduled || a.Status == StatusConfirmed)
}

// Cancel is idempotent: a repeated cancellation must not increment the version.
func (a Appointment) Cancel(now time.Time) (Appointment, error) {
	if a.Status == StatusCancelled {
		return a, nil
	}
	if !a.CanCancel(now) {
		return Appointment{}, ErrCancellationClosed
	}
	a.Status = StatusCancelled
	a.Version++
	a.UpdatedAt = now.UTC()
	return a, nil
}
