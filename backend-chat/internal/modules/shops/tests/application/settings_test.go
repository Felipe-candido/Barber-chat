package application_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type settingsFake struct {
	current                                      app.ShopSettings
	actor, shop                                  uuid.UUID
	denied, conflict, missing, mismatchedVersion bool
	failure, commitErr                           error
	transactions, writes                         int
}

func (f *settingsFake) RequireSettingsOwner(_ context.Context, actor, shop uuid.UUID) error {
	f.actor, f.shop = actor, shop
	if f.denied {
		return app.ErrSettingsAccessDenied
	}
	return nil
}
func (f *settingsFake) FindSettings(context.Context, uuid.UUID) (app.ShopSettings, bool, error) {
	return f.current, !f.missing, f.failure
}
func (f *settingsFake) ReplaceSettings(_ context.Context, _ uuid.UUID, version int64, settings domain.BasicSettings) (bool, error) {
	if f.failure != nil {
		return false, f.failure
	}
	if f.conflict {
		return false, nil
	}
	f.writes++
	f.current.Name, f.current.Phone, f.current.Description, f.current.Address = settings.Name, settings.Phone, settings.Description, &settings.Address
	f.current.Version = version + 1
	if f.mismatchedVersion {
		f.current.Version++
	}
	return true, nil
}
func (f *settingsFake) WithinShopSettingsTx(ctx context.Context, fn func(context.Context, app.ShopSettingsTx) error) error {
	f.transactions++
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fn(ctx, app.ShopSettingsTx{Access: f, Shops: f}); err != nil {
		return err
	}
	return f.commitErr
}

func settingsInput() app.UpdateShopSettingsInput {
	return app.UpdateShopSettingsInput{Scope: app.SettingsScope{ActorID: uuid.New(), ShopID: uuid.New()}, ExpectedVersion: 1,
		Settings: domain.BasicSettings{Name: " Updated shop ", Phone: "(11) 91234-5678", Description: " Description ", Address: registrationAddress()}}
}

func TestGetAndUpdateShopSettings(t *testing.T) {
	input := settingsInput()
	f := &settingsFake{current: app.ShopSettings{ShopID: input.Scope.ShopID, Name: "Legacy", Slug: "legacy", Timezone: "America/Sao_Paulo", Version: 1}}
	get := app.NewGetShopSettings(f)
	legacy, err := get.Execute(context.Background(), input.Scope)
	if err != nil || legacy.Address != nil || f.actor != input.Scope.ActorID || f.shop != input.Scope.ShopID {
		t.Fatal(legacy, err)
	}
	got, err := app.NewUpdateShopSettings(f).Execute(context.Background(), input)
	if err != nil || got.Version != 2 || got.Name != "Updated shop" || got.Phone != "+5511912345678" || got.Address == nil || got.Slug != "legacy" || got.Timezone != "America/Sao_Paulo" {
		t.Fatal(got, err)
	}
	if f.writes != 1 {
		t.Fatal("unexpected writes", f.writes)
	}
}

func TestSettingsFailureNeverReportsSuccess(t *testing.T) {
	input := settingsInput()
	failure := errors.New("injected failure")
	for _, test := range []struct {
		name   string
		change func(*settingsFake)
		want   error
	}{
		{"denied", func(f *settingsFake) { f.denied = true }, app.ErrSettingsAccessDenied},
		{"conflict", func(f *settingsFake) { f.conflict = true }, app.ErrSettingsConflict},
		{"missing", func(f *settingsFake) { f.conflict, f.missing = true, true }, app.ErrSettingsNotFound},
		{"read identity mismatch", func(f *settingsFake) { f.current.ShopID = uuid.New() }, app.ErrSettingsMismatch},
		{"write version mismatch", func(f *settingsFake) { f.mismatchedVersion = true }, app.ErrSettingsMismatch},
		{"dependency", func(f *settingsFake) { f.failure = failure }, failure},
		{"commit", func(f *settingsFake) { f.commitErr = failure }, failure},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := &settingsFake{current: app.ShopSettings{ShopID: input.Scope.ShopID, Version: 1}}
			test.change(f)
			got, err := app.NewUpdateShopSettings(f).Execute(context.Background(), input)
			if !errors.Is(err, test.want) || got.ShopID != uuid.Nil {
				t.Fatal(got, err)
			}
			if f.denied && f.writes != 0 {
				t.Fatal("denied request wrote settings")
			}
		})
	}
	for _, change := range []func(*settingsFake){
		func(f *settingsFake) { f.denied = true }, func(f *settingsFake) { f.missing = true },
		func(f *settingsFake) { f.current.ShopID = uuid.New() }, func(f *settingsFake) { f.commitErr = failure },
	} {
		f := &settingsFake{current: app.ShopSettings{ShopID: input.Scope.ShopID, Version: 1}}
		change(f)
		got, err := app.NewGetShopSettings(f).Execute(context.Background(), input.Scope)
		if err == nil || got.ShopID != uuid.Nil {
			t.Fatal("read failure reported success", got, err)
		}
	}
}

func TestSettingsInvalidInputDoesNotOpenTransaction(t *testing.T) {
	for _, change := range []func(*app.UpdateShopSettingsInput){
		func(i *app.UpdateShopSettingsInput) { i.Scope.ActorID = uuid.Nil },
		func(i *app.UpdateShopSettingsInput) { i.Scope.ShopID = uuid.Nil },
		func(i *app.UpdateShopSettingsInput) { i.ExpectedVersion = 0 },
		func(i *app.UpdateShopSettingsInput) { i.ExpectedVersion = 9007199254740991 },
		func(i *app.UpdateShopSettingsInput) { i.Settings.Address = domain.Address{} },
	} {
		input := settingsInput()
		change(&input)
		f := &settingsFake{}
		if _, err := app.NewUpdateShopSettings(f).Execute(context.Background(), input); err == nil || f.transactions != 0 {
			t.Fatal(err, f.transactions)
		}
	}
}
