package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
)

type CancelSale struct {
	tx  SalesTransactor
	now func() time.Time
}

func NewCancelSale(tx SalesTransactor, now func() time.Time) *CancelSale {
	if now == nil {
		now = time.Now
	}
	return &CancelSale{tx, now}
}

func normalizeCancellation(input CancelSaleInput) (CancelSaleInput, SaleCommandKey, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil || input.SaleID == uuid.Nil || input.ExpectedVersion < 1 ||
		!utf8.ValidString(input.Reason) || utf8.RuneCountInString(input.Reason) < 1 || utf8.RuneCountInString(input.Reason) > 500 || strings.ContainsFunc(input.Reason, unicode.IsControl) {
		return input, SaleCommandKey{}, ErrInvalidSalesInput
	}
	raw, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != input.IdempotencyKey {
		return input, SaleCommandKey{}, ErrInvalidSaleCommandKey
	}
	// Persisted protocol, independent from future additions to the public DTO.
	canonical := struct {
		Protocol                int
		ActorID, ShopID, SaleID uuid.UUID
		ExpectedVersion         int64
		Reason                  string
		Restock                 bool
	}{1, input.Scope.ActorID, input.Scope.ShopID, input.SaleID, input.ExpectedVersion, input.Reason, input.Restock}
	data, err := json.Marshal(canonical)
	if err != nil {
		return input, SaleCommandKey{}, err
	}
	mac := hmac.New(sha256.New, raw)
	_, _ = mac.Write(data)
	key := SaleCommandKey{Operation: CancelSaleOperation, Digest: sha256.Sum256(raw)}
	copy(key.Fingerprint[:], mac.Sum(nil))
	return input, key, nil
}

func (uc *CancelSale) Execute(ctx context.Context, raw CancelSaleInput) (SaleOutput, error) {
	input, key, err := normalizeCancellation(raw)
	if err != nil {
		return SaleOutput{}, err
	}
	var output SaleOutput
	err = uc.tx.WithinSalesTx(ctx, func(ctx context.Context, tx SalesTx) error {
		preliminary, err := tx.Access.ReadSalesAccess(ctx, input.Scope)
		if err != nil {
			return err
		}
		if err := preliminary.RequireOwner(input.Scope); err != nil {
			return err
		}
		resultID, replayed, err := tx.Commands.ClaimSaleCommand(ctx, input.Scope, key)
		if err != nil {
			return err
		}
		authority, err := tx.Access.RequireSalesAccess(ctx, input.Scope)
		if err != nil {
			return err
		}
		if err := authority.RequireOwner(input.Scope); err != nil {
			return err
		}
		if replayed && resultID != input.SaleID {
			return ErrInconsistentSalesData
		}
		// This lock serializes different cancellation keys for the same sale.
		sale, found, err := tx.Cancellation.LockSale(ctx, input.Scope.ShopID, input.SaleID)
		if err != nil {
			return err
		}
		if !found {
			return ErrSaleNotFound
		}
		if sale.ID != input.SaleID || sale.ShopID != input.Scope.ShopID {
			return ErrInconsistentSalesData
		}
		if replayed {
			if sale.Status != domain.SaleCancelled || sale.Cancellation == nil || sale.Cancellation.Reason != input.Reason || sale.Cancellation.Restock != input.Restock {
				return ErrInconsistentSalesData
			}
			output = saleOutput(sale, true)
			return nil
		}
		cancelled, err := sale.Cancel(input.Scope.ActorID, input.Reason, input.Restock, input.ExpectedVersion, uc.now().UTC().Truncate(time.Microsecond))
		if err != nil {
			return err
		}
		alreadyCancelled := sale.Status == domain.SaleCancelled
		if !alreadyCancelled {
			saved, err := tx.Cancellation.SaveSaleCancellation(ctx, cancelled, input.ExpectedVersion)
			if err != nil {
				return err
			}
			if !saved {
				return domain.ErrSaleVersionConflict
			}
			if input.Restock {
				command := SaleStockCommand{ShopID: sale.ShopID, ActorID: input.Scope.ActorID, OccurredAt: cancelled.Cancellation.CancelledAt, Reason: cancelled.Cancellation.Reason, Items: make([]SaleStockItem, 0, len(sale.Items))}
				for _, item := range sale.Items {
					command.Items = append(command.Items, SaleStockItem{SaleItemID: item.ID, ProductID: item.ProductID, Quantity: item.Quantity})
				}
				if err := tx.Returns.RestockCancelledSale(ctx, command); err != nil {
					return err
				}
			}
		}
		if err := tx.Commands.CompleteSaleCommand(ctx, input.Scope, key, sale.ID); err != nil {
			return err
		}
		persisted, found, err := tx.Sales.FindRecordedSale(ctx, sale.ShopID, sale.ID)
		if err != nil {
			return err
		}
		if !found || persisted.Status != domain.SaleCancelled {
			return ErrInconsistentSalesData
		}
		output = saleOutput(persisted, alreadyCancelled)
		return nil
	})
	if err != nil {
		return SaleOutput{}, err
	}
	return output, nil
}

func saleOutput(sale domain.Sale, replayed bool) SaleOutput {
	items := make([]SaleItemOutput, 0, len(sale.Items))
	for _, item := range sale.Items {
		items = append(items, SaleItemOutput{ID: item.ID, ProductID: item.ProductID, ProductName: item.ProductName, Quantity: item.Quantity, UnitPriceCents: item.UnitPriceCents, LineTotalCents: item.LineTotalCents, Currency: item.Currency})
	}
	out := SaleOutput{ID: sale.ID, ShopID: sale.ShopID, RecordedByUserID: sale.RecordedByUserID, ProfessionalID: sale.ProfessionalID, AppointmentID: sale.AppointmentID, ProfessionalName: sale.ProfessionalName, Currency: sale.Currency, Items: items, Status: sale.Status, TotalCents: sale.TotalCents, Version: sale.Version, SoldAt: sale.SoldAt, Replayed: replayed}
	if c := sale.Cancellation; c != nil {
		out.Cancellation = &SaleCancellationOutput{ActorID: c.ActorID, Reason: c.Reason, Restock: c.Restock, CancelledAt: c.CancelledAt}
	}
	return out
}
