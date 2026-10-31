package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxStockQuantity int64 = math.MaxInt32

var (
	ErrInvalidStock         = errors.New("invalid stock balance or movement")
	ErrInsufficientStock    = errors.New("insufficient product stock")
	ErrStockLimitExceeded   = errors.New("stock quantity exceeds its limit")
	ErrStockVersionConflict = errors.New("stock version changed")
)

type StockMovementKind string

const (
	StockOpening      StockMovementKind = "opening"
	StockReceipt      StockMovementKind = "receipt"
	StockAdjustment   StockMovementKind = "adjustment"
	StockSale         StockMovementKind = "sale"
	StockSaleReversal StockMovementKind = "sale_reversal"
)

type StockMovement struct {
	ID, ShopID, ProductID, ActorID uuid.UUID
	Kind                           StockMovementKind
	QuantityDelta                  int64
	Reason                         string
	SaleItemID, ReversesMovementID uuid.UUID
	CreatedAt                      time.Time
}

// Origin ownership and reversal uniqueness additionally require SQL FKs/uniques.
func NewStockMovement(m StockMovement, now time.Time) (StockMovement, error) {
	m.Reason = strings.TrimSpace(m.Reason)
	if m.ID == uuid.Nil || m.ShopID == uuid.Nil || m.ProductID == uuid.Nil || m.ActorID == uuid.Nil || now.IsZero() ||
		m.QuantityDelta == 0 || m.QuantityDelta < -MaxStockQuantity || m.QuantityDelta > MaxStockQuantity ||
		!utf8.ValidString(m.Reason) || utf8.RuneCountInString(m.Reason) < 1 || utf8.RuneCountInString(m.Reason) > 500 || strings.ContainsFunc(m.Reason, unicode.IsControl) {
		return StockMovement{}, ErrInvalidStock
	}
	switch m.Kind {
	case StockOpening, StockReceipt:
		if m.QuantityDelta < 0 || m.SaleItemID != uuid.Nil || m.ReversesMovementID != uuid.Nil {
			return StockMovement{}, ErrInvalidStock
		}
	case StockAdjustment:
		if m.SaleItemID != uuid.Nil || m.ReversesMovementID != uuid.Nil {
			return StockMovement{}, ErrInvalidStock
		}
	case StockSale:
		if m.QuantityDelta > 0 || m.SaleItemID == uuid.Nil || m.ReversesMovementID != uuid.Nil {
			return StockMovement{}, ErrInvalidStock
		}
	case StockSaleReversal:
		if m.QuantityDelta < 0 || m.SaleItemID == uuid.Nil || m.ReversesMovementID == uuid.Nil || m.ReversesMovementID == m.ID {
			return StockMovement{}, ErrInvalidStock
		}
	default:
		return StockMovement{}, ErrInvalidStock
	}
	m.CreatedAt = now.UTC()
	return m, nil
}

// Reversal copies the original item/tenant/quantity, never a browser's quantity.
func ReverseSaleStock(id uuid.UUID, original StockMovement, actorID uuid.UUID, reason string, now time.Time) (StockMovement, error) {
	validated, err := NewStockMovement(original, original.CreatedAt)
	if err != nil || validated.Kind != StockSale || now.Before(original.CreatedAt) {
		return StockMovement{}, ErrInvalidStock
	}
	return NewStockMovement(StockMovement{ID: id, ShopID: original.ShopID, ProductID: original.ProductID, ActorID: actorID,
		Kind: StockSaleReversal, QuantityDelta: -original.QuantityDelta, Reason: reason, SaleItemID: original.SaleItemID, ReversesMovementID: original.ID}, now)
}

type StockBalance struct {
	ShopID, ProductID                        uuid.UUID
	QuantityOnHand, MinimumQuantity, Version int64
}

func NewStockBalance(shopID, productID uuid.UUID, minimumQuantity int64) (StockBalance, error) {
	b := StockBalance{ShopID: shopID, ProductID: productID, MinimumQuantity: minimumQuantity, Version: 1}
	if !b.valid() {
		return StockBalance{}, ErrInvalidStock
	}
	return b, nil
}

func (b StockBalance) valid() bool {
	return b.ShopID != uuid.Nil && b.ProductID != uuid.Nil && b.QuantityOnHand >= 0 && b.QuantityOnHand <= MaxStockQuantity &&
		b.MinimumQuantity >= 0 && b.MinimumQuantity <= MaxStockQuantity && b.Version > 0
}

func (b StockBalance) LowStock() bool { return b.valid() && b.QuantityOnHand <= b.MinimumQuantity }

// Changes the alert threshold without overwriting quantity or movement history.
func (b StockBalance) SetMinimum(minimum, expectedVersion int64) (StockBalance, error) {
	if !b.valid() || minimum < 0 || minimum > MaxStockQuantity {
		return StockBalance{}, ErrInvalidStock
	}
	if expectedVersion < 1 || b.Version != expectedVersion || b.Version == math.MaxInt64 {
		return StockBalance{}, ErrStockVersionConflict
	}
	if minimum == b.MinimumQuantity {
		return b, nil
	}
	b.MinimumQuantity, b.Version = minimum, b.Version+1
	return b, nil
}

// Pure arithmetic only. The future SQL transaction/trigger must enforce locking,
// origin deduplication and append-only movements; this method cannot prove those.
func (b StockBalance) Apply(m StockMovement) (StockBalance, error) {
	if !b.valid() || b.Version == math.MaxInt64 {
		return StockBalance{}, ErrInvalidStock
	}
	if _, err := NewStockMovement(m, m.CreatedAt); err != nil || m.ShopID != b.ShopID || m.ProductID != b.ProductID {
		return StockBalance{}, ErrInvalidStock
	}
	if m.Kind == StockOpening && (b.Version != 1 || b.QuantityOnHand != 0) {
		return StockBalance{}, ErrInvalidStock
	}
	next := b.QuantityOnHand + m.QuantityDelta // Both operands are bounded int32 magnitudes.
	if next < 0 {
		return StockBalance{}, ErrInsufficientStock
	}
	if next > MaxStockQuantity {
		return StockBalance{}, ErrStockLimitExceeded
	}
	b.QuantityOnHand, b.Version = next, b.Version+1
	return b, nil
}
