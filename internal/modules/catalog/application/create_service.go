package application

import (
	"context"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
)

type CreateServiceInput struct {
	// ShopSlug must come from a trusted administrative scope, not the request body.
	ShopSlug        string
	Name            string
	Description     string
	DurationMinutes int
	PriceCents      int64
}
type CreateService struct {
	repository ServiceRepository
	shops      ShopResolver
}

func NewCreateService(repository ServiceRepository, shops ShopResolver) *CreateService {
	return &CreateService{repository: repository, shops: shops}
}
func (uc *CreateService) Execute(ctx context.Context, input CreateServiceInput) (ServiceOutput, error) {
	shopID, found, err := uc.shops.LookupActiveShop(ctx, input.ShopSlug)
	if err != nil {
		return ServiceOutput{}, err
	}
	if !found {
		return ServiceOutput{}, ErrShopNotFound
	}
	service, err := domain.NewService(shopID, input.Name, input.Description, input.DurationMinutes, input.PriceCents)
	if err != nil {
		return ServiceOutput{}, err
	}
	created, err := uc.repository.Create(ctx, service)
	if err != nil {
		return ServiceOutput{}, err
	}
	return serviceOutput(created), nil
}
