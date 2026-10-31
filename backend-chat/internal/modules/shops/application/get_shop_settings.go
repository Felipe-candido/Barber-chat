package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type GetShopSettings struct{ transactions ShopSettingsTransactor }

func NewGetShopSettings(transactions ShopSettingsTransactor) *GetShopSettings {
	return &GetShopSettings{transactions: transactions}
}

func (uc *GetShopSettings) Execute(ctx context.Context, scope SettingsScope) (ShopSettings, error) {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil {
		return ShopSettings{}, ErrSettingsAccessDenied
	}
	var output ShopSettings
	err := uc.transactions.WithinShopSettingsTx(ctx, func(ctx context.Context, tx ShopSettingsTx) error {
		if err := tx.Access.RequireSettingsOwner(ctx, scope.ActorID, scope.ShopID); err != nil {
			return err
		}
		settings, err := findSettings(ctx, tx.Shops, scope.ShopID)
		output = settings
		return err
	})
	if err != nil {
		return ShopSettings{}, fmt.Errorf("get shop settings: %w", err)
	}
	return output, nil
}

func findSettings(ctx context.Context, store ShopSettingsStore, shopID uuid.UUID) (ShopSettings, error) {
	settings, found, err := store.FindSettings(ctx, shopID)
	if err != nil {
		return ShopSettings{}, err
	}
	if !found {
		return ShopSettings{}, ErrSettingsNotFound
	}
	if settings.ShopID != shopID || settings.Version < 1 {
		return ShopSettings{}, ErrSettingsMismatch
	}
	return settings, nil
}
