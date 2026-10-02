package application

import (
	"context"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

type CreateServiceInput struct {
	// ShopID must come from successful authorization, never from request JSON.
	ShopID          uuid.UUID
	Name            string
	Description     string
	DurationMinutes int
	PriceCents      int64
}
type CreateService struct {
	repository ServiceRepository
}

func NewCreateService(repository ServiceRepository) *CreateService {
	return &CreateService{repository: repository}
}

func (uc *CreateService) Execute(ctx context.Context, input CreateServiceInput) (ServiceOutput, error) {
	service, err := domain.NewService(input.ShopID, input.Name, input.Description, input.DurationMinutes, input.PriceCents)
	if err != nil {
		return ServiceOutput{}, err
	}
	created, err := uc.repository.Create(ctx, service)
	if err != nil {
		return ServiceOutput{}, err
	}
	return serviceOutput(created), nil
}
