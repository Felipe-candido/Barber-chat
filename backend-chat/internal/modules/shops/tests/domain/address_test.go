package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
)

func TestAddressRequiredAndNormalized(t *testing.T) {
	input := domain.Address{PostalCode: " 01310-100 ", Street: " Avenida Paulista ", Number: " S/N ", City: " São Paulo ", District: " Bela Vista ", State: " sp "}
	got, err := domain.NewAddress(input)
	if err != nil || got.PostalCode != "01310100" || got.State != "SP" || got.Street != "Avenida Paulista" || got.Number != "S/N" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, change := range []func(*domain.Address){
		func(a *domain.Address) { *a = domain.Address{} },
		func(a *domain.Address) { a.PostalCode = "01310abc" },
		func(a *domain.Address) { a.Street = " " },
		func(a *domain.Address) { a.Number = "" },
		func(a *domain.Address) { a.City = "" },
		func(a *domain.Address) { a.District = "" },
		func(a *domain.Address) { a.State = "ZZ" },
		func(a *domain.Address) { a.Complement = strings.Repeat("x", 101) },
		func(a *domain.Address) { a.Street = "Rua\x00" },
	} {
		bad := input
		change(&bad)
		if _, err := domain.NewAddress(bad); !errors.Is(err, domain.ErrInvalidAddress) {
			t.Fatal("invalid address accepted", err)
		}
	}
}
