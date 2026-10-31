package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

func TestStockArithmeticTenantAndExactSaleReversal(t *testing.T) {
	shop, product, actor, item := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	balance, err := domain.NewStockBalance(shop, product, 1)
	if err != nil || balance.QuantityOnHand != 0 || !balance.LowStock() {
		t.Fatal(err)
	}
	opening, err := domain.NewStockMovement(domain.StockMovement{ID: uuid.New(), ShopID: shop, ProductID: product, ActorID: actor, Kind: domain.StockOpening, QuantityDelta: 2, Reason: "Initial count"}, now)
	if err != nil {
		t.Fatal(err)
	}
	balance, err = balance.Apply(opening)
	if err != nil || balance.QuantityOnHand != 2 || balance.LowStock() {
		t.Fatal(err)
	}
	if _, err := balance.Apply(opening); !errors.Is(err, domain.ErrInvalidStock) {
		t.Fatal("second opening accepted", err)
	}
	sale, err := domain.NewStockMovement(domain.StockMovement{ID: uuid.New(), ShopID: shop, ProductID: product, ActorID: actor, Kind: domain.StockSale, QuantityDelta: -2, Reason: "Product sale", SaleItemID: item}, now)
	if err != nil {
		t.Fatal(err)
	}
	remaining, err := balance.Apply(sale)
	if err != nil || remaining.QuantityOnHand != 0 || balance.QuantityOnHand != 2 {
		t.Fatal("incorrect sale arithmetic", err)
	}
	if _, err := remaining.Apply(sale); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatal("negative stock accepted", err)
	}
	reversal, err := domain.ReverseSaleStock(uuid.New(), sale, actor, "Incorrect record", now.Add(time.Minute))
	if err != nil || reversal.QuantityDelta != 2 || reversal.SaleItemID != item || reversal.ReversesMovementID != sale.ID || reversal.ShopID != shop {
		t.Fatal("reversal changed source", err)
	}
	restored, err := remaining.Apply(reversal)
	if err != nil || restored.QuantityOnHand != 2 {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.StockMovement){func(m *domain.StockMovement) { m.ShopID = uuid.New() }, func(m *domain.StockMovement) { m.ProductID = uuid.New() }} {
		foreign := sale
		mutate(&foreign)
		if _, err := balance.Apply(foreign); !errors.Is(err, domain.ErrInvalidStock) {
			t.Fatal("foreign movement accepted", err)
		}
	}
	full := balance
	full.QuantityOnHand = domain.MaxStockQuantity
	if _, err := full.Apply(opening); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := domain.ReverseSaleStock(uuid.New(), opening, actor, "Wrong source", now); err == nil {
		t.Fatal("non-sale reversed")
	}
}

func TestStockMovementRejectsWrongSignOrOrigin(t *testing.T) {
	now := time.Now()
	valid := domain.StockMovement{ID: uuid.New(), ShopID: uuid.New(), ProductID: uuid.New(), ActorID: uuid.New(), Kind: domain.StockReceipt, QuantityDelta: 1, Reason: "Delivery"}
	for _, mutate := range []func(*domain.StockMovement){
		func(m *domain.StockMovement) { m.ID = uuid.Nil }, func(m *domain.StockMovement) { m.ActorID = uuid.Nil },
		func(m *domain.StockMovement) { m.QuantityDelta = 0 }, func(m *domain.StockMovement) { m.QuantityDelta = math.MinInt64 },
		func(m *domain.StockMovement) { m.QuantityDelta = -1 }, func(m *domain.StockMovement) { m.SaleItemID = uuid.New() },
		func(m *domain.StockMovement) { m.Kind = "unknown" }, func(m *domain.StockMovement) { m.Reason = " " },
		func(m *domain.StockMovement) { m.Kind = domain.StockSale },
		func(m *domain.StockMovement) { m.Kind = domain.StockSaleReversal; m.SaleItemID = uuid.New() },
	} {
		input := valid
		mutate(&input)
		if _, err := domain.NewStockMovement(input, now); !errors.Is(err, domain.ErrInvalidStock) {
			t.Fatal("invalid movement accepted", err)
		}
	}
	valid.Kind, valid.QuantityDelta = domain.StockAdjustment, -1
	if _, err := domain.NewStockMovement(valid, now); err != nil {
		t.Fatal("negative adjustment rejected", err)
	}
	if _, err := domain.NewStockBalance(valid.ShopID, valid.ProductID, -1); err == nil {
		t.Fatal("negative minimum accepted")
	}
	balance, _ := domain.NewStockBalance(valid.ShopID, valid.ProductID, 0)
	balance.QuantityOnHand = domain.MaxStockQuantity
	valid.Kind, valid.QuantityDelta = domain.StockReceipt, 1
	m, _ := domain.NewStockMovement(valid, now)
	if _, err := balance.Apply(m); !errors.Is(err, domain.ErrStockLimitExceeded) {
		t.Fatal("quantity limit not checked", err)
	}
}

func TestStockMinimumEditPreservesQuantityAndChecksVersion(t *testing.T) {
	balance, _ := domain.NewStockBalance(uuid.New(), uuid.New(), 0)
	balance.QuantityOnHand = 5
	updated, err := balance.SetMinimum(6, 1)
	if err != nil || updated.QuantityOnHand != 5 || updated.Version != 2 || !updated.LowStock() || balance.MinimumQuantity != 0 {
		t.Fatal("minimum edit changed stock", err)
	}
	if _, err := updated.SetMinimum(2, 1); !errors.Is(err, domain.ErrStockVersionConflict) {
		t.Fatal("stale minimum edit accepted", err)
	}
	if _, err := updated.SetMinimum(-1, 2); !errors.Is(err, domain.ErrInvalidStock) {
		t.Fatal("negative minimum accepted", err)
	}
	if same, err := updated.SetMinimum(6, 2); err != nil || same.Version != 2 {
		t.Fatal("no-op minimum edit changed version", err)
	}
}
