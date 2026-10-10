package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const AgendaPageSize = 100

// ShopID is supplied by authenticated shop authorization, never query parameters.
type ListAgendaInput struct {
	ShopID uuid.UUID
	// Set by the HTTP adapter from authorization, not supplied by the browser.
	// Nil means the owner can read the entire shop agenda.
	ProfessionalID uuid.UUID
	Month          string
	After          *AgendaCursor
}

// A cursor is only a pagination position, not an authorization credential.
type AgendaCursor struct {
	StartsAt time.Time
	ID       uuid.UUID
}

// AdminAppointment contains private contact data. Do not reuse it in public APIs.
// Booking terms remain snapshots; customer contact is the current local profile.
type AdminAppointment struct {
	Appointment   AppointmentReceipt
	CustomerName  string
	CustomerPhone string
}

type AgendaOutput struct {
	Month        string
	Today        string
	FromDate     string
	UntilDate    string
	ShopTimezone string
	Appointments []AdminAppointment
	Next         *AgendaCursor
}

type AgendaQuery struct {
	ShopID         uuid.UUID
	ProfessionalID uuid.UUID
	From           time.Time
	Until          time.Time
	After          *AgendaCursor
	Limit          int
}

type ListAgenda struct {
	shops        AgendaShopReader
	appointments AgendaReader
	clock        Clock
}

func NewListAgenda(shops AgendaShopReader, appointments AgendaReader, clock Clock) *ListAgenda {
	return &ListAgenda{shops: shops, appointments: appointments, clock: clock}
}

func (uc *ListAgenda) Execute(ctx context.Context, input ListAgendaInput) (AgendaOutput, error) {
	if input.ShopID == uuid.Nil {
		return AgendaOutput{}, ErrInvalidAgenda
	}
	shop, found, err := uc.shops.FindAgendaShop(ctx, input.ShopID)
	if err != nil {
		return AgendaOutput{}, fmt.Errorf("read agenda shop: %w", err)
	}
	if !found {
		return AgendaOutput{}, ErrShopNotFound
	}
	location, err := time.LoadLocation(shop.Timezone)
	if err != nil || shop.Timezone == "" || shop.ShopID != input.ShopID {
		return AgendaOutput{}, ErrInconsistentBookingData
	}
	now := uc.clock.Now().In(location)
	month := input.Month
	if month == "" {
		month = now.Format("2006-01")
	}
	first, err := time.ParseInLocation("2006-01", month, location)
	if err != nil || first.Format("2006-01") != month || first.Year() < 1900 || first.Year() > 9998 {
		return AgendaOutput{}, ErrInvalidAgenda
	}
	// Six complete Monday-first weeks match the panel's monthly calendar.
	from := first.AddDate(0, 0, -(int(first.Weekday())+6)%7)
	until := from.AddDate(0, 0, 42)
	if input.After != nil && (input.After.ID == uuid.Nil || input.After.StartsAt.Before(from) || !input.After.StartsAt.Before(until)) {
		return AgendaOutput{}, ErrInvalidAgenda
	}
	items, err := uc.appointments.ListAgendaAppointments(ctx, AgendaQuery{
		ShopID: input.ShopID, From: from.UTC(), Until: until.UTC(), After: input.After, Limit: AgendaPageSize + 1,
		ProfessionalID: input.ProfessionalID,
	})
	if err != nil {
		return AgendaOutput{}, fmt.Errorf("list agenda appointments: %w", err)
	}
	output := AgendaOutput{Month: month, Today: now.Format("2006-01-02"), FromDate: from.Format("2006-01-02"),
		UntilDate: until.Format("2006-01-02"), ShopTimezone: shop.Timezone, Appointments: make([]AdminAppointment, 0)}
	if len(items) > AgendaPageSize {
		items = items[:AgendaPageSize]
		last := items[len(items)-1].Appointment
		output.Next = &AgendaCursor{StartsAt: last.StartsAt, ID: last.AppointmentID}
	}
	for _, item := range items {
		item.Appointment.ShopName = shop.Name
		output.Appointments = append(output.Appointments, item)
	}
	return output, nil
}
