package application

import (
	"context"
	"fmt"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type UpdateShopSettings struct{ transactions ShopSettingsTransactor }

func NewUpdateShopSettings(transactions ShopSettingsTransactor) *UpdateShopSettings {
	return &UpdateShopSettings{transactions: transactions}
}

func (uc *UpdateShopSettings) Execute(ctx context.Context, input UpdateShopSettingsInput) (ShopSettings, error) {
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil {
		return ShopSettings{}, ErrSettingsAccessDenied
	}
	// Match JSON's safely representable integers and reserve room for increment.
	if input.ExpectedVersion < 1 || input.ExpectedVersion >= 9007199254740991 {
		return ShopSettings{}, ErrInvalidSettingsVersion
	}
	settings, err := domain.NewBasicSettings(input.Settings)
	if err != nil {
		return ShopSettings{}, err
	}
	var output ShopSettings
	err = uc.transactions.WithinShopSettingsTx(ctx, func(ctx context.Context, tx ShopSettingsTx) error {
		if err := tx.Access.RequireSettingsOwner(ctx, input.Scope.ActorID, input.Scope.ShopID); err != nil {
			return err
		}
		changed, err := tx.Shops.ReplaceSettings(ctx, input.Scope.ShopID, input.ExpectedVersion, settings)
		if err != nil {
			return err
		}
		if !changed {
			if _, err := findSettings(ctx, tx.Shops, input.Scope.ShopID); err != nil {
				return err
			}
			return ErrSettingsConflict
		}
		output, err = findSettings(ctx, tx.Shops, input.Scope.ShopID)
		if err != nil {
			return err
		}
		if output.Version != input.ExpectedVersion+1 {
			return ErrSettingsMismatch
		}
		return nil
	})
	if err != nil {
		return ShopSettings{}, fmt.Errorf("update shop settings: %w", err)
	}
	return output, nil
}
