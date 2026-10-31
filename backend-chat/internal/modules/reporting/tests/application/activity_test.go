package application_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/google/uuid"
)

func TestCombinedActivityKeepsMeasuresSeparateAndBoundsIntegerMoney(t *testing.T) {
	values := application.ActivityValues{Currency: "BRL", CompletedServiceValueCents: 10000, ProductSaleValueCents: 9000, CompletedAppointmentCount: 2, ProductSaleCount: 1, ProductUnitCount: 3}
	if total, err := values.CombinedValueCents(); err != nil || total != 19000 || values.CompletedAppointmentCount != 2 || values.ProductSaleCount != 1 || values.ProductUnitCount != 3 {
		t.Fatal("R$190 example failed", err)
	}
	if total, err := (application.ActivityValues{Currency: "BRL"}).CombinedValueCents(); err != nil || total != 0 {
		t.Fatal("empty report failed", err)
	}
	for _, mutate := range []func(*application.ActivityValues){
		func(v *application.ActivityValues) { v.Currency = "" }, func(v *application.ActivityValues) { v.Currency = "brl" },
		func(v *application.ActivityValues) { v.ProductSaleValueCents = -1 }, func(v *application.ActivityValues) { v.CompletedServiceValueCents = -1 },
		func(v *application.ActivityValues) { v.ProductSaleValueCents = math.MaxInt64 }, func(v *application.ActivityValues) { v.CompletedServiceValueCents = math.MaxInt64 },
		func(v *application.ActivityValues) { v.CompletedAppointmentCount = -1 }, func(v *application.ActivityValues) { v.ProductUnitCount = -1 },
	} {
		bad := values
		mutate(&bad)
		if _, err := bad.CombinedValueCents(); !errors.Is(err, application.ErrInconsistentData) {
			t.Fatal("invalid report bucket accepted", err)
		}
	}
}

func TestActivityAudienceCannotExpandToAnotherProfessionalOrTenant(t *testing.T) {
	actor, shop, professional := uuid.New(), uuid.New(), uuid.New()
	barber := application.ActivityAudience{ActorID: actor, ShopID: shop, ProfessionalID: professional}
	if got, err := barber.ProfessionalFor(actor, shop, uuid.Nil); err != nil || got != professional {
		t.Fatal("barber report unscoped", err)
	}
	if _, err := barber.ProfessionalFor(actor, shop, uuid.New()); !errors.Is(err, application.ErrActivityAccessDenied) {
		t.Fatal("foreign professional accepted", err)
	}
	owner := application.ActivityAudience{ActorID: actor, ShopID: shop, CanReadAll: true}
	if got, err := owner.ProfessionalFor(actor, shop, uuid.Nil); err != nil || got != uuid.Nil {
		t.Fatal("owner cannot read unit", err)
	}
	if got, err := owner.ProfessionalFor(actor, shop, professional); err != nil || got != professional {
		t.Fatal(err)
	}
	if _, err := owner.ProfessionalFor(actor, uuid.New(), uuid.Nil); !errors.Is(err, application.ErrActivityAccessDenied) {
		t.Fatal("foreign tenant accepted", err)
	}
	if _, err := owner.ProfessionalFor(uuid.New(), shop, uuid.Nil); !errors.Is(err, application.ErrActivityAccessDenied) {
		t.Fatal("foreign actor accepted", err)
	}
	barber.ProfessionalID = uuid.Nil
	if _, err := barber.ProfessionalFor(actor, shop, uuid.Nil); !errors.Is(err, application.ErrActivityAccessDenied) {
		t.Fatal("barber without professional saw whole unit", err)
	}
}
