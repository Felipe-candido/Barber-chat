package application

import (
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidPreparation  = errors.New("invalid booking notification preparation")
	ErrInvalidSnapshot     = errors.New("invalid booking notification snapshot")
	ErrInvalidPayloadScope = errors.New("invalid notification payload scope")
)

// BookingSnapshot is internal data from validated booking/shop/contact adapters,
// never a public request DTO. It is intentionally independent of their entities.
// Phone normalization and permission verification belong to their owning module.
type BookingSnapshot struct {
	ShopName         string
	ShopSlug         string
	ShopAddress      string
	CustomerName     string
	RecipientPhone   string
	AppointmentCode  string
	ProfessionalName string
	ServiceName      string
	PriceCents       int64
	Currency         string
	StartsAt         time.Time
	EndsAt           time.Time
	ShopTimezone     string
}

// These guards prevent accidental fmt/slog dumps of capabilities and contact data.
// Explicit JSON serialization is only for the protected internal document.
func (BookingSnapshot) String() string         { return "[redacted booking notification]" }
func (s BookingSnapshot) GoString() string     { return s.String() }
func (s BookingSnapshot) LogValue() slog.Value { return slog.StringValue(s.String()) }

func (s BookingSnapshot) Valid() bool {
	if !boundedText(s.ShopName, 200) || !boundedText(s.ShopAddress, 1000) ||
		!boundedText(s.CustomerName, 400) || !boundedText(s.ProfessionalName, 200) ||
		!boundedText(s.ServiceName, 200) || !validSlug(s.ShopSlug) ||
		!canonicalPhone(s.RecipientPhone) || !boundedText(s.AppointmentCode, 80) ||
		strings.IndexFunc(s.AppointmentCode, unicode.IsSpace) >= 0 || s.PriceCents < 0 ||
		len(s.Currency) != 3 || strings.Trim(s.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" ||
		!validTime(s.StartsAt) || !validTime(s.EndsAt) || !s.EndsAt.After(s.StartsAt) ||
		!boundedText(s.ShopTimezone, 100) || s.ShopTimezone == "Local" {
		return false
	}
	_, err := time.LoadLocation(s.ShopTimezone)
	return err == nil
}

// PayloadScope is authenticated alongside the ciphertext. A payload cannot be
// moved to another tenant, appointment, schedule generation or retention period.
// Expiry must have PostgreSQL's microsecond precision before authentication.
type PayloadScope struct {
	ShopID             uuid.UUID
	AppointmentID      uuid.UUID
	ScheduleGeneration int64
	ExpiresAt          time.Time
}

func (s PayloadScope) Valid() bool {
	return s.ShopID != uuid.Nil && s.AppointmentID != uuid.Nil && s.ScheduleGeneration > 0 &&
		validTime(s.ExpiresAt) && s.ExpiresAt.Nanosecond()%1000 == 0
}

// ProtectedPayload contains no plaintext. The protocol version and key ID are
// authenticated too; key material never belongs in this DTO or database record.
type ProtectedPayload struct {
	Version    int
	KeyID      string
	Ciphertext []byte
}

type PrepareBookingNotificationsInput struct {
	ShopID             uuid.UUID
	AppointmentID      uuid.UUID
	ScheduleGeneration int64
	CreatedAt          time.Time
	Policy             domain.Policy
	// Derived from the recorded transactional permission, not assumed from phone.
	PermissionGranted bool
	Snapshot          BookingSnapshot
}

// PreparedBookingNotifications must eventually be stored by the caller in the
// same transaction as the appointment. Preparing alone does not persist or send.
type PreparedBookingNotifications struct {
	Schedule domain.Schedule
	Scope    PayloadScope
	Payload  ProtectedPayload
}

func boundedText(s string, max int) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && strings.TrimSpace(s) == s &&
		strings.IndexFunc(s, unicode.IsControl) < 0
}

func validSlug(s string) bool {
	return len(s) >= 3 && len(s) <= 100 && s[0] != '-' && s[len(s)-1] != '-' && !strings.Contains(s, "--") &&
		strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789-") == ""
}

func canonicalPhone(s string) bool {
	// Structural E.164 check only; do not duplicate Customers' numbering metadata.
	return len(s) >= 3 && len(s) <= 16 && s[0] == '+' && s[1] >= '1' && s[1] <= '9' &&
		strings.Trim(s[2:], "0123456789") == ""
}

func validTime(t time.Time) bool {
	return !t.IsZero() && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
}
