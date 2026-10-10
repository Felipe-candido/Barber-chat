package application

import (
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

// ServiceOutput exposes catalog data without persistence types or tenant internals.
type ServiceOutput struct {
	ID              uuid.UUID
	Name            string
	Description     string
	DurationMinutes int
	PriceCents      int64
	Currency        string
	Active          bool
}

func serviceOutput(s domain.Service) ServiceOutput {
	return ServiceOutput{ID: s.ID, Name: s.Name, Description: s.Description, DurationMinutes: s.DurationMinutes,
		PriceCents: s.PriceCents, Currency: s.Currency, Active: s.Active}
}

// InitialSchedule is input to the booking-owned calendar setup port, not a
// catalog-owned calendar entity. This onboarding workflow creates both atomically.
type InitialSchedule struct {
	ShopID, ProfessionalID         uuid.UUID
	IntervalMinutes, BufferMinutes int
	Weekdays                       []int
	OpensAtMinute, ClosesAtMinute  int
}

type CreateProfessionalInput struct {
	ShopID     uuid.UUID
	Name       string
	ServiceIDs []uuid.UUID
	Schedule   InitialSchedule
}

type ProfessionalOutput struct {
	ID         uuid.UUID
	Name       string
	Active     bool
	ServiceIDs []uuid.UUID
}
