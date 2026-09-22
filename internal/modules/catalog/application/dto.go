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
