package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
)

type RegisterSale struct {
	tx  SalesTransactor
	now func() time.Time
}

func NewRegisterSale(tx SalesTransactor, now func() time.Time) *RegisterSale {
	if now == nil {
		now = time.Now
	}
	return &RegisterSale{tx, now}
}

func normalizeSale(input RegisterSaleInput) (RegisterSaleInput, SaleCommandKey, error) {
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil {
		return input, SaleCommandKey{}, ErrSalesAccessDenied
	}
	if len(input.Items) < 1 || len(input.Items) > domain.MaxSaleItems {
		return input, SaleCommandKey{}, ErrInvalidSalesInput
	}
	input.Items = append([]SaleItemInput(nil), input.Items...)
	sort.Slice(input.Items, func(i, j int) bool { return input.Items[i].ProductID.String() < input.Items[j].ProductID.String() })
	for i, item := range input.Items {
		if item.ProductID == uuid.Nil || item.Quantity < 1 || item.Quantity > domain.MaxItemQuantity || item.ExpectedProductVersion < 1 || item.ExpectedUnitPriceCents < 1 || item.ExpectedUnitPriceCents > domain.MaxSaleTotalCents || (i > 0 && input.Items[i-1].ProductID == item.ProductID) {
			return input, SaleCommandKey{}, ErrInvalidSalesInput
		}
	}
	raw, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != input.IdempotencyKey {
		return input, SaleCommandKey{}, ErrInvalidSaleCommandKey
	}
	// Freeze the persisted fingerprint format: adding a DTO field must not
	// silently invalidate receipts from earlier deployments of protocol 1.
	type canonicalItem struct {
		ProductID                                                uuid.UUID
		Quantity, ExpectedProductVersion, ExpectedUnitPriceCents int64
	}
	canonical := struct {
		Protocol                                       int
		ActorID, ShopID, ProfessionalID, AppointmentID uuid.UUID
		Items                                          []canonicalItem
	}{
		Protocol: 1, ActorID: input.Scope.ActorID, ShopID: input.Scope.ShopID, ProfessionalID: input.ProfessionalID, AppointmentID: input.AppointmentID, Items: make([]canonicalItem, 0, len(input.Items)),
	}
	for _, item := range input.Items {
		canonical.Items = append(canonical.Items, canonicalItem{item.ProductID, item.Quantity, item.ExpectedProductVersion, item.ExpectedUnitPriceCents})
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return input, SaleCommandKey{}, err
	}
	mac := hmac.New(sha256.New, raw)
	_, _ = mac.Write(encoded)
	key := SaleCommandKey{Operation: RegisterSaleOperation, Digest: sha256.Sum256(raw)}
	copy(key.Fingerprint[:], mac.Sum(nil))
	return input, key, nil
}

func (uc *RegisterSale) Execute(ctx context.Context, raw RegisterSaleInput) (SaleOutput, error) {
	input, key, err := normalizeSale(raw)
	if err != nil {
		return SaleOutput{}, err
	}
	var output SaleOutput
	err = uc.tx.WithinSalesTx(ctx, func(ctx context.Context, tx SalesTx) error {
		preliminary, err := tx.Access.ReadSalesAccess(ctx, input.Scope)
		if err != nil {
			return err
		}
		// The claim is rolled back unless the whole command commits. No partial
		// claim is allowed to commit, and no untrusted receipt is disclosed here.
		saleID, replayed, err := tx.Commands.ClaimSaleCommand(ctx, input.Scope, key)
		if err != nil {
			return err
		}
		if replayed {
			authority, err := tx.Access.RequireSalesAccess(ctx, input.Scope)
			if err != nil {
				return err
			}
			return loadSaleOutput(ctx, tx, input.Scope, authority, saleID, true, &output)
		}
		requested := input.ProfessionalID
		if input.AppointmentID != uuid.Nil {
			appointment, found, err := tx.Appointments.ReadSaleAppointment(ctx, input.Scope.ShopID, input.AppointmentID)
			if err != nil {
				return err
			}
			if !found {
				return ErrSaleAppointmentUnavailable
			}
			if appointment.ShopID != input.Scope.ShopID || appointment.ID != input.AppointmentID {
				return ErrInconsistentSalesData
			}
			if requested == uuid.Nil {
				requested = appointment.ProfessionalID
			} else if requested != appointment.ProfessionalID {
				return ErrSaleAppointmentUnavailable
			}
		}
		professionalID, err := preliminary.ProfessionalFor(input.Scope, requested)
		if err != nil {
			return err
		}
		professionalName := ""
		if professionalID != uuid.Nil {
			professional, found, err := tx.Catalog.FindSaleProfessional(ctx, input.Scope.ShopID, professionalID)
			if err != nil {
				return err
			}
			if !found || !professional.Active {
				return ErrSalesAccessDenied
			}
			if professional.ID != professionalID || professional.ShopID != input.Scope.ShopID {
				return ErrInconsistentSalesData
			}
			professionalName = professional.Name
		}
		// Professional is locked before membership/user/shop, as in booking.
		authority, err := tx.Access.RequireSalesAccess(ctx, input.Scope)
		if err != nil {
			return err
		}
		currentProfessional, err := authority.ProfessionalFor(input.Scope, requested)
		if err != nil {
			return err
		}
		if currentProfessional != professionalID {
			return ErrSalesAccessDenied
		}
		if input.AppointmentID != uuid.Nil {
			a, found, err := tx.Appointments.LockSaleAppointment(ctx, input.Scope.ShopID, input.AppointmentID)
			if err != nil {
				return err
			}
			if !found || a.ProfessionalID != professionalID {
				return ErrSaleAppointmentUnavailable
			}
			if a.ShopID != input.Scope.ShopID || a.ID != input.AppointmentID {
				return ErrInconsistentSalesData
			}
			if a.State != SaleAppointmentScheduled && a.State != SaleAppointmentConfirmed && a.State != SaleAppointmentCompleted {
				return ErrSaleAppointmentUnavailable
			}
		}
		if err := tx.Eligibility.RequireNewSaleCoverage(ctx, input.Scope.ShopID, uc.now().UTC()); err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(input.Items))
		for i, item := range input.Items {
			ids[i] = item.ProductID
		}
		products, err := tx.Catalog.LockSaleProducts(ctx, input.Scope.ShopID, ids)
		if err != nil {
			return err
		}
		if len(products) != len(ids) {
			return ErrSaleProductUnavailable
		}
		byID := make(map[uuid.UUID]ProductForSale, len(products))
		for _, p := range products {
			if p.ShopID != input.Scope.ShopID || p.ID == uuid.Nil || byID[p.ID].ID != uuid.Nil {
				return ErrInconsistentSalesData
			}
			byID[p.ID] = p
		}
		drafts := make([]domain.ItemDraft, 0, len(ids))
		for _, item := range input.Items {
			p, found := byID[item.ProductID]
			if !found || !p.Active {
				return ErrSaleProductUnavailable
			}
			if p.Version != item.ExpectedProductVersion || p.PriceCents != item.ExpectedUnitPriceCents {
				return ErrSalePriceChanged
			}
			if p.QuantityOnHand < item.Quantity {
				return ErrSaleStockUnavailable
			}
			id, err := uuid.NewRandom()
			if err != nil {
				return err
			}
			drafts = append(drafts, domain.ItemDraft{ID: id, ShopID: p.ShopID, ProductID: p.ID, ProductName: p.Name, Quantity: item.Quantity, UnitPriceCents: p.PriceCents, Currency: p.Currency})
		}
		now := uc.now().UTC()
		saleID, err = uuid.NewRandom()
		if err != nil {
			return err
		}
		sale, err := domain.NewSale(domain.SaleDraft{ID: saleID, ShopID: input.Scope.ShopID, RecordedByUserID: input.Scope.ActorID, ProfessionalID: professionalID, ProfessionalName: professionalName, AppointmentID: input.AppointmentID, Items: drafts}, now)
		if err != nil {
			return err
		}
		if err := tx.Sales.InsertSale(ctx, sale); err != nil {
			return err
		}
		stock := SaleStockCommand{ShopID: sale.ShopID, ActorID: sale.RecordedByUserID, OccurredAt: sale.SoldAt, Items: make([]SaleStockItem, 0, len(sale.Items))}
		for _, item := range sale.Items {
			stock.Items = append(stock.Items, SaleStockItem{SaleItemID: item.ID, ProductID: item.ProductID, Quantity: item.Quantity})
		}
		if err := tx.Stock.RecordSaleStock(ctx, stock); err != nil {
			return err
		}
		if err := tx.Commands.CompleteSaleCommand(ctx, input.Scope, key, sale.ID); err != nil {
			return err
		}
		if err := loadSaleOutput(ctx, tx, input.Scope, authority, sale.ID, false, &output); err != nil {
			return err
		}
		// Locks/writes may have crossed the paid deadline. No response escapes
		// before commit; an expired final check rolls back every sale/stock row.
		return tx.Eligibility.RequireNewSaleCoverage(ctx, input.Scope.ShopID, uc.now().UTC())
	})
	if err != nil {
		return SaleOutput{}, err
	}
	return output, nil
}

func loadSaleOutput(ctx context.Context, tx SalesTx, scope SalesScope, authority SalesAuthority, id uuid.UUID, replayed bool, out *SaleOutput) error {
	sale, found, err := tx.Sales.FindRecordedSale(ctx, scope.ShopID, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrSaleNotFound
	}
	if sale.ID != id || sale.ShopID != scope.ShopID || sale.RecordedByUserID != scope.ActorID {
		return ErrInconsistentSalesData
	}
	professional, err := authority.ProfessionalFor(scope, sale.ProfessionalID)
	if err != nil {
		return err
	}
	if professional != sale.ProfessionalID {
		return ErrSalesAccessDenied
	}
	*out = saleOutput(sale, replayed)
	return nil
}
