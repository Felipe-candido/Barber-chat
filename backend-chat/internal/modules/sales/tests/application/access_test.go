package application_test

import (
	"errors"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/google/uuid"
)

func TestSalesAuthorityClampsBarberAndSeparatesOwnerAttribution(t *testing.T) {
	scope := application.SalesScope{ActorID: uuid.New(), ShopID: uuid.New()}
	professional, other := uuid.New(), uuid.New()
	barber := application.SalesAuthority{Scope: scope, ProfessionalID: professional}
	for _, requested := range []uuid.UUID{uuid.Nil, professional} {
		if resolved, err := barber.ProfessionalFor(scope, requested); err != nil || resolved != professional {
			t.Fatal("barber scope lost", err)
		}
	}
	if _, err := barber.ProfessionalFor(scope, other); !errors.Is(err, application.ErrSalesAccessDenied) {
		t.Fatal("barber chose another professional", err)
	}
	if err := barber.RequireOwner(scope); !errors.Is(err, application.ErrSalesAccessDenied) {
		t.Fatal("barber became owner", err)
	}
	owner := application.SalesAuthority{Scope: scope, Owner: true}
	for _, requested := range []uuid.UUID{uuid.Nil, professional, other} {
		if got, err := owner.ProfessionalFor(scope, requested); err != nil || got != requested {
			t.Fatal("owner attribution rejected", err)
		}
	}
	if err := owner.RequireOwner(scope); err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []application.SalesScope{{ActorID: uuid.New(), ShopID: scope.ShopID}, {ActorID: scope.ActorID, ShopID: uuid.New()}, {ShopID: scope.ShopID}} {
		if _, err := owner.ProfessionalFor(foreign, professional); !errors.Is(err, application.ErrSalesAccessDenied) {
			t.Fatal("authority crossed actor/tenant", err)
		}
	}
	owner.ProfessionalID = professional
	if _, err := owner.ProfessionalFor(scope, professional); !errors.Is(err, application.ErrInconsistentSalesData) {
		t.Fatal("invalid owner projection accepted", err)
	}
	barber.ProfessionalID = uuid.Nil
	if _, err := barber.ProfessionalFor(scope, uuid.Nil); !errors.Is(err, application.ErrInconsistentSalesData) {
		t.Fatal("barber without professional accepted", err)
	}
}
