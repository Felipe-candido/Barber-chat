package application

import (
	"context"
	"errors"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

var (
	ErrProductAccessDenied      = errors.New("product access denied")
	ErrProductNotFound          = errors.New("product not found in the authorized shop")
	ErrProductCommandConflict   = errors.New("product command key was used with another input")
	ErrProductCommandPending    = errors.New("product command is still pending")
	ErrInvalidProductQuery      = errors.New("invalid product query")
	ErrInvalidProductCommandKey = errors.New("invalid product idempotency key")
	ErrInconsistentProductData  = errors.New("inconsistent product data")
)

// Revalidate active local user, shop and membership on the shared transaction.
type ProductAccess interface {
	RequireProductOwner(context.Context, ProductScope) error
	RequireProductStaff(context.Context, ProductScope) error
}

type ProductRepository interface {
	InsertProduct(context.Context, domain.Product) error
	LockProduct(ctx context.Context, shopID, productID uuid.UUID) (domain.Product, bool, error)
	UpdateProduct(ctx context.Context, product domain.Product, expectedVersion int64) (bool, error)
}

type ProductReader interface {
	FindProductOutput(ctx context.Context, shopID, productID uuid.UUID) (ProductOutput, bool, error)
	ListProducts(context.Context, ProductQuery) ([]ProductOutput, error)
	// Owner only: expose reasons/actors on the authorized product's history.
	ListStockMovements(context.Context, StockMovementQuery) ([]domain.StockMovement, error)
}

type ProductStockStore interface {
	InsertStockBalance(context.Context, domain.StockBalance) error
	LockStockBalance(ctx context.Context, shopID, productID uuid.UUID) (domain.StockBalance, bool, error)
	// AFTER INSERT applies the delta atomically; callers must not update quantity separately.
	AppendStockMovement(context.Context, domain.StockMovement) error
	UpdateStockMinimum(ctx context.Context, shopID, productID uuid.UUID, minimumQuantity, expectedVersion int64) (bool, error)
}

type ProductCommandStore interface {
	// Claims are bound to shop + actor + operation + digest. The bool means a
	// completed replay; false means this transaction owns a new claim. Conflicting
	// fingerprints/pending claims return typed errors, never a new mutation.
	ClaimProductCommand(context.Context, ProductScope, ProductCommandKey) (ProductCommandResult, bool, error)
	CompleteProductCommand(context.Context, ProductScope, ProductCommandKey, ProductCommandResult) error
}

type ProductTx struct {
	Access   ProductAccess
	Products ProductRepository
	Stock    ProductStockStore
	Commands ProductCommandStore
	Reader   ProductReader
}

type ProductTransactor interface {
	WithinProductTx(context.Context, func(context.Context, ProductTx) error) error
}
