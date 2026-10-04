package application

import (
	"context"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

var ErrShopNotFound = errors.New("active shop not found")

type ServiceRepository interface {
	Create(context.Context, domain.Service) (domain.Service, error)
	ListActiveByShop(context.Context, uuid.UUID) ([]domain.Service, error)
}

// ShopResolver resolves an active tenant; it does not authorize administrative access.
type ShopResolver interface {
	LookupActiveShop(context.Context, string) (uuid.UUID, bool, error)
}

var (
	ErrInvalidProfessionalServices = errors.New("choose active services from the authorized shop")
	ErrInvalidProfessionalSchedule = errors.New("invalid initial professional schedule")
)

type ProfessionalRepository interface {
	CreateProfessional(context.Context, domain.Professional, []uuid.UUID) error
	ListProfessionals(context.Context, uuid.UUID) ([]ProfessionalOutput, error)
}
type InitialScheduleWriter interface {
	ConfigureInitialSchedule(context.Context, InitialSchedule) error
}
type ProfessionalTx struct {
	Professionals ProfessionalRepository
	Schedule      InitialScheduleWriter
}
type ProfessionalTransactor interface {
	WithinProfessionalTx(context.Context, func(context.Context, ProfessionalTx) error) error
}
