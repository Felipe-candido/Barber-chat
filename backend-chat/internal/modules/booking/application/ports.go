package application

import (
	"context"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

type ShopResolver interface {
	ResolveActiveShop(context.Context, string) (ShopContext, bool, error)
}

// Read-only projections for the authorized administrative agenda. These do not
// acquire booking locks or expose customer contact through public booking ports.
type AgendaShopReader interface {
	FindAgendaShop(context.Context, uuid.UUID) (ShopContext, bool, error)
}
type AgendaReader interface {
	ListAgendaAppointments(context.Context, AgendaQuery) ([]AdminAppointment, error)
}

// Catalog reads revalidate active resources and their same-tenant association.
type BookableCatalogReader interface {
	FindBookableSelection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (BookableSelection, bool, error)
	ListBookableProfessionals(context.Context, uuid.UUID, uuid.UUID) ([]Professional, error)
}

// All calendar writers lock the same professional before reading/changing its
// policy, hours, blocks or appointments. PostgreSQL is the final overlap guard.
type ScheduleGuard interface {
	LockProfessionalSchedule(context.Context, uuid.UUID, uuid.UUID) error
	LoadCalendar(context.Context, AvailabilityQuery) (domain.Calendar, bool, error)
}

// Validation is separate from persistence. It does not prove phone ownership.
type CustomerNormalizer interface {
	Normalize(CustomerInput) (CustomerInput, error)
}
type CustomerResolver interface {
	ResolveForBooking(context.Context, CustomerResolutionInput) (CustomerResolution, error)
}

type AppointmentRepository interface {
	Insert(context.Context, domain.Appointment, [32]byte) (domain.Appointment, error)
	FindByCode(context.Context, uuid.UUID, [32]byte) (domain.Appointment, bool, error)
	SaveCancellation(context.Context, domain.Appointment) error
}

// Claim is exclusive per tenant/key. A mismatched request conflicts; a completed
// match replays its original encrypted receipt. Expired keys cannot be reused.
// Claim, contact, appointment and completion share one commit, never pool calls.
type IdempotencyRepository interface {
	ClaimCreateAppointment(context.Context, IdempotencyClaim) (ClaimedReceipt, bool, error)
	CompleteCreateAppointment(context.Context, uuid.UUID, string, ClaimedReceipt) error
}

// Codes are unpredictable bearer capabilities for one appointment only.
type AppointmentCodes interface {
	New() (string, [32]byte, error)
	Hash(string) ([32]byte, error)
}

// Identity implements this read-only port using the same transaction as booking.
// Call after the professional lock, including on an idempotent replay.
type StaffBookingAuthorizer interface {
	CanCreateBooking(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}

type BookingTx struct {
	Staff        StaffBookingAuthorizer
	Shops        ShopResolver
	Catalog      BookableCatalogReader
	Schedule     ScheduleGuard
	Customers    CustomerResolver
	Appointments AppointmentRepository
	Idempotency  IdempotencyRepository
}

// The callback runs exactly once. A nil result requires a successful commit;
// errors and panics roll back. Never retain transaction-bound adapters afterward.
type BookingTransactor interface {
	WithinBookingTx(context.Context, func(context.Context, BookingTx) error) error
}

type Clock interface{ Now() time.Time }

// ClockFunc accepts time.Now in production and a deterministic function in tests.
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }
