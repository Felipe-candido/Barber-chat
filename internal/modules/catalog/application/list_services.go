package application

import "context"

type ListServices struct {
	repository ServiceRepository
	shops      ShopResolver
}

func NewListServices(repository ServiceRepository, shops ShopResolver) *ListServices {
	return &ListServices{repository: repository, shops: shops}
}
func (uc *ListServices) Execute(ctx context.Context, slug string) ([]ServiceOutput, error) {
	shopID, found, err := uc.shops.LookupActiveShop(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrShopNotFound
	}
	services, err := uc.repository.ListActiveByShop(ctx, shopID)
	if err != nil {
		return nil, err
	}
	output := make([]ServiceOutput, 0, len(services))
	for _, s := range services {
		output = append(output, serviceOutput(s))
	}
	return output, nil
}
