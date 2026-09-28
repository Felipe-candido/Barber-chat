package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

type fakeShops struct {
	id    uuid.UUID
	found bool
	err   error
	slug  string
}

func (f *fakeShops) LookupActiveShop(_ context.Context, slug string) (uuid.UUID, bool, error) {
	f.slug = slug
	return f.id, f.found, f.err
}

type fakeRepository struct {
	created    []domain.Service
	listedShop uuid.UUID
	services   []domain.Service
	err        error
}

func (f *fakeRepository) Create(_ context.Context, s domain.Service) (domain.Service, error) {
	f.created = append(f.created, s)
	return s, f.err
}
func (f *fakeRepository) ListActiveByShop(_ context.Context, id uuid.UUID) ([]domain.Service, error) {
	f.listedShop = id
	return f.services, f.err
}

func TestCreateUsesResolvedShop(t *testing.T) {
	shops := &fakeShops{id: uuid.New(), found: true}
	repo := &fakeRepository{}
	output, err := application.NewCreateService(repo, shops).Execute(context.Background(), application.CreateServiceInput{ShopSlug: "trusted-shop", Name: " Corte ", DurationMinutes: 30, PriceCents: 3500})
	if err != nil {
		t.Fatal(err)
	}
	if shops.slug != "trusted-shop" || len(repo.created) != 1 || repo.created[0].ShopID != shops.id || output.Name != "Corte" || output.Currency != "BRL" {
		t.Fatal("resolved tenant or normalized service was not preserved")
	}
}
func TestCreateDoesNotPersistRejectedInput(t *testing.T) {
	for _, tc := range []struct {
		name        string
		found       bool
		resolverErr error
		title       string
		want        error
	}{
		{"unknown shop", false, nil, "Corte", application.ErrShopNotFound},
		{"resolver failure", false, context.DeadlineExceeded, "Corte", context.DeadlineExceeded},
		{"invalid service", true, nil, " ", domain.ErrInvalidName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			shops := &fakeShops{id: uuid.New(), found: tc.found, err: tc.resolverErr}
			_, err := application.NewCreateService(repo, shops).Execute(context.Background(), application.CreateServiceInput{ShopSlug: "shop", Name: tc.title, DurationMinutes: 30})
			if !errors.Is(err, tc.want) || len(repo.created) != 0 {
				t.Fatalf("error=%v, writes=%d", err, len(repo.created))
			}
		})
	}
}
func TestListScopesRepositoryAndReturnsEmptySlice(t *testing.T) {
	shops := &fakeShops{id: uuid.New(), found: true}
	repo := &fakeRepository{}
	result, err := application.NewListServices(repo, shops).Execute(context.Background(), "shop")
	if err != nil || result == nil || len(result) != 0 || repo.listedShop != shops.id {
		t.Fatal("list scope or empty result is incorrect", err)
	}
	shops.found = false
	if _, err := application.NewListServices(repo, shops).Execute(context.Background(), "missing"); !errors.Is(err, application.ErrShopNotFound) {
		t.Fatal(err)
	}
}
func TestRepositoryFailureIsPreserved(t *testing.T) {
	sentinel := errors.New("storage failure")
	shops := &fakeShops{id: uuid.New(), found: true}
	repo := &fakeRepository{err: sentinel}
	_, err := application.NewCreateService(repo, shops).Execute(context.Background(), application.CreateServiceInput{ShopSlug: "shop", Name: "Corte", DurationMinutes: 30})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := application.NewListServices(repo, shops).Execute(context.Background(), "shop"); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
