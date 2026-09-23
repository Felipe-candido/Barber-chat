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
