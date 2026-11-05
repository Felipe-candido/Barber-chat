package application

import (
	"context"
	"errors"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

var (
	ErrCompletionSettingsAccessDenied = errors.New("completion settings access denied")
	ErrInvalidCompletionSettings      = errors.New("invalid completion settings")
	ErrCompletionSettingsConflict     = errors.New("completion settings changed")
	ErrCompletionSettingsUnavailable  = errors.New("completion settings unavailable")
)

type CompletionSettingsScope struct{ ActorID, ShopID uuid.UUID }
type CompletionSettings struct {
	ShopID                    uuid.UUID
	Mode                      domain.CompletionMode
	Version                   int64
	AutomaticSince, UpdatedAt time.Time
}
type CompletionSettingsAccess interface {
	RequireCompletionSettingsAccess(context.Context, CompletionSettingsScope) error
}
type CompletionSettingsStore interface {
	Find(context.Context, uuid.UUID) (CompletionSettings, error)
	Replace(context.Context, uuid.UUID, int64, domain.CompletionMode) (bool, error)
}
type CompletionSettingsTx struct {
	Access CompletionSettingsAccess
	Store  CompletionSettingsStore
}
type CompletionSettingsTransactor interface {
	WithinCompletionSettingsTx(context.Context, func(context.Context, CompletionSettingsTx) error) error
}
type ManageCompletionSettings struct{ tx CompletionSettingsTransactor }

func NewManageCompletionSettings(tx CompletionSettingsTransactor) *ManageCompletionSettings {
	return &ManageCompletionSettings{tx}
}
func (u *ManageCompletionSettings) execute(ctx context.Context, s CompletionSettingsScope, change func(context.Context, CompletionSettingsStore) error) (CompletionSettings, error) {
	if s.ActorID == uuid.Nil || s.ShopID == uuid.Nil {
		return CompletionSettings{}, ErrCompletionSettingsAccessDenied
	}
	var result CompletionSettings
	err := u.tx.WithinCompletionSettingsTx(ctx, func(ctx context.Context, tx CompletionSettingsTx) error {
		if err := tx.Access.RequireCompletionSettingsAccess(ctx, s); err != nil {
			return err
		}
		if change != nil {
			if err := change(ctx, tx.Store); err != nil {
				return err
			}
		}
		var err error
		result, err = tx.Store.Find(ctx, s.ShopID)
		if err != nil {
			return err
		}
		if result.ShopID != s.ShopID || !result.Mode.Valid() || result.Version < 1 || result.Version > 9007199254740991 || result.UpdatedAt.IsZero() ||
			(result.Mode == domain.CompletionAutomatic && result.AutomaticSince.IsZero()) ||
			(result.Mode == domain.CompletionManual && !result.AutomaticSince.IsZero()) {
			return ErrCompletionSettingsUnavailable
		}
		return nil
	})
	return result, err
}
func (u *ManageCompletionSettings) Read(ctx context.Context, s CompletionSettingsScope) (CompletionSettings, error) {
	return u.execute(ctx, s, nil)
}
func (u *ManageCompletionSettings) Save(ctx context.Context, s CompletionSettingsScope, mode domain.CompletionMode, version int64) (CompletionSettings, error) {
	return u.execute(ctx, s, func(ctx context.Context, store CompletionSettingsStore) error {
		if !mode.Valid() || version < 1 || version >= 9007199254740991 {
			return ErrInvalidCompletionSettings
		}
		changed, err := store.Replace(ctx, s.ShopID, version, mode)
		if err != nil {
			return err
		}
		if !changed {
			return ErrCompletionSettingsConflict
		}
		return nil
	})
}
