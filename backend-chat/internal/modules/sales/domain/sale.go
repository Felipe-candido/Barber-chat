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

const (
	MaxSaleItems            = 50
	MaxItemQuantity   int64 = math.MaxInt32
	MaxSaleTotalCents int64 = 9_007_199_254_740_991
)

var (
	ErrInvalidSale          = errors.New("invalid product sale")
	ErrInvalidSaleItem      = errors.New("invalid sale item")
	ErrInvalidSaleAmount    = errors.New("sale amount exceeds supported integer cents")
	ErrSaleVersionConflict  = errors.New("sale version changed")
	ErrSaleAlreadyCancelled = errors.New("sale was already cancelled with different instructions")
)

type SaleStatus string

const (
	SaleRecorded  SaleStatus = "recorded"
	SaleCancelled SaleStatus = "cancelled"
)

// ItemDraft is a trusted server-side product snapshot, not an HTTP request.
type ItemDraft struct {
	ID, ShopID, ProductID    uuid.UUID
	ProductName              string
	Quantity, UnitPriceCents int64
	Currency                 string
}

type SaleItem struct {
	ID, ProductID                            uuid.UUID
	ProductName                              string
	Quantity, UnitPriceCents, LineTotalCents int64
	Currency                                 string
}

type SaleDraft struct {
	ID, ShopID, RecordedByUserID  uuid.UUID
	ProfessionalID, AppointmentID uuid.UUID
	ProfessionalName              string
	Items                         []ItemDraft
}

type Cancellation struct {
	ActorID     uuid.UUID
	Reason      string
	Restock     bool
	CancelledAt time.Time
}

type Sale struct {
	ID, ShopID, RecordedByUserID  uuid.UUID
	ProfessionalID, AppointmentID uuid.UUID
	ProfessionalName              string
	Items                         []SaleItem
	Status                        SaleStatus
	TotalCents                    int64
	Currency                      string
	Version                       int64
	SoldAt, CreatedAt             time.Time
	Cancellation                  *Cancellation
}

func saleText(raw string, max int) (string, bool) {
	value := strings.TrimSpace(raw)
	return value, utf8.ValidString(value) && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= max && !strings.ContainsFunc(value, unicode.IsControl)
}

func NewSale(draft SaleDraft, now time.Time) (Sale, error) {
	if draft.ID == uuid.Nil || draft.ShopID == uuid.Nil || draft.RecordedByUserID == uuid.Nil || now.IsZero() || len(draft.Items) == 0 || len(draft.Items) > MaxSaleItems {
		return Sale{}, ErrInvalidSale
	}
	professionalName := strings.TrimSpace(draft.ProfessionalName)
	if draft.ProfessionalID == uuid.Nil {
		if professionalName != "" || draft.AppointmentID != uuid.Nil {
			return Sale{}, ErrInvalidSale
		}
	} else if _, ok := saleText(professionalName, 100); !ok {
		return Sale{}, ErrInvalidSale
	}
	items := make([]SaleItem, 0, len(draft.Items))
	products, ids := make(map[uuid.UUID]bool), make(map[uuid.UUID]bool)
	var total int64
	for _, raw := range draft.Items {
		name, ok := saleText(raw.ProductName, 100)
		if !ok || raw.ID == uuid.Nil || raw.ShopID != draft.ShopID || raw.ProductID == uuid.Nil || products[raw.ProductID] || ids[raw.ID] || raw.Quantity <= 0 || raw.Quantity > MaxItemQuantity || raw.Currency != "BRL" {
			return Sale{}, ErrInvalidSaleItem
		}
		if raw.UnitPriceCents <= 0 || raw.UnitPriceCents > MaxSaleTotalCents/raw.Quantity {
			return Sale{}, ErrInvalidSaleAmount
		}
		lineTotal := raw.UnitPriceCents * raw.Quantity
		if total > MaxSaleTotalCents-lineTotal {
			return Sale{}, ErrInvalidSaleAmount
		}
		total += lineTotal
		products[raw.ProductID], ids[raw.ID] = true, true
		items = append(items, SaleItem{ID: raw.ID, ProductID: raw.ProductID, ProductName: name, Quantity: raw.Quantity, UnitPriceCents: raw.UnitPriceCents, LineTotalCents: lineTotal, Currency: "BRL"})
	}
	return Sale{ID: draft.ID, ShopID: draft.ShopID, RecordedByUserID: draft.RecordedByUserID, ProfessionalID: draft.ProfessionalID, AppointmentID: draft.AppointmentID,
		ProfessionalName: professionalName, Items: items, Status: SaleRecorded, TotalCents: total, Currency: "BRL", Version: 1, SoldAt: now.UTC(), CreatedAt: now.UTC()}, nil
}

// This decides the transition only. The application must authorize the owner
// and persist cancellation + optional stock reversals in one transaction.
// A matching repeat returns the stored cancellation without a new version;
// stock writes are permitted only for a new transition, never for that replay.
func (s Sale) Cancel(actorID uuid.UUID, reason string, restock bool, expectedVersion int64, now time.Time) (Sale, error) {
	normalized, ok := saleText(reason, 500)
	if s.ID == uuid.Nil || s.ShopID == uuid.Nil || actorID == uuid.Nil || !ok || now.IsZero() || s.SoldAt.IsZero() || now.Before(s.SoldAt) {
		return Sale{}, ErrInvalidSale
	}
	if expectedVersion < 1 {
		return Sale{}, ErrSaleVersionConflict
	}
	if s.Status == SaleCancelled {
		if s.Cancellation == nil {
			return Sale{}, ErrInvalidSale
		}
		if s.Cancellation.Reason != normalized || s.Cancellation.Restock != restock {
			return Sale{}, ErrSaleAlreadyCancelled
		}
		return s, nil
	}
	if s.Status != SaleRecorded || s.Cancellation != nil {
		return Sale{}, ErrInvalidSale
	}
	if s.Version != expectedVersion || s.Version == math.MaxInt64 {
		return Sale{}, ErrSaleVersionConflict
	}
	s.Status, s.Version = SaleCancelled, s.Version+1
	s.Cancellation = &Cancellation{ActorID: actorID, Reason: normalized, Restock: restock, CancelledAt: now.UTC()}
	return s, nil
}
