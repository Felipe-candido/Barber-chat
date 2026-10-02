package application

import (
	"context"
	"fmt"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

// ListAuthorizedServices reads an already authorized shop without resolving its
// slug again. ShopID must come from successful authorization, never client input.
type ListAuthorizedServices struct {
	repository ServiceRepository
}

func NewListAuthorizedServices(repository ServiceRepository) *ListAuthorizedServices {
	return &ListAuthorizedServices{repository: repository}
}

func (uc *ListAuthorizedServices) Execute(ctx context.Context, shopID uuid.UUID) ([]ServiceOutput, error) {
	if shopID == uuid.Nil {
		return nil, domain.ErrInvalidShopID
	}
	services, err := uc.repository.ListActiveByShop(ctx, shopID)
	if err != nil {
		return nil, fmt.Errorf("list authorized services: %w", err)
	}
	items := make([]ServiceOutput, 0, len(services))
	for _, service := range services {
		items = append(items, serviceOutput(service))
	}
	return items, nil
}
