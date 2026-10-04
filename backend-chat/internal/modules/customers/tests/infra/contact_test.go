package infra_test

import (
	"errors"
	"strings"
	"testing"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
)

func TestContactNormalization(t *testing.T) {
	normalizer := infra.ContactNormalizer{}
	input := booking.CustomerInput{FirstName: " Ana ", LastName: " Silva ", Phone: "(11) 91234-5678", PhoneCountry: "br"}
	normalized, err := normalizer.Normalize(input)
	if err != nil || normalized.FirstName != "Ana" || normalized.LastName != "Silva" || normalized.Phone != "+5511912345678" || normalized.PhoneCountry != "BR" {
		t.Fatalf("normalization: %+v %v", normalized, err)
	}
	for _, mutate := range []func(*booking.CustomerInput){
		func(i *booking.CustomerInput) { i.FirstName = " " }, func(i *booking.CustomerInput) { i.LastName = strings.Repeat("a", 101) },
		func(i *booking.CustomerInput) { i.Phone = "123" }, func(i *booking.CustomerInput) { i.Phone = "(00) 91234-5678" },
		func(i *booking.CustomerInput) { i.Phone = "+55 11 91234-5678 ext 123" }, func(i *booking.CustomerInput) { i.FirstName = "A\nB" },
	} {
		invalid := input
		mutate(&invalid)
		if _, err := normalizer.Normalize(invalid); !errors.Is(err, booking.ErrInvalidCustomer) {
			t.Fatalf("invalid contact accepted: %v", err)
		}
	}
}
