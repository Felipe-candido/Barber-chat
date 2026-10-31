package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

const BookingHorizonDays = 90
const IdempotencyRetention = 24 * time.Hour

var ErrStaffBookingAccessDenied = errors.New("staff booking access denied")

type CreateAppointment struct {
	transactions BookingTransactor
	contacts     CustomerNormalizer
	codes        AppointmentCodes
	clock        Clock
}

func NewCreateAppointment(tx BookingTransactor, contacts CustomerNormalizer, codes AppointmentCodes, clock Clock) *CreateAppointment {
	return &CreateAppointment{transactions: tx, contacts: contacts, codes: codes, clock: clock}
}

func (uc *CreateAppointment) Execute(ctx context.Context, input CreateAppointmentInput) (CreateAppointmentOutput, error) {
	return uc.execute(ctx, input, nil)
}

func (uc *CreateAppointment) ExecuteForStaff(ctx context.Context, input CreateAppointmentInput, userID, shopID, allowedProfessionalID uuid.UUID) (CreateAppointmentOutput, error) {
	if userID == uuid.Nil || shopID == uuid.Nil || (allowedProfessionalID != uuid.Nil && input.ProfessionalID != allowedProfessionalID) {
		return CreateAppointmentOutput{}, ErrStaffBookingAccessDenied
	}
	return uc.execute(ctx, input, &StaffBookingScope{userID, shopID, allowedProfessionalID})
}

func (uc *CreateAppointment) execute(ctx context.Context, input CreateAppointmentInput, staff *StaffBookingScope) (CreateAppointmentOutput, error) {
	input.ShopSlug = strings.TrimSpace(input.ShopSlug)
	input.StartsAt = input.StartsAt.UTC()
	key, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(key) != 32 || input.IdempotencyKey != strings.ToLower(input.IdempotencyKey) ||
		input.ShopSlug == "" || input.ServiceID == uuid.Nil || input.ProfessionalID == uuid.Nil ||
		input.StartsAt.IsZero() || input.StartsAt.Second() != 0 || input.StartsAt.Nanosecond() != 0 {
		return CreateAppointmentOutput{}, ErrInvalidAppointment
	}
	input.Customer, err = uc.contacts.Normalize(input.Customer)
	if err != nil {
		return CreateAppointmentOutput{}, err
	}
	// A keyed fingerprint also prevents offline guessing of contact data from a
	// database-only leak. The random request key is never persisted or logged.
	canonical := input
	canonical.IdempotencyKey = ""
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return CreateAppointmentOutput{}, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	var fingerprint [32]byte
	copy(fingerprint[:], mac.Sum(nil))
	var output CreateAppointmentOutput
	err = uc.transactions.WithinBookingTx(ctx, func(ctx context.Context, tx BookingTx) error {
		shop, err := resolveShop(ctx, tx.Shops, input.ShopSlug)
		if err != nil {
			return err
		}
		if staff != nil && shop.ShopID != staff.ShopID {
			return ErrStaffBookingAccessDenied
		}
		now := uc.clock.Now().UTC()
		previous, replayed, err := tx.Idempotency.ClaimCreateAppointment(ctx, IdempotencyClaim{
			ShopID: shop.ShopID, Key: input.IdempotencyKey, RequestFingerprint: fingerprint, RequestedAt: now, ExpiresAt: now.Add(IdempotencyRetention)})
		if err != nil {
			return fmt.Errorf("claim booking request: %w", err)
		}
		if staff != nil {
			if err := tx.Schedule.LockProfessionalSchedule(ctx, shop.ShopID, input.ProfessionalID); err != nil {
				return err
			}
			if tx.Staff == nil {
				return ErrStaffBookingAccessDenied
			}
			allowed, err := tx.Staff.CanCreateBooking(ctx, staff.UserID, shop.ShopID, input.ProfessionalID)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrStaffBookingAccessDenied
			}
		}
		if replayed {
			output = CreateAppointmentOutput{Appointment: previous.Appointment, Code: previous.Code, Replayed: true}
			return nil
		}
		if staff != nil && shop.SelfService {
			if tx.StaffEligibility == nil {
				return ErrBookingEligibilityUnavailable
			}
			allowed, err := tx.StaffEligibility.CanCreateStaffBooking(ctx, shop.ShopID)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrBookingEligibilityUnavailable, err)
			}
			if !allowed {
				return ErrBookingSubscriptionRequired
			}
		}
		if staff == nil && shop.SelfService {
			if tx.PublicAccess == nil {
				return ErrShopNotFound
			}
			allowed, err := tx.PublicAccess.CanCreatePublicBooking(ctx, shop.Slug)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrShopNotFound
			}
		}
		if staff == nil {
			if err := tx.Schedule.LockProfessionalSchedule(ctx, shop.ShopID, input.ProfessionalID); err != nil {
				return err
			}
		}
		selection, err := findSelection(ctx, tx.Catalog, shop.ShopID, input.ServiceID, input.ProfessionalID)
		if err != nil {
			return err
		}
		location, _ := time.LoadLocation(shop.Timezone)
		date := input.StartsAt.In(location).Format("2006-01-02")
		now = uc.clock.Now().UTC()
		if !validBookingDate(date, shop.Timezone, now) || !input.StartsAt.After(now) {
			return ErrInvalidAppointment
		}
		calendar, found, err := tx.Schedule.LoadCalendar(ctx, AvailabilityQuery{ShopID: shop.ShopID, ServiceID: input.ServiceID, ProfessionalID: input.ProfessionalID, LocalDate: date, ShopTimezone: shop.Timezone})
		if err != nil {
			return err
		}
		if !found {
			return ErrSlotUnavailable
		}
		slots, err := calendar.AvailableSlots(date, shop.Timezone, selection.Service.DurationMinutes, now)
		if err != nil {
			return fmt.Errorf("calculate booking interval: %w", err)
		}
		var end time.Time
		for _, slot := range slots {
			if slot.StartsAt.Equal(input.StartsAt) {
				end = slot.EndsAt
				break
			}
		}
		if end.IsZero() {
			return ErrSlotUnavailable
		}
		customer, err := tx.Customers.ResolveForBooking(ctx, CustomerResolutionInput{ShopID: shop.ShopID, Customer: input.Customer, RecordedAt: now})
		if err != nil {
			return err
		}
		if customer.CustomerID == uuid.Nil {
			return ErrInconsistentBookingData
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		appointment, err := domain.NewAppointment(domain.Appointment{ID: id, ShopID: shop.ShopID, CustomerID: customer.CustomerID,
			ServiceID: input.ServiceID, ProfessionalID: input.ProfessionalID, Service: selection.Service, ProfessionalName: selection.ProfessionalName,
			ShopTimezone: shop.Timezone, StartsAt: input.StartsAt, EndsAt: end}, uc.clock.Now().UTC())
		if err != nil {
			return err
		}
		code, digest, err := uc.codes.New()
		if err != nil {
			return err
		}
		saved, err := tx.Appointments.Insert(ctx, appointment, digest)
		if err != nil {
			return fmt.Errorf("insert appointment: %w", err)
		}
		if saved != appointment {
			return ErrInconsistentBookingData
		}
		output = CreateAppointmentOutput{Appointment: receiptOf(appointment, shop.Name), Code: code}
		return tx.Idempotency.CompleteCreateAppointment(ctx, shop.ShopID, input.IdempotencyKey, ClaimedReceipt{Appointment: output.Appointment, Code: code})
	})
	if err != nil {
		return CreateAppointmentOutput{}, err
	}
	return output, nil
}

func resolveShop(ctx context.Context, resolver ShopResolver, slug string) (ShopContext, error) {
	shop, found, err := resolver.ResolveActiveShop(ctx, slug)
	if err != nil {
		return ShopContext{}, err
	}
	if !found {
		return ShopContext{}, ErrShopNotFound
	}
	if shop.ShopID == uuid.Nil || shop.Slug != slug || strings.TrimSpace(shop.Name) == "" || shop.Timezone == "" {
		return ShopContext{}, ErrInconsistentBookingData
	}
	if _, err := time.LoadLocation(shop.Timezone); err != nil {
		return ShopContext{}, ErrInconsistentBookingData
	}
	return shop, nil
}

func findSelection(ctx context.Context, reader BookableCatalogReader, shopID, serviceID, professionalID uuid.UUID) (BookableSelection, error) {
	s, found, err := reader.FindBookableSelection(ctx, shopID, serviceID, professionalID)
	if err != nil {
		return BookableSelection{}, err
	}
	if !found {
		return BookableSelection{}, ErrBookingOptionNotFound
	}
	if s.ShopID != shopID || s.ServiceID != serviceID || s.ProfessionalID != professionalID || strings.TrimSpace(s.ProfessionalName) == "" ||
		strings.TrimSpace(s.Service.Name) == "" || s.Service.DurationMinutes < 1 || s.Service.DurationMinutes > 1440 ||
		s.Service.Currency != "BRL" || s.Service.PriceCents < 0 || s.Service.PriceCents > 9007199254740991 {
		return BookableSelection{}, ErrInconsistentBookingData
	}
	return s, nil
}

func validBookingDate(date, timezone string, now time.Time) bool {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return false
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false
	}
	local := now.In(location)
	return date >= local.Format("2006-01-02") && date <= local.AddDate(0, 0, BookingHorizonDays).Format("2006-01-02")
}

func receiptOf(a domain.Appointment, shopName string) AppointmentReceipt {
	return AppointmentReceipt{ShopName: shopName, AppointmentID: a.ID, Status: a.Status, Service: a.Service, ProfessionalID: a.ProfessionalID,
		ProfessionalName: a.ProfessionalName, StartsAt: a.StartsAt, EndsAt: a.EndsAt, ShopTimezone: a.ShopTimezone}
}
