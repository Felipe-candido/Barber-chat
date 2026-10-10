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
		if !validBookingDate(input.LocalDate, shop.Timezone, uc.clock.Now()) {
			return ErrInvalidAvailability
		}
		// A short shared workflow serializes this observation with calendar writes;
		// the lock ends with this request and never reserves a slot for the browser.
		if err := tx.Schedule.LockProfessionalSchedule(ctx, shop.ShopID, input.ProfessionalID); err != nil {
			return err
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
	if serviceID == uuid.Nil {
		return ProfessionalsOutput{}, ErrInvalidAvailability
	}
	var output ProfessionalsOutput
	err := uc.transactions.WithinBookingTx(ctx, func(ctx context.Context, tx BookingTx) error {
		shop, err := resolveShop(ctx, tx.Shops, strings.TrimSpace(slug))
		if err != nil {
			return err
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
