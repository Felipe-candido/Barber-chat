package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ListAvailability struct {
	transactions BookingTransactor
	clock        Clock
}

func NewListAvailability(tx BookingTransactor, clock Clock) *ListAvailability {
	return &ListAvailability{tx, clock}
}

func (uc *ListAvailability) Execute(ctx context.Context, input ListAvailabilityInput) (AvailabilityOutput, error) {
	return uc.execute(ctx, input, nil)
}

func (uc *ListAvailability) ExecuteForStaff(ctx context.Context, input ListAvailabilityInput, scope StaffBookingScope) (AvailabilityOutput, error) {
	if scope.UserID == uuid.Nil || scope.ShopID == uuid.Nil || (scope.ProfessionalID != uuid.Nil && scope.ProfessionalID != input.ProfessionalID) {
		return AvailabilityOutput{}, ErrStaffBookingAccessDenied
	}
	return uc.execute(ctx, input, &scope)
}

func (uc *ListAvailability) execute(ctx context.Context, input ListAvailabilityInput, staff *StaffBookingScope) (AvailabilityOutput, error) {
	input.ShopSlug = strings.TrimSpace(input.ShopSlug)
	if input.ShopSlug == "" || input.ServiceID == uuid.Nil || input.ProfessionalID == uuid.Nil {
		return AvailabilityOutput{}, ErrInvalidAvailability
	}
	output := AvailabilityOutput{Slots: make([]AvailabilitySlot, 0)}
	err := uc.transactions.WithinBookingTx(ctx, func(ctx context.Context, tx BookingTx) error {
		shop, err := resolveShop(ctx, tx.Shops, input.ShopSlug)
		if err != nil {
			return err
		}
		if staff != nil && shop.ShopID != staff.ShopID {
			return ErrStaffBookingAccessDenied
		}
		if !validBookingDate(input.LocalDate, shop.Timezone, uc.clock.Now()) {
			return ErrInvalidAvailability
		}
		// A short shared workflow serializes this observation with calendar writes;
		// the lock ends with this request and never reserves a slot for the browser.
		if err := tx.Schedule.LockProfessionalSchedule(ctx, shop.ShopID, input.ProfessionalID); err != nil {
			return err
		}
		if staff != nil {
			if err := requireStaffBookingRead(ctx, tx, *staff, input.ProfessionalID); err != nil {
				return err
			}
		}
		selection, err := findSelection(ctx, tx.Catalog, shop.ShopID, input.ServiceID, input.ProfessionalID)
		if err != nil {
			return err
		}
		output.ShopTimezone = shop.Timezone
		calendar, found, err := tx.Schedule.LoadCalendar(ctx, AvailabilityQuery{ShopID: shop.ShopID, ServiceID: input.ServiceID, ProfessionalID: input.ProfessionalID, LocalDate: input.LocalDate, ShopTimezone: shop.Timezone})
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		slots, err := calendar.AvailableSlots(input.LocalDate, shop.Timezone, selection.Service.DurationMinutes, uc.clock.Now())
		if err != nil {
			return err
		}
		for _, slot := range slots {
			output.Slots = append(output.Slots, AvailabilitySlot{StartsAt: slot.StartsAt, EndsAt: slot.EndsAt})
		}
		return nil
	})
	if err != nil {
		return AvailabilityOutput{}, err
	}
	return output, nil
}

type ListProfessionals struct {
	transactions BookingTransactor
	clock        Clock
}

func NewListProfessionals(tx BookingTransactor, clock Clock) *ListProfessionals {
	return &ListProfessionals{tx, clock}
}
func (uc *ListProfessionals) Execute(ctx context.Context, slug string, serviceID uuid.UUID) (ProfessionalsOutput, error) {
	return uc.execute(ctx, slug, serviceID, nil)
}

func (uc *ListProfessionals) ExecuteForStaff(ctx context.Context, slug string, serviceID uuid.UUID, scope StaffBookingScope) (ProfessionalsOutput, error) {
	if scope.UserID == uuid.Nil || scope.ShopID == uuid.Nil {
		return ProfessionalsOutput{}, ErrStaffBookingAccessDenied
	}
	return uc.execute(ctx, slug, serviceID, &scope)
}

func (uc *ListProfessionals) execute(ctx context.Context, slug string, serviceID uuid.UUID, staff *StaffBookingScope) (ProfessionalsOutput, error) {
	if serviceID == uuid.Nil {
		return ProfessionalsOutput{}, ErrInvalidAvailability
	}
	var output ProfessionalsOutput
	err := uc.transactions.WithinBookingTx(ctx, func(ctx context.Context, tx BookingTx) error {
		shop, err := resolveShop(ctx, tx.Shops, strings.TrimSpace(slug))
		if err != nil {
			return err
		}
		if staff != nil {
			if shop.ShopID != staff.ShopID {
				return ErrStaffBookingAccessDenied
			}
			if staff.ProfessionalID != uuid.Nil {
				if err := tx.Schedule.LockProfessionalSchedule(ctx, shop.ShopID, staff.ProfessionalID); err != nil {
					return err
				}
			}
			if err := requireStaffBookingRead(ctx, tx, *staff, staff.ProfessionalID); err != nil {
				return err
			}
		}
		items, err := tx.Catalog.ListBookableProfessionals(ctx, shop.ShopID, serviceID)
		if err != nil {
			return err
		}
		if items == nil {
			items = make([]Professional, 0)
		}
		seen := make(map[uuid.UUID]bool)
		for _, item := range items {
			if item.ID == uuid.Nil || strings.TrimSpace(item.Name) == "" || seen[item.ID] {
				return ErrInconsistentBookingData
			}
			seen[item.ID] = true
		}
		if staff != nil && staff.ProfessionalID != uuid.Nil {
			filtered := make([]Professional, 0, 1)
			for _, item := range items {
				if item.ID == staff.ProfessionalID {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		location, _ := time.LoadLocation(shop.Timezone)
		now := uc.clock.Now().In(location)
		output = ProfessionalsOutput{ShopName: shop.Name, ShopTimezone: shop.Timezone, Today: now.Format("2006-01-02"),
			LastBookingDate: now.AddDate(0, 0, BookingHorizonDays).Format("2006-01-02"), Professionals: items}
		return nil
	})
	if err != nil {
		return ProfessionalsOutput{}, err
	}
	return output, nil
}

func requireStaffBookingRead(ctx context.Context, tx BookingTx, scope StaffBookingScope, professionalID uuid.UUID) error {
	if tx.Staff == nil {
		return ErrStaffBookingAccessDenied
	}
	allowed, err := tx.Staff.CanCreateBooking(ctx, scope.UserID, scope.ShopID, professionalID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrStaffBookingAccessDenied
	}
	return nil
}
