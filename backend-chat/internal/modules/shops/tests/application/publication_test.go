package application_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type publicationFake struct {
	shop                          app.PublicationShop
	coverage                      app.CommercialCoverage
	catalog                       app.PublicationCatalog
	schedule                      bool
	accessErr, readErr, commitErr error
	writes, financialReads        int
	missing                       bool
}

func (f *publicationFake) RequireSettingsOwner(context.Context, uuid.UUID, uuid.UUID) error {
	return f.accessErr
}
func (f *publicationFake) LockPublicationShop(context.Context, uuid.UUID) error { return nil }
func (f *publicationFake) FindPublicationShop(context.Context, uuid.UUID) (app.PublicationShop, bool, error) {
	return f.shop, !f.missing, nil
}
func (f *publicationFake) SetPublicationRequested(_ context.Context, _ uuid.UUID, value bool) error {
	f.writes++
	f.shop.Requested = value
	return nil
}
func (f *publicationFake) ReadCommercialCoverage(context.Context, uuid.UUID) (app.CommercialCoverage, error) {
	f.financialReads++
	return f.coverage, f.readErr
}
func (f *publicationFake) ReadPublicationCatalog(context.Context, uuid.UUID) (app.PublicationCatalog, error) {
	return f.catalog, nil
}
func (f *publicationFake) HasPublicationSchedule(context.Context, uuid.UUID, []app.PublicationCandidate) (bool, error) {
	return f.schedule, nil
}
func (f *publicationFake) WithinPublicationTx(ctx context.Context, fn func(context.Context, app.PublicationTx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fn(ctx, app.PublicationTx{Access: f, Shops: f, Catalog: f, Schedule: f, Billing: f}); err != nil {
		return err
	}
	return f.commitErr
}

func readyPublication() (*publicationFake, app.SettingsScope) {
	scope := app.SettingsScope{ActorID: uuid.New(), ShopID: uuid.New()}
	address := registrationAddress()
	return &publicationFake{shop: app.PublicationShop{ShopID: scope.ShopID, Slug: "shop", SelfService: true, Address: &address},
		coverage: app.CommercialCoverage{Allowed: true, Status: "active", Reason: "subscribed"},
		catalog:  app.PublicationCatalog{HasService: true, Candidates: []app.PublicationCandidate{{ProfessionalID: uuid.New(), DurationMinutes: 40}}}, schedule: true}, scope
}

func TestPublicationSeparatesPaymentAndIntent(t *testing.T) {
	f, scope := readyPublication()
	uc := app.NewShopPublication(f)
	status, err := uc.Get(context.Background(), scope)
	if err != nil || !status.CanPublish || status.PublicBookingEnabled || f.writes != 0 {
		t.Fatal(status, err)
	}
	for range 2 {
		if err := uc.Set(context.Background(), scope, true); err != nil {
			t.Fatal(err)
		}
	}
	status, err = uc.Get(context.Background(), scope)
	if err != nil || !status.PublicBookingEnabled || !f.shop.Requested {
		t.Fatal(status, err)
	}
	// Loss of coverage hides booking but preserves the owner's saved intent.
	f.coverage = app.CommercialCoverage{Reason: "payment_required"}
	status, err = uc.Get(context.Background(), scope)
	if err != nil || status.PublicBookingEnabled || !status.Requested {
		t.Fatal(status, err)
	}
	f.readErr = errors.New("financial dependency unavailable")
	if err := uc.Set(context.Background(), scope, false); err != nil || f.shop.Requested {
		t.Fatal(err)
	}
}

func TestPublicationPrerequisitesFailClosed(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*publicationFake)
		blocker string
	}{
		{"unpaid", func(f *publicationFake) { f.coverage = app.CommercialCoverage{Reason: "subscription_required"} }, "subscription_required"},
		{"address", func(f *publicationFake) { f.shop.Address = nil }, "address_required"},
		{"invalid address", func(f *publicationFake) { f.shop.Address = &domain.Address{} }, "address_required"},
		{"service", func(f *publicationFake) { f.catalog.HasService = false }, "service_required"},
		{"professional", func(f *publicationFake) { f.catalog.Candidates = nil }, "professional_required"},
		{"schedule", func(f *publicationFake) { f.schedule = false }, "schedule_required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, scope := readyPublication()
			test.change(f)
			uc := app.NewShopPublication(f)
			status, err := uc.Get(context.Background(), scope)
			if err != nil || status.CanPublish {
				t.Fatal(status, err)
			}
			found := false
			for _, reason := range status.Blockers {
				found = found || reason == test.blocker
			}
			if !found {
				t.Fatal(status.Blockers)
			}
			if err := uc.Set(context.Background(), scope, true); !errors.Is(err, app.ErrPublicationBlocked) || f.writes != 0 {
				t.Fatal(f.writes, err)
			}
		})
	}
}

func TestPublicationLegacyAndFailureBoundaries(t *testing.T) {
	f, scope := readyPublication()
	f.shop.SelfService = false
	uc := app.NewShopPublication(f)
	status, err := uc.Get(context.Background(), scope)
	if err != nil || status.PublicationManaged || !status.PublicBookingEnabled || status.CanPublish || f.financialReads != 0 {
		t.Fatal(status, err)
	}
	if err := uc.Set(context.Background(), scope, false); !errors.Is(err, app.ErrPublicationNotManaged) {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*publicationFake)
		want   error
	}{
		{"access", func(f *publicationFake) { f.accessErr = app.ErrSettingsAccessDenied }, app.ErrSettingsAccessDenied},
		{"missing", func(f *publicationFake) { f.missing = true }, app.ErrSettingsNotFound},
		{"mismatch", func(f *publicationFake) { f.shop.ShopID = uuid.New() }, app.ErrSettingsMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, scope := readyPublication()
			test.change(f)
			uc := app.NewShopPublication(f)
			if _, err := uc.Get(context.Background(), scope); !errors.Is(err, test.want) {
				t.Fatal(err)
			}
			if err := uc.Set(context.Background(), scope, true); !errors.Is(err, test.want) || f.writes != 0 {
				t.Fatal(err)
			}
		})
	}
	f, scope = readyPublication()
	f.commitErr = errors.New("commit failed")
	if got, err := app.NewShopPublication(f).Get(context.Background(), scope); err == nil || got.ShopID != uuid.Nil {
		t.Fatal(got, err)
	}
	if err := app.NewShopPublication(f).Set(context.Background(), scope, true); err == nil {
		t.Fatal("commit failure reported success")
	}
	if err := uc.Set(context.Background(), app.SettingsScope{}, true); !errors.Is(err, app.ErrSettingsAccessDenied) {
		t.Fatal(err)
	}
}
