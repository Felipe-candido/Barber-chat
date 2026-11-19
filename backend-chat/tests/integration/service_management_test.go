//go:build integration

package integration

import (
	"errors"
	"testing"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	catalogpg "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
)

func TestServiceManagementPreservesAppointments(t *testing.T) {
	f := newBookingFixture(t)
	other := newBookingFixture(t)
	created := bookingResult(t, f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey()), 201)
	repo := catalogpg.NewRepository(db.New(f.pool))
	manage := catalog.NewManageServices(repo)
	input := catalog.UpdateServiceInput{ShopID: other.shop, ServiceID: f.service, Name: "Changed", Description: "New details", DurationMinutes: 20, PriceCents: 2500}
	if _, err := manage.Update(f.ctx, input); !errors.Is(err, catalog.ErrServiceNotFound) {
		t.Fatal("cross-shop update", err)
	}
	if err := manage.Delete(f.ctx, other.shop, f.service); !errors.Is(err, catalog.ErrServiceNotFound) {
		t.Fatal("cross-shop delete", err)
	}
	input.ShopID = f.shop
	updated, err := manage.Update(f.ctx, input)
	if err != nil || updated.ID != f.service || updated.Name != input.Name || updated.PriceCents != 2500 || updated.DurationMinutes != 20 {
		t.Fatalf("update not persisted: %+v %v", updated, err)
	}
	items, err := repo.ListActiveByShop(f.ctx, f.shop)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == f.service {
			found = item.Name == input.Name
		}
	}
	if !found {
		t.Fatal("catalog not updated")
	}
	for range 2 {
		if err := manage.Delete(f.ctx, f.shop, f.service); err != nil {
			t.Fatal("idempotent delete", err)
		}
	}
	items, err = repo.ListActiveByShop(f.ctx, f.shop)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == f.service {
			t.Fatal("deleted service still listed")
		}
	}
	if _, err := manage.Update(f.ctx, input); !errors.Is(err, catalog.ErrServiceNotFound) {
		t.Fatal("edit reactivated a deleted service", err)
	}
	details := bookingResult(t, f.request("POST", "/appointments/lookup", map[string]string{"code": created.Code}, ""), 200)
	if details.Appointment != created.Appointment {
		t.Fatal("service management changed appointment snapshots")
	}
	if w := f.request("POST", "/appointments", f.body(f.start.Add(2*time.Hour), f.service), bookingKey()); w.Code != 404 {
		t.Fatalf("deleted service accepted a new booking: %d %s", w.Code, w.Body)
	}
}
