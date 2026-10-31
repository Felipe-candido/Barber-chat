package application

import (
	"context"
	"fmt"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type ShopPublication struct{ transactions PublicationTransactor }

func NewShopPublication(transactions PublicationTransactor) *ShopPublication {
	return &ShopPublication{transactions: transactions}
}

func (uc *ShopPublication) Get(ctx context.Context, scope SettingsScope) (ShopCommercialStatus, error) {
	var output ShopCommercialStatus
	err := uc.withOwner(ctx, scope, func(ctx context.Context, tx PublicationTx) error {
		shop, err := publicationShop(ctx, tx.Shops, scope.ShopID)
		if err != nil {
			return err
		}
		output, err = commercialStatus(ctx, tx, shop)
		return err
	})
	if err != nil {
		return ShopCommercialStatus{}, fmt.Errorf("get commercial status: %w", err)
	}
	return output, nil
}

// An explicit desired state, not a toggle. A repeat cannot reverse publication.
// Unpublishing deliberately does not require payment or configuration readiness.
func (uc *ShopPublication) Set(ctx context.Context, scope SettingsScope, requested bool) error {
	return uc.withOwner(ctx, scope, func(ctx context.Context, tx PublicationTx) error {
		if err := tx.Shops.LockPublicationShop(ctx, scope.ShopID); err != nil {
			return err
		}
		shop, err := publicationShop(ctx, tx.Shops, scope.ShopID)
		if err != nil {
			return err
		}
		if !shop.SelfService {
			return ErrPublicationNotManaged
		}
		if requested {
			status, err := commercialStatus(ctx, tx, shop)
			if err != nil {
				return err
			}
			if !status.CanPublish {
				return ErrPublicationBlocked
			}
		}
		return tx.Shops.SetPublicationRequested(ctx, scope.ShopID, requested)
	})
}

func (uc *ShopPublication) withOwner(ctx context.Context, scope SettingsScope, fn func(context.Context, PublicationTx) error) error {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil {
		return ErrSettingsAccessDenied
	}
	return uc.transactions.WithinPublicationTx(ctx, func(ctx context.Context, tx PublicationTx) error {
		if err := tx.Access.RequireSettingsOwner(ctx, scope.ActorID, scope.ShopID); err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

func publicationShop(ctx context.Context, store PublicationStore, id uuid.UUID) (PublicationShop, error) {
	shop, found, err := store.FindPublicationShop(ctx, id)
	if err != nil {
		return PublicationShop{}, err
	}
	if !found {
		return PublicationShop{}, ErrSettingsNotFound
	}
	if shop.ShopID != id {
		return PublicationShop{}, ErrSettingsMismatch
	}
	return shop, nil
}

func commercialStatus(ctx context.Context, tx PublicationTx, shop PublicationShop) (ShopCommercialStatus, error) {
	result := ShopCommercialStatus{ShopID: shop.ShopID, Slug: shop.Slug, PublicationManaged: shop.SelfService,
		Requested: shop.Requested, Blockers: []string{}}
	// Legacy behavior is deliberate. Do not silently enroll or hide manual shops.
	if !shop.SelfService {
		result.PublicBookingEnabled = true
		result.Subscription = CommercialCoverage{Status: "not_required", Allowed: true, Reason: "legacy_access"}
		result.Blockers = append(result.Blockers, "operator_managed")
		return result, nil
	}
	coverage, err := tx.Billing.ReadCommercialCoverage(ctx, shop.ShopID)
	if err != nil {
		return ShopCommercialStatus{}, err
	}
	result.Subscription = coverage
	result.PublicBookingEnabled = (domain.Publication{Requested: shop.Requested}).PublicBookingEnabled(true, coverage.Allowed)
	if shop.Address != nil {
		_, err := domain.NewAddress(*shop.Address)
		result.Readiness.Address = err == nil
	}
	catalog, err := tx.Catalog.ReadPublicationCatalog(ctx, shop.ShopID)
	if err != nil {
		return ShopCommercialStatus{}, err
	}
	result.Readiness.Service = catalog.HasService
	result.Readiness.Professional = len(catalog.Candidates) > 0
	if len(catalog.Candidates) > 0 {
		result.Readiness.Schedule, err = tx.Schedule.HasPublicationSchedule(ctx, shop.ShopID, catalog.Candidates)
		if err != nil {
			return ShopCommercialStatus{}, err
		}
	}
	if !coverage.Allowed {
		result.Blockers = append(result.Blockers, coverage.Reason)
	}
	for _, check := range []struct {
		ready  bool
		reason string
	}{
		{result.Readiness.Address, "address_required"}, {result.Readiness.Service, "service_required"},
		{result.Readiness.Professional, "professional_required"}, {result.Readiness.Schedule, "schedule_required"},
	} {
		if !check.ready {
			result.Blockers = append(result.Blockers, check.reason)
		}
	}
	result.CanPublish = len(result.Blockers) == 0
	return result, nil
}
