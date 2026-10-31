package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type SalesCatalog struct{ q *db.Queries }

func NewSalesCatalog(q *db.Queries) *SalesCatalog { return &SalesCatalog{q} }

var _ sales.SaleCatalogReader = (*SalesCatalog)(nil)
var _ sales.SaleStockRecorder = (*SalesCatalog)(nil)
var _ sales.SaleStockReturner = (*SalesCatalog)(nil)

func (s *SalesCatalog) FindSaleProfessional(ctx context.Context, shopID, professionalID uuid.UUID) (sales.SaleProfessional, bool, error) {
	r, err := s.q.LockSalesProfessional(ctx, db.LockSalesProfessionalParams{ShopID: shopID, ID: professionalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.SaleProfessional{}, false, nil
	}
	if err != nil {
		return sales.SaleProfessional{}, false, err
	}
	return sales.SaleProfessional{ID: r.ID, ShopID: r.ShopID, Name: r.Name, Active: r.Active}, true, nil
}
func (s *SalesCatalog) LockSaleProducts(ctx context.Context, shopID uuid.UUID, ids []uuid.UUID) ([]sales.ProductForSale, error) {
	ids = append([]uuid.UUID(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	products, err := s.q.LockSaleCatalogProducts(ctx, db.LockSaleCatalogProductsParams{ShopID: shopID, ProductIds: ids})
	if err != nil {
		return nil, err
	}
	stocks, err := s.q.LockSaleProductStocks(ctx, db.LockSaleProductStocksParams{ShopID: shopID, ProductIds: ids})
	if err != nil {
		return nil, err
	}
	quantities := make(map[uuid.UUID]int64, len(stocks))
	for _, r := range stocks {
		quantities[r.ProductID] = int64(r.QuantityOnHand)
	}
	out := make([]sales.ProductForSale, 0, len(products))
	for _, p := range products {
		quantity, found := quantities[p.ID]
		if !found {
			return nil, sales.ErrInconsistentSalesData
		}
		out = append(out, sales.ProductForSale{ID: p.ID, ShopID: p.ShopID, Name: p.Name, Currency: p.Currency, PriceCents: p.PriceCents, Version: p.Version, QuantityOnHand: quantity, Active: p.Active})
	}
	return out, nil
}
func (s *SalesCatalog) RecordSaleStock(ctx context.Context, command sales.SaleStockCommand) error {
	items := append([]sales.SaleStockItem(nil), command.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID.String() < items[j].ProductID.String() })
	type movementWrite struct {
		ID            uuid.UUID `json:"id"`
		ShopID        uuid.UUID `json:"shop_id"`
		ProductID     uuid.UUID `json:"product_id"`
		ActorID       uuid.UUID `json:"actor_id"`
		QuantityDelta int64     `json:"quantity_delta"`
		SaleItemID    uuid.UUID `json:"sale_item_id"`
		CreatedAt     time.Time `json:"created_at"`
	}
	movements := make([]movementWrite, 0, len(items))
	for _, item := range items {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		m, err := domain.NewStockMovement(domain.StockMovement{ID: id, ShopID: command.ShopID, ProductID: item.ProductID, ActorID: command.ActorID, Kind: domain.StockSale, QuantityDelta: -item.Quantity, Reason: "Product sale", SaleItemID: item.SaleItemID}, command.OccurredAt)
		if err != nil {
			return err
		}
		movements = append(movements, movementWrite{m.ID, m.ShopID, m.ProductID, m.ActorID, m.QuantityDelta, m.SaleItemID, m.CreatedAt})
	}
	data, err := json.Marshal(movements)
	if err != nil {
		return err
	}
	count, err := s.q.AppendSaleStockMovements(ctx, data)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "product_stock_available" {
		return sales.ErrSaleStockUnavailable
	}
	if err != nil {
		return err
	}
	if count != int64(len(items)) {
		return sales.ErrInconsistentSalesData
	}
	return nil
}

func (s *SalesCatalog) RestockCancelledSale(ctx context.Context, command sales.SaleStockCommand) error {
	items := make(map[uuid.UUID]sales.SaleStockItem, len(command.Items))
	ids := make([]uuid.UUID, 0, len(command.Items))
	for _, item := range command.Items {
		if item.SaleItemID == uuid.Nil || item.ProductID == uuid.Nil || item.Quantity < 1 || item.Quantity > domain.MaxStockQuantity || items[item.SaleItemID].SaleItemID != uuid.Nil {
			return sales.ErrInconsistentSalesData
		}
		items[item.SaleItemID] = item
		ids = append(ids, item.SaleItemID)
	}
	originals, err := s.q.ReadOriginalSaleStock(ctx, db.ReadOriginalSaleStockParams{ShopID: command.ShopID, ItemIds: ids})
	if err != nil {
		return err
	}
	if len(originals) != len(items) || len(items) == 0 {
		return sales.ErrInconsistentSalesData
	}
	// Original immutable withdrawals supply the quantities and origin IDs.
	// The ordered INSERT follows the same product order as sale/stock writes.
	type reversalWrite struct {
		ID                 uuid.UUID `json:"id"`
		ShopID             uuid.UUID `json:"shop_id"`
		ProductID          uuid.UUID `json:"product_id"`
		ActorID            uuid.UUID `json:"actor_id"`
		QuantityDelta      int64     `json:"quantity_delta"`
		Reason             string    `json:"reason"`
		SaleItemID         uuid.UUID `json:"sale_item_id"`
		ReversesMovementID uuid.UUID `json:"reverses_movement_id"`
		CreatedAt          time.Time `json:"created_at"`
	}
	writes := make([]reversalWrite, 0, len(originals))
	for _, r := range originals {
		if !r.SaleItemID.Valid || r.ShopID != command.ShopID || !r.CreatedAt.Valid {
			return sales.ErrInconsistentSalesData
		}
		item, ok := items[r.SaleItemID.Bytes]
		if !ok || item.ProductID != r.ProductID || item.Quantity != -int64(r.QuantityDelta) {
			return sales.ErrInconsistentSalesData
		}
		original := domain.StockMovement{ID: r.ID, ShopID: r.ShopID, ProductID: r.ProductID, ActorID: r.ActorID, Kind: domain.StockMovementKind(r.Kind), QuantityDelta: int64(r.QuantityDelta), Reason: r.Reason, SaleItemID: r.SaleItemID.Bytes, CreatedAt: r.CreatedAt.Time}
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		m, err := domain.ReverseSaleStock(id, original, command.ActorID, command.Reason, command.OccurredAt)
		if err != nil {
			return err
		}
		writes = append(writes, reversalWrite{m.ID, m.ShopID, m.ProductID, m.ActorID, m.QuantityDelta, m.Reason, m.SaleItemID, m.ReversesMovementID, m.CreatedAt})
		delete(items, item.SaleItemID)
	}
	data, err := json.Marshal(writes)
	if err != nil {
		return err
	}
	count, err := s.q.AppendSaleStockReversals(ctx, data)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "product_stock_limit" {
		return sales.ErrSaleStockLimitExceeded
	}
	if err != nil {
		return err
	}
	if count != int64(len(writes)) {
		return sales.ErrInconsistentSalesData
	}
	return nil
}
