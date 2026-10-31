package application

import (
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

// Internal authority comes from authenticated context, never request JSON.
type ProductScope struct{ ActorID, ShopID uuid.UUID }

const (
	DefaultProductPageSize = 25
	MaxProductPageSize     = 100
)

type CreateProductInput struct {
	Scope                                        ProductScope
	Name, Description                            string
	PriceCents, InitialQuantity, MinimumQuantity int64
	IdempotencyKey                               string
}

type UpdateProductInput struct {
	Scope           ProductScope
	ProductID       uuid.UUID
	Details         domain.ProductDetails
	ExpectedVersion int64
}

type ManualStockOperation string

const (
	StockReceive ManualStockOperation = "receipt"
	StockAdjust  ManualStockOperation = "adjustment"
)

// Signed delta: receipt > 0; adjustment != 0. Sale/reversal are server-only.
type RecordStockInput struct {
	Scope                  ProductScope
	ProductID              uuid.UUID
	Operation              ManualStockOperation
	QuantityDelta          int64
	Reason, IdempotencyKey string
}

type UpdateStockMinimumInput struct {
	Scope                                 ProductScope
	ProductID                             uuid.UUID
	MinimumQuantity, ExpectedStockVersion int64
}

type ProductOutput struct {
	ID, ShopID                                    uuid.UUID
	Name, Description, Currency                   string
	PriceCents, Version                           int64
	Active                                        bool
	QuantityOnHand, MinimumQuantity, StockVersion int64
	CreatedAt, UpdatedAt                          time.Time
}

type ProductCursor struct {
	CreatedAt time.Time
	ProductID uuid.UUID
}

type ProductListInput struct {
	Scope           ProductScope
	Search          string
	IncludeInactive bool // Owner only; staff sees active products.
	After           *ProductCursor
	PageSize        int
}

type ProductQuery struct {
	ShopID          uuid.UUID
	Search          string
	IncludeInactive bool
	After           *ProductCursor
	Limit           int // Bounded lookahead; filtering precedes pagination.
}

type ProductPage struct {
	Items []ProductOutput
	Next  *ProductCursor
}

type StockMovementCursor struct {
	CreatedAt  time.Time
	MovementID uuid.UUID
}

type StockMovementQuery struct {
	ShopID, ProductID uuid.UUID
	After             *StockMovementCursor
	Limit             int
}

type ProductCommand string

const (
	ProductCreateCommand ProductCommand = "create_product"
	StockRecordCommand   ProductCommand = "record_stock"
)

type ProductCommandKey struct {
	Operation           ProductCommand
	Digest, Fingerprint [32]byte
}

type ProductCommandResult struct{ ProductID, MovementID uuid.UUID }

type ProductMutationOutput struct {
	Product    ProductOutput
	MovementID uuid.UUID
	Replayed   bool
}

type StockMovementListInput struct {
	Scope     ProductScope
	ProductID uuid.UUID
	After     *StockMovementCursor
	PageSize  int
}

type StockMovementPage struct {
	Items []domain.StockMovement
	Next  *StockMovementCursor
}
