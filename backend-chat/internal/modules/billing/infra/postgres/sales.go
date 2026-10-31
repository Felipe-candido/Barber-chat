package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SalesEligibility struct{ q *db.Queries }

func NewSalesEligibility(q *db.Queries) *SalesEligibility { return &SalesEligibility{q} }

var _ sales.SaleEligibility = (*SalesEligibility)(nil)

func (g *SalesEligibility) RequireNewSaleCoverage(ctx context.Context, shopID uuid.UUID, at time.Time) error {
	shop, err := g.q.LockSalesBillingShop(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.ErrSalesAccessDenied
	}
	if err != nil {
		return err
	}
	if !shop.Active {
		return sales.ErrSalesAccessDenied
	}
	if shop.ProvisioningSource == "manual" {
		return nil
	}
	if shop.ProvisioningSource != "self_service" {
		return sales.ErrInconsistentSalesData
	}
	if _, err := g.q.LockBillingOperationAccount(ctx, shopID); err != nil {
		return err
	}
	allowed, err := application.NewGetShopEntitlements(NewReader(g.q), application.ClockFunc(func() time.Time { return at })).CanOperateShop(ctx, shopID)
	if err != nil {
		return err
	}
	if !allowed {
		return sales.ErrSaleSubscriptionRequired
	}
	return nil
}
