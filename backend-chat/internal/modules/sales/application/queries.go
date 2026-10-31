package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/google/uuid"
)

type SaleQueries struct{ tx SalesTransactor }

func NewSaleQueries(tx SalesTransactor) *SaleQueries { return &SaleQueries{tx} }

func (uc *SaleQueries) Get(ctx context.Context, input GetSaleInput) (SaleOutput, error) {
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil {
		return SaleOutput{}, ErrSalesAccessDenied
	}
	if input.SaleID == uuid.Nil {
		return SaleOutput{}, ErrInvalidSalesQuery
	}
	var out SaleOutput
	err := uc.tx.WithinSalesTx(ctx, func(ctx context.Context, tx SalesTx) error {
		authority, err := tx.Access.RequireSalesAccess(ctx, input.Scope)
		if err != nil {
			return err
		}
		professional, err := authority.ProfessionalFor(input.Scope, uuid.Nil)
		if err != nil {
			return err
		}
		sale, found, err := tx.Reader.FindSale(ctx, input.Scope.ShopID, input.SaleID, professional)
		if err != nil {
			return err
		}
		if !found {
			return ErrSaleNotFound
		}
		if professional != uuid.Nil && professional != sale.ProfessionalID {
			return ErrSaleNotFound
		}
		if sale.ID != input.SaleID || sale.ShopID != input.Scope.ShopID {
			return ErrInconsistentSalesData
		}
		out = saleOutput(sale, false)
		return nil
	})
	if err != nil {
		return SaleOutput{}, err
	}
	return out, nil
}

// A cursor is a page position, not an authorization credential. Its binding
// prevents accidental reuse with a different actor, tenant or filter set.
type boundSaleCursor struct {
	Binding  [32]byte
	Position SaleCursor
}

func (uc *SaleQueries) List(ctx context.Context, input ListSalesInput) (SalePage, error) {
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil {
		return SalePage{}, ErrSalesAccessDenied
	}
	if input.PageSize == 0 {
		input.PageSize = 25
	}
	if input.PageSize < 1 || input.PageSize > 100 || len(input.Cursor) > 1024 || (input.Status != "" && input.Status != domain.SaleRecorded && input.Status != domain.SaleCancelled) {
		return SalePage{}, ErrInvalidSalesQuery
	}
	out := SalePage{Items: []SaleOutput{}}
	err := uc.tx.WithinSalesTx(ctx, func(ctx context.Context, tx SalesTx) error {
		authority, err := tx.Access.RequireSalesAccess(ctx, input.Scope)
		if err != nil {
			return err
		}
		professional, err := authority.ProfessionalFor(input.Scope, input.ProfessionalID)
		if err != nil {
			return err
		}
		zone, err := tx.Calendar.ShopTimezone(ctx, input.Scope.ShopID)
		if err != nil {
			return err
		}
		if zone == "" || zone == "Local" {
			return ErrInconsistentSalesData
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return ErrInconsistentSalesData
		}
		from, err := time.ParseInLocation(time.DateOnly, input.From, loc)
		if err != nil || from.Format(time.DateOnly) != input.From || from.Year() < 1900 || from.Year() > 9998 {
			return ErrInvalidSalesQuery
		}
		through, err := time.ParseInLocation(time.DateOnly, input.Through, loc)
		if err != nil || through.Format(time.DateOnly) != input.Through || through.Year() > 9998 || through.Before(from) || through.After(from.AddDate(1, 0, 0).AddDate(0, 0, -1)) {
			return ErrInvalidSalesQuery
		}
		until := through.AddDate(0, 0, 1)
		bindingData, err := json.Marshal(struct {
			Protocol     int
			Scope        SalesScope
			Professional uuid.UUID
			From, Until  time.Time
			Status       domain.SaleStatus
			Timezone     string
		}{1, input.Scope, professional, from.UTC(), until.UTC(), input.Status, zone})
		if err != nil {
			return ErrInvalidSalesQuery
		}
		binding := sha256.Sum256(bindingData)
		query := SaleListQuery{ShopID: input.Scope.ShopID, ProfessionalID: professional, From: from.UTC(), Until: until.UTC(), Status: input.Status, Limit: input.PageSize + 1}
		if input.Cursor != "" {
			data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
			var cursor boundSaleCursor
			if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Binding != binding || cursor.Position.SaleID == uuid.Nil || cursor.Position.SoldAt.IsZero() || cursor.Position.SoldAt.Before(query.From) || !cursor.Position.SoldAt.Before(query.Until) {
				return ErrInvalidSalesQuery
			}
			query.After = &cursor.Position
		}
		rows, err := tx.Reader.ListSales(ctx, query)
		if err != nil {
			return err
		}
		if len(rows) > input.PageSize {
			rows = rows[:input.PageSize]
			last := rows[len(rows)-1]
			data, err := json.Marshal(boundSaleCursor{binding, SaleCursor{SoldAt: last.SoldAt, SaleID: last.ID}})
			if err != nil {
				return err
			}
			out.NextCursor = base64.RawURLEncoding.EncodeToString(data)
		}
		for _, sale := range rows {
			if sale.ShopID != input.Scope.ShopID || sale.ID == uuid.Nil || (professional != uuid.Nil && sale.ProfessionalID != professional) || (input.Status != "" && sale.Status != input.Status) || sale.SoldAt.Before(query.From) || !sale.SoldAt.Before(query.Until) {
				return ErrInconsistentSalesData
			}
			out.Items = append(out.Items, saleOutput(sale, false))
		}
		out.Timezone = zone
		return nil
	})
	if err != nil {
		return SalePage{}, err
	}
	return out, nil
}
