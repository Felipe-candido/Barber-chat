package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
)

func saleDraft() domain.SaleDraft {
	shopID := uuid.New()
	return domain.SaleDraft{ID: uuid.New(), ShopID: shopID, RecordedByUserID: uuid.New(), Items: []domain.ItemDraft{
		{ID: uuid.New(), ShopID: shopID, ProductID: uuid.New(), ProductName: "  Pomada  ", Quantity: 3, UnitPriceCents: 3000, Currency: "BRL"},
		{ID: uuid.New(), ShopID: shopID, ProductID: uuid.New(), ProductName: "Shampoo", Quantity: 1, UnitPriceCents: 4000, Currency: "BRL"},
	}}
}

func TestSaleComputesIntegerTotalsAndPreservesSnapshots(t *testing.T) {
	draft := saleDraft()
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.FixedZone("shop", -3*3600))
	sale, err := domain.NewSale(draft, now)
	if err != nil || sale.TotalCents != 13000 || sale.Items[0].LineTotalCents != 9000 || sale.Items[0].ProductName != "Pomada" || sale.Version != 1 || sale.Status != domain.SaleRecorded || sale.SoldAt.Location() != time.UTC {
		t.Fatal("wrong recorded sale", err)
	}
	draft.Items[0].UnitPriceCents, draft.Items[0].ProductName = 5000, "New catalog name"
	if sale.Items[0].UnitPriceCents != 3000 || sale.Items[0].ProductName != "Pomada" || sale.TotalCents != 13000 {
		t.Fatal("sale shares its input slice")
	}
	if sale.ProfessionalID != uuid.Nil || sale.AppointmentID != uuid.Nil {
		t.Fatal("standalone sale created a mandatory appointment/professional")
	}
}

func TestSaleRejectsDuplicateItemsAndUnsafeAmounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*domain.SaleDraft)
		want   error
	}{
		{"missing tenant", func(d *domain.SaleDraft) { d.ShopID = uuid.Nil }, domain.ErrInvalidSale},
		{"missing actor", func(d *domain.SaleDraft) { d.RecordedByUserID = uuid.Nil }, domain.ErrInvalidSale},
		{"empty items", func(d *domain.SaleDraft) { d.Items = nil }, domain.ErrInvalidSale},
		{"same product twice", func(d *domain.SaleDraft) { d.Items[1].ProductID = d.Items[0].ProductID }, domain.ErrInvalidSaleItem},
		{"same item ID twice", func(d *domain.SaleDraft) { d.Items[1].ID = d.Items[0].ID }, domain.ErrInvalidSaleItem},
		{"missing product", func(d *domain.SaleDraft) { d.Items[0].ProductID = uuid.Nil }, domain.ErrInvalidSaleItem},
		{"foreign product tenant", func(d *domain.SaleDraft) { d.Items[0].ShopID = uuid.New() }, domain.ErrInvalidSaleItem},
		{"zero quantity", func(d *domain.SaleDraft) { d.Items[0].Quantity = 0 }, domain.ErrInvalidSaleItem},
		{"too much quantity", func(d *domain.SaleDraft) { d.Items[0].Quantity = math.MaxInt64 }, domain.ErrInvalidSaleItem},
		{"currency mismatch", func(d *domain.SaleDraft) { d.Items[0].Currency = "USD" }, domain.ErrInvalidSaleItem},
		{"missing name", func(d *domain.SaleDraft) { d.Items[0].ProductName = " " }, domain.ErrInvalidSaleItem},
		{"zero price", func(d *domain.SaleDraft) { d.Items[0].UnitPriceCents = 0 }, domain.ErrInvalidSaleAmount},
		{"unsafe multiplication", func(d *domain.SaleDraft) { d.Items[0].UnitPriceCents = math.MaxInt64 }, domain.ErrInvalidSaleAmount},
		{"unsafe total", func(d *domain.SaleDraft) {
			d.Items[0].Quantity = 1
			d.Items[0].UnitPriceCents = domain.MaxSaleTotalCents
		}, domain.ErrInvalidSaleAmount},
		{"appointment without professional", func(d *domain.SaleDraft) { d.AppointmentID = uuid.New() }, domain.ErrInvalidSale},
		{"professional without snapshot", func(d *domain.SaleDraft) { d.ProfessionalID = uuid.New() }, domain.ErrInvalidSale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := saleDraft()
			tc.mutate(&draft)
			if _, err := domain.NewSale(draft, time.Now()); !errors.Is(err, tc.want) {
				t.Fatal("invalid draft accepted", err)
			}
		})
	}
	draft := saleDraft()
	draft.Items = draft.Items[:1]
	draft.Items[0].Quantity = 1
	draft.Items[0].UnitPriceCents = domain.MaxSaleTotalCents
	if _, err := domain.NewSale(draft, time.Now()); err != nil {
		t.Fatal("exact supported boundary rejected", err)
	}
	draft = saleDraft()
	draft.ProfessionalID, draft.AppointmentID, draft.ProfessionalName = uuid.New(), uuid.New(), " Felipe "
	if sale, err := domain.NewSale(draft, time.Now()); err != nil || sale.ProfessionalName != "Felipe" {
		t.Fatal("valid professional link rejected", err)
	}
}

func TestSaleCancellationIsExplicitAndDoesNotMutateTheOriginal(t *testing.T) {
	now := time.Now()
	sale, err := domain.NewSale(saleDraft(), now)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	for _, restock := range []bool{true, false} {
		cancelled, err := sale.Cancel(actor, "  Incorrect record  ", restock, 1, now.Add(time.Minute))
		if err != nil || cancelled.Status != domain.SaleCancelled || cancelled.Version != 2 || cancelled.TotalCents != sale.TotalCents || cancelled.Cancellation.Restock != restock || cancelled.Cancellation.Reason != "Incorrect record" || sale.Status != domain.SaleRecorded || sale.Cancellation != nil {
			t.Fatal("incorrect cancellation", err)
		}
		replayed, err := cancelled.Cancel(actor, "Incorrect record", restock, 1, now.Add(2*time.Minute))
		if err != nil || replayed.Version != 2 || !replayed.Cancellation.CancelledAt.Equal(cancelled.Cancellation.CancelledAt) {
			t.Fatal("repeat changed cancellation", err)
		}
		if _, err := cancelled.Cancel(actor, "Incorrect record", !restock, 2, now.Add(time.Minute)); !errors.Is(err, domain.ErrSaleAlreadyCancelled) {
			t.Fatal("repeat changed restock instructions", err)
		}
	}
	if _, err := sale.Cancel(actor, "Wrong record", true, 2, now.Add(time.Minute)); !errors.Is(err, domain.ErrSaleVersionConflict) {
		t.Fatal("stale version accepted", err)
	}
	if _, err := sale.Cancel(actor, " ", true, 1, now.Add(time.Minute)); !errors.Is(err, domain.ErrInvalidSale) {
		t.Fatal("missing reason accepted", err)
	}
	if _, err := sale.Cancel(actor, "Wrong record", true, 1, now.Add(-time.Minute)); !errors.Is(err, domain.ErrInvalidSale) {
		t.Fatal("backdated cancellation accepted", err)
	}
}
