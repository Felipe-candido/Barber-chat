package postgres

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ q *db.Queries }

func NewStore(q *db.Queries) *Store { return &Store{q} }

var _ application.SaleRecorder = (*Store)(nil)
var _ application.SaleCommandStore = (*Store)(nil)
var _ application.SaleStore = (*Store)(nil)
var _ application.SaleReader = (*Store)(nil)

func saleUUID(id uuid.UUID) pgtype.UUID       { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }
func saleTime(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func (s *Store) InsertSale(ctx context.Context, sale domain.Sale) error {
	if sale.Status != domain.SaleRecorded || sale.Version != 1 || sale.Currency != "BRL" || sale.Cancellation != nil {
		return application.ErrInconsistentSalesData
	}
	if err := s.q.InsertRecordedSale(ctx, db.InsertRecordedSaleParams{ID: sale.ID, ShopID: sale.ShopID, RecordedByUserID: sale.RecordedByUserID, ProfessionalID: saleUUID(sale.ProfessionalID), AppointmentID: saleUUID(sale.AppointmentID), ProfessionalNameSnapshot: sale.ProfessionalName, TotalCents: sale.TotalCents, SoldAt: saleTime(sale.SoldAt), CreatedAt: saleTime(sale.CreatedAt)}); err != nil {
		return err
	}
	type itemWrite struct {
		ID             uuid.UUID `json:"id"`
		ShopID         uuid.UUID `json:"shop_id"`
		SaleID         uuid.UUID `json:"sale_id"`
		ProductID      uuid.UUID `json:"product_id"`
		ProductName    string    `json:"product_name"`
		Quantity       int64     `json:"quantity"`
		UnitPriceCents int64     `json:"unit_price_cents"`
	}
	items := make([]itemWrite, 0, len(sale.Items))
	for _, item := range sale.Items {
		if item.Quantity < 1 || item.Quantity > domain.MaxItemQuantity {
			return domain.ErrInvalidSaleItem
		}
		items = append(items, itemWrite{item.ID, sale.ShopID, sale.ID, item.ProductID, item.ProductName, item.Quantity, item.UnitPriceCents})
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	count, err := s.q.InsertRecordedSaleItems(ctx, data)
	if err != nil {
		return err
	}
	if count != int64(len(items)) {
		return application.ErrInconsistentSalesData
	}
	return nil
}
func (s *Store) FindRecordedSale(ctx context.Context, shopID, saleID uuid.UUID) (domain.Sale, bool, error) {
	h, err := s.q.GetRecordedSale(ctx, db.GetRecordedSaleParams{ShopID: shopID, ID: saleID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Sale{}, false, nil
	}
	if err != nil {
		return domain.Sale{}, false, err
	}
	rows, err := s.q.GetRecordedSaleItems(ctx, db.GetRecordedSaleItemsParams{ShopID: shopID, SaleID: saleID})
	if err != nil {
		return domain.Sale{}, false, err
	}
	sale, err := hydrateSale(h, rows)
	return sale, err == nil, err
}

func hydrateSale(h db.Sale, rows []db.SaleItem) (domain.Sale, error) {
	if !h.SoldAt.Valid || h.SoldAt.InfinityModifier != pgtype.Finite || !h.CreatedAt.Valid || h.CreatedAt.InfinityModifier != pgtype.Finite || h.Currency != "BRL" {
		return domain.Sale{}, application.ErrInconsistentSalesData
	}
	draft := domain.SaleDraft{ID: h.ID, ShopID: h.ShopID, RecordedByUserID: h.RecordedByUserID, ProfessionalName: h.ProfessionalNameSnapshot, Items: make([]domain.ItemDraft, 0, len(rows))}
	if h.ProfessionalID.Valid {
		draft.ProfessionalID = h.ProfessionalID.Bytes
	}
	if h.AppointmentID.Valid {
		draft.AppointmentID = h.AppointmentID.Bytes
	}
	for _, r := range rows {
		if r.ShopID != h.ShopID || r.SaleID != h.ID || !r.LineTotalCents.Valid || r.Quantity <= 0 || r.UnitPriceCents > domain.MaxSaleTotalCents/int64(r.Quantity) || r.LineTotalCents.Int64 != r.UnitPriceCents*int64(r.Quantity) {
			return domain.Sale{}, application.ErrInconsistentSalesData
		}
		draft.Items = append(draft.Items, domain.ItemDraft{ID: r.ID, ShopID: r.ShopID, ProductID: r.ProductID, ProductName: r.ProductNameSnapshot, Quantity: int64(r.Quantity), UnitPriceCents: r.UnitPriceCents, Currency: r.Currency})
	}
	sale, err := domain.NewSale(draft, h.SoldAt.Time)
	if err != nil || sale.TotalCents != h.TotalCents {
		return domain.Sale{}, application.ErrInconsistentSalesData
	}
	sale.CreatedAt = h.CreatedAt.Time.UTC()
	if h.Status == string(domain.SaleCancelled) {
		if !h.CancelledByUserID.Valid || !h.CancellationReason.Valid || !h.Restock.Valid || !h.CancelledAt.Valid || h.CancelledAt.InfinityModifier != pgtype.Finite {
			return domain.Sale{}, application.ErrInconsistentSalesData
		}
		sale, err = sale.Cancel(h.CancelledByUserID.Bytes, h.CancellationReason.String, h.Restock.Bool, 1, h.CancelledAt.Time)
		if err != nil {
			return domain.Sale{}, application.ErrInconsistentSalesData
		}
	} else if h.Status != string(domain.SaleRecorded) || h.CancelledByUserID.Valid || h.CancellationReason.Valid || h.Restock.Valid || h.CancelledAt.Valid {
		return domain.Sale{}, application.ErrInconsistentSalesData
	}
	if sale.Version != h.Version {
		return domain.Sale{}, application.ErrInconsistentSalesData
	}
	return sale, nil
}

func (s *Store) FindSale(ctx context.Context, shopID, saleID, professionalID uuid.UUID) (domain.Sale, bool, error) {
	h, err := s.q.GetReadableSale(ctx, db.GetReadableSaleParams{ShopID: shopID, SaleID: saleID, ProfessionalID: saleUUID(professionalID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Sale{}, false, nil
	}
	if err != nil {
		return domain.Sale{}, false, err
	}
	rows, err := s.q.GetRecordedSaleItems(ctx, db.GetRecordedSaleItemsParams{ShopID: shopID, SaleID: saleID})
	if err != nil {
		return domain.Sale{}, false, err
	}
	sale, err := hydrateSale(h, rows)
	return sale, err == nil, err
}

func (s *Store) LockSale(ctx context.Context, shopID, saleID uuid.UUID) (domain.Sale, bool, error) {
	h, err := s.q.LockSaleForCancellation(ctx, db.LockSaleForCancellationParams{ShopID: shopID, ID: saleID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Sale{}, false, nil
	}
	if err != nil {
		return domain.Sale{}, false, err
	}
	rows, err := s.q.GetRecordedSaleItems(ctx, db.GetRecordedSaleItemsParams{ShopID: shopID, SaleID: saleID})
	if err != nil {
		return domain.Sale{}, false, err
	}
	sale, err := hydrateSale(h, rows)
	return sale, err == nil, err
}

func (s *Store) SaveSaleCancellation(ctx context.Context, sale domain.Sale, expectedVersion int64) (bool, error) {
	c := sale.Cancellation
	if sale.Status != domain.SaleCancelled || c == nil || sale.Version != expectedVersion+1 {
		return false, application.ErrInconsistentSalesData
	}
	count, err := s.q.SaveSaleCancellation(ctx, db.SaveSaleCancellationParams{ShopID: sale.ShopID, SaleID: sale.ID, ActorID: saleUUID(c.ActorID), Reason: pgtype.Text{String: c.Reason, Valid: true}, Restock: pgtype.Bool{Bool: c.Restock, Valid: true}, CancelledAt: saleTime(c.CancelledAt), ExpectedVersion: expectedVersion})
	return count == 1, err
}

func (s *Store) ListSales(ctx context.Context, query application.SaleListQuery) ([]domain.Sale, error) {
	params := db.ListSalesPageParams{ShopID: query.ShopID, ProfessionalID: saleUUID(query.ProfessionalID), FromAt: saleTime(query.From), UntilAt: saleTime(query.Until), Status: string(query.Status), PageLimit: int32(query.Limit)}
	if query.After != nil {
		params.AfterAt = saleTime(query.After.SoldAt)
		params.AfterID = saleUUID(query.After.SaleID)
	}
	headers, err := s.q.ListSalesPage(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Sale, 0, len(headers))
	if len(headers) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, 0, len(headers))
	for _, h := range headers {
		ids = append(ids, h.ID)
	}
	rows, err := s.q.ListSalesPageItems(ctx, db.ListSalesPageItemsParams{ShopID: query.ShopID, SaleIds: ids})
	if err != nil {
		return nil, err
	}
	bySale := make(map[uuid.UUID][]db.SaleItem, len(headers))
	for _, row := range rows {
		bySale[row.SaleID] = append(bySale[row.SaleID], row)
	}
	for _, h := range headers {
		sale, err := hydrateSale(h, bySale[h.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, sale)
	}
	return out, nil
}
func (s *Store) ClaimSaleCommand(ctx context.Context, scope application.SalesScope, key application.SaleCommandKey) (uuid.UUID, bool, error) {
	count, err := s.q.ClaimSaleCommand(ctx, db.ClaimSaleCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, Operation: string(key.Operation), KeyDigest: key.Digest[:], Fingerprint: key.Fingerprint[:]})
	if err != nil {
		return uuid.Nil, false, err
	}
	if count == 1 {
		return uuid.Nil, false, nil
	}
	r, err := s.q.LockSaleCommand(ctx, db.LockSaleCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, Operation: string(key.Operation), KeyDigest: key.Digest[:]})
	if err != nil {
		return uuid.Nil, false, err
	}
	if subtle.ConstantTimeCompare(r.Fingerprint, key.Fingerprint[:]) != 1 {
		return uuid.Nil, false, application.ErrSaleCommandConflict
	}
	if !r.Completed {
		return uuid.Nil, false, application.ErrSaleCommandPending
	}
	if !r.SaleID.Valid {
		return uuid.Nil, false, application.ErrInconsistentSalesData
	}
	return r.SaleID.Bytes, true, nil
}
func (s *Store) CompleteSaleCommand(ctx context.Context, scope application.SalesScope, key application.SaleCommandKey, saleID uuid.UUID) error {
	count, err := s.q.CompleteSaleCommand(ctx, db.CompleteSaleCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, Operation: string(key.Operation), KeyDigest: key.Digest[:], Fingerprint: key.Fingerprint[:], SaleID: saleUUID(saleID)})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrInconsistentSalesData
	}
	return nil
}

type Transactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.SalesTx
}

func NewTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.SalesTx) *Transactor {
	return &Transactor{pool, bind}
}
func (t *Transactor) WithinSalesTx(ctx context.Context, fn func(context.Context, application.SalesTx) error) error {
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
