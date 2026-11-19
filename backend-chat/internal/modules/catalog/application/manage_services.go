package application

import (
	"context"
	"errors"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidServiceID = errors.New("service ID is required")
	ErrServiceNotFound  = errors.New("service not found in the authorized shop")
)

type ServiceManagementRepository interface {
	Update(context.Context, domain.Service) (domain.Service, error)
	Deactivate(context.Context, uuid.UUID, uuid.UUID) error
}

type UpdateServiceInput struct {
	// ShopID comes from authorization; ServiceID identifies a resource within it.
	ShopID, ServiceID uuid.UUID
	Name, Description string
	DurationMinutes   int
	PriceCents        int64
}

type ManageServices struct{ repository ServiceManagementRepository }

func NewManageServices(repository ServiceManagementRepository) *ManageServices {
	return &ManageServices{repository: repository}
}

func (uc *ManageServices) Update(ctx context.Context, input UpdateServiceInput) (ServiceOutput, error) {
	if input.ServiceID == uuid.Nil {
		return ServiceOutput{}, ErrInvalidServiceID
	}
	service, err := domain.NewService(input.ShopID, input.Name, input.Description, input.DurationMinutes, input.PriceCents)
	if err != nil {
		return ServiceOutput{}, err
	}
	service.ID = input.ServiceID
	saved, err := uc.repository.Update(ctx, service)
	if err != nil {
		return ServiceOutput{}, err
	}
	return serviceOutput(saved), nil
}

// Delete removes the service from new bookings without deleting appointment history.
func (uc *ManageServices) Delete(ctx context.Context, shopID, serviceID uuid.UUID) error {
	if shopID == uuid.Nil {
		return domain.ErrInvalidShopID
	}
	if serviceID == uuid.Nil {
		return ErrInvalidServiceID
	}
	return uc.repository.Deactivate(ctx, shopID, serviceID)
}
