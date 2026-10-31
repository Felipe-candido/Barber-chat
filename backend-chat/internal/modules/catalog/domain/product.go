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

// Bound integer cents to exact JSON/JavaScript integers as well as Go int64.
const MaxProductPriceCents int64 = 9_007_199_254_740_991

var (
	ErrInvalidProduct         = errors.New("invalid product")
	ErrProductVersionConflict = errors.New("product version changed")
)

type ProductDetails struct {
	Name, Description string
	PriceCents        int64
	Active            bool
}

// Persisted sales keep their own snapshots; editing this offer never edits them.
type Product struct {
	ID, ShopID uuid.UUID
	ProductDetails
	Currency             string
	Version              int64
	CreatedAt, UpdatedAt time.Time
}

func normalizeProductDetails(details ProductDetails) (ProductDetails, error) {
	details.Name = strings.TrimSpace(details.Name)
	details.Description = strings.TrimSpace(details.Description)
	if !utf8.ValidString(details.Name) || utf8.RuneCountInString(details.Name) < 1 || utf8.RuneCountInString(details.Name) > 100 ||
		strings.ContainsFunc(details.Name, unicode.IsControl) || !utf8.ValidString(details.Description) || utf8.RuneCountInString(details.Description) > 1000 ||
		strings.ContainsFunc(details.Description, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' }) ||
		details.PriceCents <= 0 || details.PriceCents > MaxProductPriceCents {
		return ProductDetails{}, ErrInvalidProduct
	}
	return details, nil
}

func NewProduct(shopID uuid.UUID, name, description string, priceCents int64, now time.Time) (Product, error) {
	if shopID == uuid.Nil || now.IsZero() {
		return Product{}, ErrInvalidProduct
	}
	details, err := normalizeProductDetails(ProductDetails{Name: name, Description: description, PriceCents: priceCents, Active: true})
	if err != nil {
		return Product{}, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Product{}, err
	}
	return Product{ID: id, ShopID: shopID, ProductDetails: details, Currency: "BRL", Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}, nil
}

func (p Product) Update(details ProductDetails, expectedVersion int64, now time.Time) (Product, error) {
	if p.ID == uuid.Nil || p.ShopID == uuid.Nil || p.Currency != "BRL" || p.CreatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) || now.IsZero() || now.Before(p.UpdatedAt) {
		return Product{}, ErrInvalidProduct
	}
	if expectedVersion < 1 || p.Version != expectedVersion || p.Version == math.MaxInt64 {
		return Product{}, ErrProductVersionConflict
	}
	normalized, err := normalizeProductDetails(details)
	if err != nil {
		return Product{}, err
	}
	if normalized == p.ProductDetails {
		return p, nil
	}
	p.ProductDetails, p.Version, p.UpdatedAt = normalized, p.Version+1, now.UTC()
	return p, nil
}
