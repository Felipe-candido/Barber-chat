package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

type ProductCommands struct {
	tx  ProductTransactor
	now func() time.Time
}

func NewProductCommands(tx ProductTransactor, now func() time.Time) *ProductCommands {
	if now == nil {
		now = time.Now
	}
	return &ProductCommands{tx, now}
}

func productCommandKey(raw string, operation ProductCommand, body any) (ProductCommandKey, error) {
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != raw {
		return ProductCommandKey{}, ErrInvalidProductCommandKey
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return ProductCommandKey{}, err
	}
	mac := hmac.New(sha256.New, decoded)
	_, _ = mac.Write(encoded)
	key := ProductCommandKey{Operation: operation, Digest: sha256.Sum256(decoded)}
	copy(key.Fingerprint[:], mac.Sum(nil))
	return key, nil
}

func validProductScope(scope ProductScope) bool {
	return scope.ActorID != uuid.Nil && scope.ShopID != uuid.Nil
}

func productOutput(ctx context.Context, tx ProductTx, shopID, productID uuid.UUID) (ProductOutput, error) {
	out, found, err := tx.Reader.FindProductOutput(ctx, shopID, productID)
	if err != nil {
		return ProductOutput{}, err
	}
	if !found {
		return ProductOutput{}, ErrProductNotFound
	}
	if out.ShopID != shopID || out.ID != productID {
		return ProductOutput{}, ErrInconsistentProductData
	}
	return out, nil
}

func (uc *ProductCommands) Create(ctx context.Context, input CreateProductInput) (ProductMutationOutput, error) {
	if !validProductScope(input.Scope) {
		return ProductMutationOutput{}, ErrProductAccessDenied
	}
	p, err := domain.NewProduct(input.Scope.ShopID, input.Name, input.Description, input.PriceCents, uc.now())
	if err != nil {
		return ProductMutationOutput{}, err
	}
	stock, err := domain.NewStockBalance(p.ShopID, p.ID, input.MinimumQuantity)
	if err != nil {
		return ProductMutationOutput{}, err
	}
	if input.InitialQuantity < 0 || input.InitialQuantity > domain.MaxStockQuantity {
		return ProductMutationOutput{}, domain.ErrInvalidStock
	}
	key, err := productCommandKey(input.IdempotencyKey, ProductCreateCommand, struct {
		Name, Description       string
		Price, Initial, Minimum int64
	}{p.Name, p.Description, p.PriceCents, input.InitialQuantity, input.MinimumQuantity})
	if err != nil {
		return ProductMutationOutput{}, err
	}
	var out ProductMutationOutput
	err = uc.tx.WithinProductTx(ctx, func(ctx context.Context, tx ProductTx) error {
		if err := tx.Access.RequireProductOwner(ctx, input.Scope); err != nil {
			return err
		}
		result, replayed, err := tx.Commands.ClaimProductCommand(ctx, input.Scope, key)
		if err != nil {
			return err
		}
		if !replayed {
			if err := tx.Products.InsertProduct(ctx, p); err != nil {
				return err
			}
			if err := tx.Stock.InsertStockBalance(ctx, stock); err != nil {
				return err
			}
			result.ProductID = p.ID
			if input.InitialQuantity > 0 {
				id, err := uuid.NewRandom()
				if err != nil {
					return err
				}
				movement, err := domain.NewStockMovement(domain.StockMovement{ID: id, ShopID: p.ShopID, ProductID: p.ID, ActorID: input.Scope.ActorID, Kind: domain.StockOpening, QuantityDelta: input.InitialQuantity, Reason: "Initial stock"}, p.CreatedAt)
				if err != nil {
					return err
				}
				if err := tx.Stock.AppendStockMovement(ctx, movement); err != nil {
					return err
				}
				result.MovementID = id
			}
			if err := tx.Commands.CompleteProductCommand(ctx, input.Scope, key, result); err != nil {
				return err
			}
		}
		product, err := productOutput(ctx, tx, input.Scope.ShopID, result.ProductID)
		if err != nil {
			return err
		}
		out = ProductMutationOutput{Product: product, MovementID: result.MovementID, Replayed: replayed}
		return nil
	})
	if err != nil {
		return ProductMutationOutput{}, err
	}
	return out, nil
}

func (uc *ProductCommands) RecordStock(ctx context.Context, input RecordStockInput) (ProductMutationOutput, error) {
	if !validProductScope(input.Scope) {
		return ProductMutationOutput{}, ErrProductAccessDenied
	}
	if input.Operation != StockReceive && input.Operation != StockAdjust {
		return ProductMutationOutput{}, domain.ErrInvalidStock
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return ProductMutationOutput{}, err
	}
	m, err := domain.NewStockMovement(domain.StockMovement{ID: id, ShopID: input.Scope.ShopID, ProductID: input.ProductID, ActorID: input.Scope.ActorID, Kind: domain.StockMovementKind(input.Operation), QuantityDelta: input.QuantityDelta, Reason: input.Reason}, uc.now())
	if err != nil {
		return ProductMutationOutput{}, err
	}
	key, err := productCommandKey(input.IdempotencyKey, StockRecordCommand, struct {
		ProductID uuid.UUID
		Kind      domain.StockMovementKind
		Delta     int64
		Reason    string
	}{input.ProductID, m.Kind, m.QuantityDelta, m.Reason})
	if err != nil {
		return ProductMutationOutput{}, err
	}
	var out ProductMutationOutput
	err = uc.tx.WithinProductTx(ctx, func(ctx context.Context, tx ProductTx) error {
		if err := tx.Access.RequireProductOwner(ctx, input.Scope); err != nil {
			return err
		}
		result, replayed, err := tx.Commands.ClaimProductCommand(ctx, input.Scope, key)
		if err != nil {
			return err
		}
		if !replayed {
			p, found, err := tx.Products.LockProduct(ctx, input.Scope.ShopID, input.ProductID)
			if err != nil {
				return err
			}
			if !found {
				return ErrProductNotFound
			}
			if p.ShopID != input.Scope.ShopID || p.ID != input.ProductID {
				return ErrInconsistentProductData
			}
			b, found, err := tx.Stock.LockStockBalance(ctx, p.ShopID, p.ID)
			if err != nil {
				return err
			}
			if !found {
				return ErrInconsistentProductData
			}
			if _, err := b.Apply(m); err != nil {
				return err
			}
			if err := tx.Stock.AppendStockMovement(ctx, m); err != nil {
				return err
			}
			result = ProductCommandResult{ProductID: p.ID, MovementID: m.ID}
			if err := tx.Commands.CompleteProductCommand(ctx, input.Scope, key, result); err != nil {
				return err
			}
		}
		product, err := productOutput(ctx, tx, input.Scope.ShopID, result.ProductID)
		if err != nil {
			return err
		}
		out = ProductMutationOutput{Product: product, MovementID: result.MovementID, Replayed: replayed}
		return nil
	})
	if err != nil {
		return ProductMutationOutput{}, err
	}
	return out, nil
}

func (uc *ProductCommands) Update(ctx context.Context, input UpdateProductInput) (ProductOutput, error) {
	if !validProductScope(input.Scope) {
		return ProductOutput{}, ErrProductAccessDenied
	}
	if input.ProductID == uuid.Nil {
		return ProductOutput{}, domain.ErrInvalidProduct
	}
	var out ProductOutput
	err := uc.tx.WithinProductTx(ctx, func(ctx context.Context, tx ProductTx) error {
		if err := tx.Access.RequireProductOwner(ctx, input.Scope); err != nil {
			return err
		}
		p, found, err := tx.Products.LockProduct(ctx, input.Scope.ShopID, input.ProductID)
		if err != nil {
			return err
		}
		if !found {
			return ErrProductNotFound
		}
		if p.ShopID != input.Scope.ShopID || p.ID != input.ProductID {
			return ErrInconsistentProductData
		}
		next, err := p.Update(input.Details, input.ExpectedVersion, uc.now())
		if err != nil {
			return err
		}
		if next.Version != p.Version {
			ok, err := tx.Products.UpdateProduct(ctx, next, p.Version)
			if err != nil {
				return err
			}
			if !ok {
				return domain.ErrProductVersionConflict
			}
		}
		out, err = productOutput(ctx, tx, p.ShopID, p.ID)
		return err
	})
	if err != nil {
		return ProductOutput{}, err
	}
	return out, nil
}

func (uc *ProductCommands) SetMinimum(ctx context.Context, input UpdateStockMinimumInput) (ProductOutput, error) {
	if !validProductScope(input.Scope) {
		return ProductOutput{}, ErrProductAccessDenied
	}
	if input.ProductID == uuid.Nil {
		return ProductOutput{}, domain.ErrInvalidStock
	}
	var out ProductOutput
	err := uc.tx.WithinProductTx(ctx, func(ctx context.Context, tx ProductTx) error {
		if err := tx.Access.RequireProductOwner(ctx, input.Scope); err != nil {
			return err
		}
		p, found, err := tx.Products.LockProduct(ctx, input.Scope.ShopID, input.ProductID)
		if err != nil {
			return err
		}
		if !found {
			return ErrProductNotFound
		}
		if p.ShopID != input.Scope.ShopID || p.ID != input.ProductID {
			return ErrInconsistentProductData
		}
		b, found, err := tx.Stock.LockStockBalance(ctx, p.ShopID, p.ID)
		if err != nil {
			return err
		}
		if !found {
			return ErrInconsistentProductData
		}
		next, err := b.SetMinimum(input.MinimumQuantity, input.ExpectedStockVersion)
		if err != nil {
			return err
		}
		if next.Version != b.Version {
			ok, err := tx.Stock.UpdateStockMinimum(ctx, p.ShopID, p.ID, next.MinimumQuantity, b.Version)
			if err != nil {
				return err
			}
			if !ok {
				return domain.ErrStockVersionConflict
			}
		}
		out, err = productOutput(ctx, tx, p.ShopID, p.ID)
		return err
	})
	if err != nil {
		return ProductOutput{}, err
	}
	return out, nil
}
