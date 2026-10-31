package domain_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

func TestProductValidationAndVersionedEdits(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.FixedZone("shop", -3*3600))
	p, err := domain.NewProduct(uuid.New(), "  Pomada  ", "  Efeito seco  ", 3000, now)
	if err != nil || p.ID == uuid.Nil || p.Name != "Pomada" || p.Description != "Efeito seco" || p.Version != 1 || !p.Active || p.Currency != "BRL" || p.CreatedAt.Location() != time.UTC {
		t.Fatal("invalid initial product", err)
	}
	details := p.ProductDetails
	details.PriceCents, details.Active = 3500, false
	updated, err := p.Update(details, 1, now.Add(time.Minute))
	if err != nil || updated.PriceCents != 3500 || updated.Active || updated.Version != 2 || updated.ID != p.ID || p.PriceCents != 3000 || !p.Active {
		t.Fatal("edit changed identity/original", err)
	}
	if _, err := updated.Update(p.ProductDetails, 1, now.Add(time.Minute)); !errors.Is(err, domain.ErrProductVersionConflict) {
		t.Fatal("stale edit accepted", err)
	}
	noChange, err := updated.Update(updated.ProductDetails, 2, now.Add(2*time.Minute))
	if err != nil || noChange.Version != 2 {
		t.Fatal("no-op edit created another version", err)
	}
	if _, err := updated.Update(details, 2, now); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatal("backdated edit accepted", err)
	}
	updated.Version = math.MaxInt64
	if _, err := updated.Update(p.ProductDetails, math.MaxInt64, now.Add(time.Minute)); !errors.Is(err, domain.ErrProductVersionConflict) {
		t.Fatal("version overflow accepted", err)
	}
}

func TestProductRejectsInvalidDetails(t *testing.T) {
	now, shop := time.Now(), uuid.New()
	for _, tc := range []struct {
		name, description string
		price             int64
	}{
		{" ", "", 3000}, {strings.Repeat("é", 101), "", 3000}, {"Pom\nada", "", 3000}, {string([]byte{0xff}), "", 3000},
		{"Pomada", string([]byte{0xff}), 3000}, {"Pomada", strings.Repeat("a", 1001), 3000}, {"Pomada", "invalid\x00", 3000},
		{"Pomada", "", 0}, {"Pomada", "", -1}, {"Pomada", "", math.MaxInt64},
	} {
		if _, err := domain.NewProduct(shop, tc.name, tc.description, tc.price, now); !errors.Is(err, domain.ErrInvalidProduct) {
			t.Fatal("invalid product accepted", err)
		}
	}
	if _, err := domain.NewProduct(uuid.Nil, "Pomada", "", 3000, now); err == nil {
		t.Fatal("missing tenant accepted")
	}
	if _, err := domain.NewProduct(shop, "Pomada", "", 3000, time.Time{}); err == nil {
		t.Fatal("missing time accepted")
	}
	if _, err := domain.NewProduct(shop, strings.Repeat("é", 100), "Two lines\nDescription", domain.MaxProductPriceCents, now); err != nil {
		t.Fatal("valid boundary rejected", err)
	}
}
