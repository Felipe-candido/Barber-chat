package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type creationFake struct {
	record          *app.ShopCreationRecord
	shop            domain.Shop
	creates, owners int
	failure         error
	inactive        bool
}

func registrationAddress() domain.Address {
	return domain.Address{PostalCode: "01310100", Street: "Avenida Paulista", Number: "100", District: "Bela Vista", City: "São Paulo", State: "SP"}
}

func (f *creationFake) EnsureActiveUser(context.Context, uuid.UUID, string) error {
	if f.inactive {
		return app.ErrOwnerInactive
	}
	return nil
}
func (f *creationFake) CreateOwnerMembership(context.Context, uuid.UUID, uuid.UUID) error {
	f.owners++
	return f.failure
}
func (f *creationFake) RequireActiveOwner(context.Context, uuid.UUID, uuid.UUID) error {
	return f.failure
}
func (f *creationFake) FindCreation(context.Context, uuid.UUID, [32]byte) (app.ShopCreationRecord, bool, error) {
	if f.record != nil {
		return *f.record, true, nil
	}
	return app.ShopCreationRecord{}, false, nil
}
func (f *creationFake) Create(_ context.Context, s domain.Shop) error {
	f.shop = s
	f.creates++
	return nil
}
func (f *creationFake) RecordCreation(_ context.Context, _ uuid.UUID, _ [32]byte, fp [32]byte, _ uuid.UUID) error {
	f.record = &app.ShopCreationRecord{Shop: f.shop, Fingerprint: fp}
	return nil
}

type creationTx struct {
	store     *creationFake
	commitErr error
}

func (f creationTx) WithinShopCreationTx(ctx context.Context, fn func(context.Context, app.ShopCreationTx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fn(ctx, app.ShopCreationTx{Owners: f.store, Shops: f.store}); err != nil {
		return err
	}
	return f.commitErr
}

func TestCreateShopIdempotencyAndOwner(t *testing.T) {
	f := &creationFake{}
	uc := app.NewCreateShop(creationTx{store: f})
	input := app.CreateShopInput{OwnerID: uuid.New(), DisplayName: " Owner ", Name: " My Shop ", Slug: " MY-SHOP ", IdempotencyKey: strings.Repeat("ab", 32), Address: registrationAddress()}
	first, err := uc.Execute(context.Background(), input)
	if err != nil || first.Replayed || first.Shop.ID == uuid.Nil || f.creates != 1 || f.owners != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	input.Name, input.Slug, input.DisplayName, input.Timezone = "My Shop", "my-shop", "Owner", "America/Sao_Paulo"
	second, err := uc.Execute(context.Background(), input)
	if err != nil || !second.Replayed || second.Shop.ID != first.Shop.ID || f.creates != 1 || f.owners != 1 {
		t.Fatalf("%+v %v", second, err)
	}
	input.Name = "Other input"
	if _, err := uc.Execute(context.Background(), input); !errors.Is(err, app.ErrCreationConflict) {
		t.Fatal(err)
	}
	input.Name = "My Shop"
	input.Address.Number = "200"
	if _, err := uc.Execute(context.Background(), input); !errors.Is(err, app.ErrCreationConflict) {
		t.Fatal("different address reused request key", err)
	}
	input.Address = registrationAddress()
	f.failure = app.ErrOwnershipDenied
	if _, err := uc.Execute(context.Background(), input); !errors.Is(err, app.ErrOwnershipDenied) {
		t.Fatal(err)
	}
}

func TestCreateShopRejectsInvalidAndDoesNotReportUncommittedSuccess(t *testing.T) {
	input := app.CreateShopInput{OwnerID: uuid.New(), DisplayName: "Owner", Name: "Shop", Slug: "my-shop", IdempotencyKey: strings.Repeat("ab", 32), Address: registrationAddress()}
	for _, mutate := range []func(*app.CreateShopInput){func(i *app.CreateShopInput) { i.OwnerID = uuid.Nil }, func(i *app.CreateShopInput) { i.IdempotencyKey = "bad" }, func(i *app.CreateShopInput) { i.IdempotencyKey = strings.Repeat("AB", 32) }, func(i *app.CreateShopInput) { i.DisplayName = "" }, func(i *app.CreateShopInput) { i.DisplayName = "Name\x00" }} {
		f := &creationFake{}
		bad := input
		mutate(&bad)
		if _, err := app.NewCreateShop(creationTx{store: f}).Execute(context.Background(), bad); !errors.Is(err, app.ErrInvalidRegistration) || f.creates != 0 {
			t.Fatal(err)
		}
	}
	failure := errors.New("commit result unknown")
	for _, f := range []*creationFake{{inactive: true}, {failure: failure}, {}} {
		tx := creationTx{store: f}
		want := failure
		if f.inactive {
			want = app.ErrOwnerInactive
		} else if f.failure == nil {
			tx.commitErr = failure
		}
		got, err := app.NewCreateShop(tx).Execute(context.Background(), input)
		if !errors.Is(err, want) || got.Shop.ID != uuid.Nil {
			t.Fatalf("%+v %v", got, err)
		}
	}
}

type publicReaderFunc func(context.Context, string) (app.PublicShopState, bool, error)

func (f publicReaderFunc) FindPublicShopState(ctx context.Context, s string) (app.PublicShopState, bool, error) {
	return f(ctx, s)
}

type eligibilityFunc func(context.Context, uuid.UUID) (bool, error)

func (f eligibilityFunc) CanOperateShop(ctx context.Context, id uuid.UUID) (bool, error) {
	return f(ctx, id)
}

func TestPublicShopRequiresPublicationAndPaidEligibility(t *testing.T) {
	id := uuid.New()
	for _, test := range []struct {
		legacy, published, paid bool
		allowed                 bool
	}{{false, false, false, false}, {false, true, false, false}, {false, false, true, false}, {false, true, true, true}, {true, false, false, true}} {
		reader := publicReaderFunc(func(context.Context, string) (app.PublicShopState, bool, error) {
			return app.PublicShopState{ShopID: id, Active: true, PublicationRequested: test.published, LegacyPublicAccess: test.legacy}, true, nil
		})
		paid := eligibilityFunc(func(context.Context, uuid.UUID) (bool, error) { return test.paid, nil })
		err := app.NewCheckPublicBooking(reader, paid).Execute(context.Background(), "my-shop")
		if (err == nil) != test.allowed {
			t.Fatalf("%+v %v", test, err)
		}
	}
	failure := errors.New("billing unavailable")
	reader := publicReaderFunc(func(context.Context, string) (app.PublicShopState, bool, error) {
		return app.PublicShopState{ShopID: id, Active: true, PublicationRequested: true}, true, nil
	})
	if err := app.NewCheckPublicBooking(reader, eligibilityFunc(func(context.Context, uuid.UUID) (bool, error) { return false, failure })).Execute(context.Background(), "my-shop"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
