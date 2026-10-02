package application

import (
	"context"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

// ShopResolver supplies an active tenant and its named timezone. Resolution is
// not administrative authorization. Missing/inactive shops return found=false;
// operational errors stay errors. A found result must match the requested slug.
// ResolveActiveShop is distinct from the existing UUID-only LookupActiveShop.
type ShopResolver interface {
	ResolveActiveShop(ctx context.Context, slug string) (shop ShopContext, found bool, err error)
}

// BookableCatalogReader supplies authoritative price/duration and checks the
// active service, professional and their association within exactly shopID.
// Missing, inactive or cross-tenant resources return found=false, not foreign
// data. A found selection must contain the exact requested IDs and valid terms.
// The creation workflow rechecks this selection under its agenda lock.
type BookableCatalogReader interface {
	FindBookableSelection(ctx context.Context, shopID, serviceID, professionalID uuid.UUID) (selection BookableSelection, found bool, err error)
}

// AvailabilityReader reads one consistent calendar observation using the shop's
// timezone, effective duration, working hours, exceptions and occupied intervals.
// Results are UTC, ordered by StartsAt, unique and non-nil, including when empty.
// It never acquires a persistent hold or guarantees later insertion will succeed.
type AvailabilityReader interface {
	ListAvailableSlots(ctx context.Context, query AvailabilityQuery) ([]AvailabilitySlot, error)
}

// ScheduleGuard participates only in a booking transaction. All future writers
// of appointments, working hours, breaks, time off and blocks must use the same
// tenant/professional locking protocol before checking or changing the calendar.
// The lock is held until transaction end; it must not be a process-local mutex.
// Interval checks cover the entire [StartsAt, EndsAt), not only the start time.
// Database overlap constraints remain required even after a successful check.
type ScheduleGuard interface {
	LockProfessionalSchedule(ctx context.Context, shopID, professionalID uuid.UUID) error
	IsIntervalAvailable(ctx context.Context, check ScheduleCheck) (bool, error)
}

// CustomerResolver is a consumer-owned port implemented by the customer boundary.
// It validates/normalizes phone data using country metadata, resolves a contact
// within ShopID, and records validated consent using the supplied server instant.
// A phone match is not proof of identity: never expose history or blindly overwrite
// another person's profile. Concurrent resolution needs tenant-scoped uniqueness.
// Resolution must not commit independently of the appointment transaction.
type CustomerResolver interface {
	ResolveForBooking(ctx context.Context, input CustomerResolutionInput) (CustomerResolution, error)
}

// AppointmentWriter inserts a domain-validated record in the current transaction.
// It must preserve the generated IDs and snapshots, enforce tenant-scoped resource
// references, and translate overlap conflicts to ErrSlotUnavailable. All new
// records start scheduled/version 1. This port does not own other modules' data.
type AppointmentWriter interface {
	Insert(ctx context.Context, appointment domain.Appointment) (domain.Appointment, error)
}

// IdempotencyRepository handles only the create-appointment operation. Claim must
// atomically serialize attempts by (ShopID, Key), compare the request fingerprint
// and enforce retention. Reusing a live key with another fingerprint returns
// ErrIdempotencyConflict. A completed match returns its saved receipt/replayed=true;
// replayed=false means this transaction exclusively claimed a new request, not
// merely that a SELECT returned no rows. Never expose a partially completed result.
// Complete saves the stable receipt; claim, booking and receipt commit together.
// Context/lock/connection failures must not be translated into a key conflict.
type IdempotencyRepository interface {
	ClaimCreateAppointment(ctx context.Context, claim IdempotencyClaim) (receipt AppointmentReceipt, replayed bool, err error)
	CompleteCreateAppointment(ctx context.Context, shopID uuid.UUID, key string, receipt AppointmentReceipt) error
}

// NotificationPlanner persists intents/jobs/outbox in the booking transaction.
// It never publishes to RabbitMQ or calls a delivery provider. Without this
// request's allowed booking-message consent it returns NotificationNotRequested
// and creates no delivery jobs; otherwise successful planning returns Pending.
// Marketing is a separate purpose, not implicit permission for booking messages.
// Failures abort the same transaction rather than leave a booking without its plan.
type NotificationPlanner interface {
	PlanForAppointment(ctx context.Context, plan AppointmentNotificationPlan) (NotificationStatus, error)
}

// BookingTx groups only the capabilities required by CreateAppointment. This is
// not a generic repository registry. Every port must be non-nil and bound to the
// same concrete transaction, including shop/catalog reads. The adapters must
// hold/revalidate active shop/catalog data consistently through insertion.
// Do not retain these ports or use them outside the transactor's callback.
type BookingTx struct {
	Shops         ShopResolver
	Catalog       BookableCatalogReader
	Schedule      ScheduleGuard
	Customers     CustomerResolver
	Appointments  AppointmentWriter
	Idempotency   IdempotencyRepository
	Notifications NotificationPlanner
}

// BookingTransactor owns the concrete transaction lifecycle, without exposing a
// database driver to application/domain. It calls fn once with transaction-bound
// ports and a context inheriting the caller's cancellation/deadline. It commits
// only after fn returns nil, rolls back on callback failure/panic, propagates
// panics, and reports commit failures. It must not silently retry the callback.
// A nil return is required before reporting success; a commit error can leave
// the outcome uncertain, so later retries must retain the same idempotency key.
// Wiring of the transaction-bound adapters belongs in the composition boundary.
type BookingTransactor interface {
	WithinBookingTx(ctx context.Context, fn func(context.Context, BookingTx) error) error
}

// Clock supplies authoritative server time for validation, audit and retention.
// A test clock can make future use-case tests deterministic without sleeping.
type Clock interface {
	Now() time.Time
}
