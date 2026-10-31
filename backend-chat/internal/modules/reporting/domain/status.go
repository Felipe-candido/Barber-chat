package domain

// AppointmentStatus is a read projection of booking's stored lifecycle.
// Reporting cannot perform transitions or infer completion from a past date.
// Keep this mapping aligned with the booking schema, without importing booking.
type AppointmentStatus string

const (
	StatusScheduled AppointmentStatus = "scheduled"
	StatusConfirmed AppointmentStatus = "confirmed"
	StatusCancelled AppointmentStatus = "cancelled"
	StatusCompleted AppointmentStatus = "completed"
	StatusNoShow    AppointmentStatus = "no_show"
)

func (s AppointmentStatus) Valid() bool {
	switch s {
	case StatusScheduled, StatusConfirmed, StatusCancelled, StatusCompleted, StatusNoShow:
		return true
	default:
		return false
	}
}

// Consumer-owned projection of stored booking provenance, not a transition command.
type AppointmentStatusSource string

const (
	SourceLegacy    AppointmentStatusSource = "legacy"
	SourceBooking   AppointmentStatusSource = "booking"
	SourcePublic    AppointmentStatusSource = "public"
	SourceStaff     AppointmentStatusSource = "staff"
	SourceAutomatic AppointmentStatusSource = "automatic"
)

func (s AppointmentStatusSource) Valid() bool {
	switch s {
	case SourceLegacy, SourceBooking, SourcePublic, SourceStaff, SourceAutomatic:
		return true
	default:
		return false
	}
}
