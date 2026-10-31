package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductStore struct{ q *db.Queries }

func NewProductStore(q *db.Queries) *ProductStore { return &ProductStore{q} }

var _ application.ProductRepository = (*ProductStore)(nil)
var _ application.ProductStockStore = (*ProductStore)(nil)
var _ application.ProductCommandStore = (*ProductStore)(nil)
var _ application.ProductReader = (*ProductStore)(nil)

func productTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}
func productUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }
func productInstant(t pgtype.Timestamptz) (time.Time, error) {
	if !t.Valid || t.InfinityModifier != pgtype.Finite {
		return time.Time{}, application.ErrInconsistentProductData
	}
	return t.Time.UTC(), nil
}

func (s *ProductStore) InsertProduct(ctx context.Context, p domain.Product) error {
	return s.q.InsertProduct(ctx, db.InsertProductParams{ID: p.ID, ShopID: p.ShopID, Name: p.Name, Description: p.Description, PriceCents: p.PriceCents, Currency: p.Currency, Active: p.Active, Version: p.Version, CreatedAt: productTimestamp(p.CreatedAt), UpdatedAt: productTimestamp(p.UpdatedAt)})
}
func (s *ProductStore) LockProduct(ctx context.Context, shopID, productID uuid.UUID) (domain.Product, bool, error) {
	r, err := s.q.LockProduct(ctx, db.LockProductParams{ShopID: shopID, ID: productID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, false, nil
	}
	if err != nil {
		return domain.Product{}, false, err
	}
	created, err := productInstant(r.CreatedAt)
	if err != nil {
		return domain.Product{}, false, err
	}
	updated, err := productInstant(r.UpdatedAt)
	if err != nil {
		return domain.Product{}, false, err
	}
	return domain.Product{ID: r.ID, ShopID: r.ShopID, ProductDetails: domain.ProductDetails{Name: r.Name, Description: r.Description, PriceCents: r.PriceCents, Active: r.Active}, Currency: r.Currency, Version: r.Version, CreatedAt: created, UpdatedAt: updated}, true, nil
}
func (s *ProductStore) UpdateProduct(ctx context.Context, p domain.Product, version int64) (bool, error) {
	count, err := s.q.UpdateProduct(ctx, db.UpdateProductParams{ShopID: p.ShopID, ProductID: p.ID, Name: p.Name, Description: p.Description, PriceCents: p.PriceCents, Active: p.Active, UpdatedAt: productTimestamp(p.UpdatedAt), ExpectedVersion: version})
	return count == 1, err
}
func (s *ProductStore) InsertStockBalance(ctx context.Context, b domain.StockBalance) error {
	if b.QuantityOnHand != 0 || b.Version != 1 || b.MinimumQuantity < 0 || b.MinimumQuantity > domain.MaxStockQuantity {
		return domain.ErrInvalidStock
	}
	return s.q.InsertProductStock(ctx, db.InsertProductStockParams{ShopID: b.ShopID, ProductID: b.ProductID, MinimumQuantity: int32(b.MinimumQuantity)})
}
func (s *ProductStore) LockStockBalance(ctx context.Context, shopID, productID uuid.UUID) (domain.StockBalance, bool, error) {
	r, err := s.q.LockProductStock(ctx, db.LockProductStockParams{ShopID: shopID, ProductID: productID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StockBalance{}, false, nil
	}
	if err != nil {
		return domain.StockBalance{}, false, err
	}
	return domain.StockBalance{ShopID: r.ShopID, ProductID: r.ProductID, QuantityOnHand: int64(r.QuantityOnHand), MinimumQuantity: int64(r.MinimumQuantity), Version: r.Version}, true, nil
}
func (s *ProductStore) AppendStockMovement(ctx context.Context, m domain.StockMovement) error {
	validated, err := domain.NewStockMovement(m, m.CreatedAt)
	if err != nil {
		return err
	}
	if validated.Kind == domain.StockSale || validated.Kind == domain.StockSaleReversal {
		return domain.ErrInvalidStock
	}
	err = s.q.AppendProductStockMovement(ctx, db.AppendProductStockMovementParams{ID: m.ID, ShopID: m.ShopID, ProductID: m.ProductID, ActorID: m.ActorID, Kind: string(m.Kind), QuantityDelta: int32(m.QuantityDelta), Reason: validated.Reason, CreatedAt: productTimestamp(m.CreatedAt)})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "product_stock_available":
			return domain.ErrInsufficientStock
		case "product_stock_limit":
			return domain.ErrStockLimitExceeded
		}
	}
	return err
}
func (s *ProductStore) UpdateStockMinimum(ctx context.Context, shopID, productID uuid.UUID, minimum, version int64) (bool, error) {
	if minimum < 0 || minimum > domain.MaxStockQuantity {
		return false, domain.ErrInvalidStock
	}
	count, err := s.q.UpdateProductStockMinimum(ctx, db.UpdateProductStockMinimumParams{ShopID: shopID, ProductID: productID, MinimumQuantity: int32(minimum), ExpectedVersion: version})
	return count == 1, err
}

func productView(r db.FindProductOutputRow) (application.ProductOutput, error) {
	created, err := productInstant(r.CreatedAt)
	if err != nil {
		return application.ProductOutput{}, err
	}
	updated, err := productInstant(r.UpdatedAt)
	if err != nil {
		return application.ProductOutput{}, err
	}
	return application.ProductOutput{ID: r.ID, ShopID: r.ShopID, Name: r.Name, Description: r.Description, PriceCents: r.PriceCents, Currency: r.Currency, Active: r.Active, Version: r.Version, CreatedAt: created, UpdatedAt: updated, QuantityOnHand: int64(r.QuantityOnHand), MinimumQuantity: int64(r.MinimumQuantity), StockVersion: r.StockVersion}, nil
}
func (s *ProductStore) FindProductOutput(ctx context.Context, shopID, productID uuid.UUID) (application.ProductOutput, bool, error) {
	r, err := s.q.FindProductOutput(ctx, db.FindProductOutputParams{ShopID: shopID, ID: productID})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ProductOutput{}, false, nil
	}
	if err != nil {
		return application.ProductOutput{}, false, err
	}
	view, err := productView(r)
	return view, err == nil, err
}
func (s *ProductStore) ListProducts(ctx context.Context, input application.ProductQuery) ([]application.ProductOutput, error) {
	p := db.ListProductsParams{ShopID: input.ShopID, Search: input.Search, IncludeInactive: input.IncludeInactive, PageLimit: int32(input.Limit)}
	if input.After != nil {
		p.HasAfter = true
		p.AfterID = input.After.ProductID
		p.AfterTime = productTimestamp(input.After.CreatedAt)
	} else {
		p.AfterTime = productTimestamp(time.Unix(0, 0))
	}
	rows, err := s.q.ListProducts(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]application.ProductOutput, 0, len(rows))
	for _, r := range rows {
		view, err := productView(db.FindProductOutputRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}
func (s *ProductStore) ListStockMovements(ctx context.Context, input application.StockMovementQuery) ([]domain.StockMovement, error) {
	p := db.ListProductStockMovementsParams{ShopID: input.ShopID, ProductID: input.ProductID, PageLimit: int32(input.Limit)}
	if input.After != nil {
		p.HasAfter = true
		p.AfterID = input.After.MovementID
		p.AfterTime = productTimestamp(input.After.CreatedAt)
	} else {
		p.AfterTime = productTimestamp(time.Unix(0, 0))
	}
	rows, err := s.q.ListProductStockMovements(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]domain.StockMovement, 0, len(rows))
	for _, r := range rows {
		at, err := productInstant(r.CreatedAt)
		if err != nil {
			return nil, err
		}
		movement := domain.StockMovement{ID: r.ID, ShopID: r.ShopID, ProductID: r.ProductID, ActorID: r.ActorID, Kind: domain.StockMovementKind(r.Kind), QuantityDelta: int64(r.QuantityDelta), Reason: r.Reason, CreatedAt: at}
		if r.SaleItemID.Valid {
			movement.SaleItemID = r.SaleItemID.Bytes
		}
		if r.ReversesMovementID.Valid {
			movement.ReversesMovementID = r.ReversesMovementID.Bytes
		}
		out = append(out, movement)
	}
	return out, nil
}
func (s *ProductStore) ClaimProductCommand(ctx context.Context, scope application.ProductScope, key application.ProductCommandKey) (application.ProductCommandResult, bool, error) {
	count, err := s.q.ClaimProductCommand(ctx, db.ClaimProductCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, Operation: string(key.Operation), KeyDigest: key.Digest[:], Fingerprint: key.Fingerprint[:]})
	if err != nil {
		return application.ProductCommandResult{}, false, err
	}
	if count == 1 {
		return application.ProductCommandResult{}, false, nil
	}
	r, err := s.q.LockProductCommand(ctx, db.LockProductCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, Operation: string(key.Operation), KeyDigest: key.Digest[:]})
	if err != nil {
		return application.ProductCommandResult{}, false, err
	}
	if subtle.ConstantTimeCompare(r.Fingerprint, key.Fingerprint[:]) != 1 {
		return application.ProductCommandResult{}, false, application.ErrProductCommandConflict
	}
	if !r.Completed {
		return application.ProductCommandResult{}, false, application.ErrProductCommandPending
	}
	if !r.ProductID.Valid {
		return application.ProductCommandResult{}, false, application.ErrInconsistentProductData
	}
	result := application.ProductCommandResult{ProductID: r.ProductID.Bytes}
	if r.MovementID.Valid {
		result.MovementID = r.MovementID.Bytes
	}
	return result, true, nil
}
func (s *ProductStore) CompleteProductCommand(ctx context.Context, scope application.ProductScope, key application.ProductCommandKey, result application.ProductCommandResult) error {
	count, err := s.q.CompleteProductCommand(ctx, db.CompleteProductCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, Operation: string(key.Operation), KeyDigest: key.Digest[:], Fingerprint: key.Fingerprint[:], ProductID: productUUID(result.ProductID), MovementID: productUUID(result.MovementID)})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrInconsistentProductData
	}
	return nil
}

type ProductTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.ProductTx
}

func NewProductTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.ProductTx) *ProductTransactor {
	return &ProductTransactor{pool, bind}
}
func (t *ProductTransactor) WithinProductTx(ctx context.Context, fn func(context.Context, application.ProductTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
