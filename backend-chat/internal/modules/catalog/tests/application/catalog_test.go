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

func TestCreateUsesAuthorizedShop(t *testing.T) {
	shopID := uuid.New()
	repo := &fakeRepository{}
	output, err := application.NewCreateService(repo).Execute(context.Background(), application.CreateServiceInput{ShopID: shopID, Name: " Corte ", DurationMinutes: 30, PriceCents: 3500})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.created) != 1 || repo.created[0].ShopID != shopID || output.Name != "Corte" || output.Currency != "BRL" {
		t.Fatal("authorized tenant or normalized service was not preserved")
	}
}
func TestCreateDoesNotPersistRejectedInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shopID uuid.UUID
		title  string
		want   error
	}{
		{"missing authorized shop", uuid.Nil, "Corte", domain.ErrInvalidShopID},
		{"invalid service", uuid.New(), " ", domain.ErrInvalidName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			_, err := application.NewCreateService(repo).Execute(context.Background(), application.CreateServiceInput{ShopID: tc.shopID, Name: tc.title, DurationMinutes: 30})
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
	_, err := application.NewCreateService(repo).Execute(context.Background(), application.CreateServiceInput{ShopID: shops.id, Name: "Corte", DurationMinutes: 30})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := application.NewListServices(repo, shops).Execute(context.Background(), "shop"); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestAuthorizedListScopesRepository(t *testing.T) {
	shopID := uuid.New()
	repo := &fakeRepository{}
	uc := application.NewListAuthorizedServices(repo)
	if _, err := uc.Execute(context.Background(), uuid.Nil); !errors.Is(err, domain.ErrInvalidShopID) || repo.listedShop != uuid.Nil {
		t.Fatal("missing scope reached persistence", err)
	}
	result, err := uc.Execute(context.Background(), shopID)
	if err != nil || result == nil || len(result) != 0 || repo.listedShop != shopID {
		t.Fatal("authorized scope or empty result is incorrect", err)
	}
	service, err := domain.NewService(shopID, "Corte", "", 30, 3500)
	if err != nil {
		t.Fatal(err)
	}
	repo.services = []domain.Service{service}
	result, err = uc.Execute(context.Background(), shopID)
	if err != nil || len(result) != 1 || result[0].ID != service.ID || result[0].Name != service.Name {
		t.Fatal("service projection is incorrect", err)
	}
	repo.err = errors.New("storage unavailable")
	if _, err := uc.Execute(context.Background(), shopID); !errors.Is(err, repo.err) {
		t.Fatal("repository failure was not preserved", err)
	}
}
