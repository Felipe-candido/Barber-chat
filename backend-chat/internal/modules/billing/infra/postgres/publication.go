package postgres

import (
	"context"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/google/uuid"
)

// Transaction-bound adapter; financial policy stays in billing's domain.
type PublicationCoverage struct {
	reader *Reader
	now    func() time.Time
}

func NewPublicationCoverage(reader *Reader, now func() time.Time) *PublicationCoverage {
	return &PublicationCoverage{reader: reader, now: now}
}

func (r *PublicationCoverage) ReadCommercialCoverage(ctx context.Context, id uuid.UUID) (shops.CommercialCoverage, error) {
	if _, err := r.reader.queries.LockBillingOperationAccount(ctx, id); err != nil {
		return shops.CommercialCoverage{}, err
	}
	state, found, err := r.reader.FindShopBillingState(ctx, id)
	if err != nil {
		return shops.CommercialCoverage{}, err
	}
	if !found || !state.Active || state.ShopID != id {
		return shops.CommercialCoverage{}, shops.ErrSettingsMismatch
	}
	entitlement, err := domain.EvaluateEntitlement(state.Subscription, r.now())
	if err != nil {
		return shops.CommercialCoverage{}, err
	}
	result := shops.CommercialCoverage{Status: "none", PlanKey: state.PlanKey, Allowed: entitlement.Allowed, Reason: string(entitlement.Reason)}
	if state.Subscription != nil {
		result.Status = string(state.Subscription.Status)
		result.CancelAtPeriodEnd = state.Subscription.CancelAtPeriodEnd
	}
	if !entitlement.Until.IsZero() {
		until := entitlement.Until.UTC()
		result.Until = &until
	}
	return result, nil
}
