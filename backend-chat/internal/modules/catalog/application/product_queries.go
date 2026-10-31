package application

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type ProductQueries struct{ tx ProductTransactor }

func NewProductQueries(tx ProductTransactor) *ProductQueries { return &ProductQueries{tx} }

func productPageSize(size int) (int, error) {
	if size == 0 {
		return DefaultProductPageSize, nil
	}
	if size < 1 || size > MaxProductPageSize {
		return 0, ErrInvalidProductQuery
	}
	return size, nil
}

func (uc *ProductQueries) List(ctx context.Context, input ProductListInput) (ProductPage, error) {
	if !validProductScope(input.Scope) {
		return ProductPage{}, ErrProductAccessDenied
	}
	size, err := productPageSize(input.PageSize)
	if err != nil {
		return ProductPage{}, err
	}
	search := strings.TrimSpace(input.Search)
	if !utf8.ValidString(search) || utf8.RuneCountInString(search) > 100 || stringsHasControl(search) {
		return ProductPage{}, ErrInvalidProductQuery
	}
	if input.After != nil && (input.After.ProductID == uuid.Nil || input.After.CreatedAt.IsZero()) {
		return ProductPage{}, ErrInvalidProductQuery
	}
	out := ProductPage{Items: []ProductOutput{}}
	err = uc.tx.WithinProductTx(ctx, func(ctx context.Context, tx ProductTx) error {
		if input.IncludeInactive {
			err = tx.Access.RequireProductOwner(ctx, input.Scope)
		} else {
			err = tx.Access.RequireProductStaff(ctx, input.Scope)
		}
		if err != nil {
			return err
		}
		rows, err := tx.Reader.ListProducts(ctx, ProductQuery{ShopID: input.Scope.ShopID, Search: search, IncludeInactive: input.IncludeInactive, After: input.After, Limit: size + 1})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.ShopID != input.Scope.ShopID || row.ID == uuid.Nil || (!input.IncludeInactive && !row.Active) {
				return ErrInconsistentProductData
			}
		}
		if len(rows) > size {
			rows = rows[:size]
			last := rows[len(rows)-1]
			out.Next = &ProductCursor{CreatedAt: last.CreatedAt, ProductID: last.ID}
		}
		out.Items = append(out.Items, rows...)
		return nil
	})
	if err != nil {
		return ProductPage{}, err
	}
	return out, nil
}

func stringsHasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func (uc *ProductQueries) Movements(ctx context.Context, input StockMovementListInput) (StockMovementPage, error) {
	if !validProductScope(input.Scope) {
		return StockMovementPage{}, ErrProductAccessDenied
	}
	if input.ProductID == uuid.Nil {
		return StockMovementPage{}, ErrInvalidProductQuery
	}
	size, err := productPageSize(input.PageSize)
	if err != nil {
		return StockMovementPage{}, err
	}
	if input.After != nil && (input.After.MovementID == uuid.Nil || input.After.CreatedAt.IsZero()) {
		return StockMovementPage{}, ErrInvalidProductQuery
	}
	out := StockMovementPage{}
	err = uc.tx.WithinProductTx(ctx, func(ctx context.Context, tx ProductTx) error {
		if err := tx.Access.RequireProductOwner(ctx, input.Scope); err != nil {
			return err
		}
		if _, err := productOutput(ctx, tx, input.Scope.ShopID, input.ProductID); err != nil {
			return err
		}
		rows, err := tx.Reader.ListStockMovements(ctx, StockMovementQuery{ShopID: input.Scope.ShopID, ProductID: input.ProductID, After: input.After, Limit: size + 1})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.ShopID != input.Scope.ShopID || row.ProductID != input.ProductID {
				return ErrInconsistentProductData
			}
		}
		if len(rows) > size {
			rows = rows[:size]
			last := rows[len(rows)-1]
			out.Next = &StockMovementCursor{CreatedAt: last.CreatedAt, MovementID: last.ID}
		}
		out.Items = rows
		return nil
	})
	if err != nil {
		return StockMovementPage{}, err
	}
	return out, nil
}
