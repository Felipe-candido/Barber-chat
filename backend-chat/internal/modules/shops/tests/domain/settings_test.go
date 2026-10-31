package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
)

func validBasicSettings() domain.BasicSettings {
	return domain.BasicSettings{Name: " Shop ", Phone: "(11) 91234-5678", Description: " Description\r\nsecond line ",
		Address: domain.Address{PostalCode: "01310-100", Street: "Avenida Paulista", Number: "100", District: "Bela Vista", City: "São Paulo", State: "sp"}}
}

func TestBasicSettingsValidation(t *testing.T) {
	got, err := domain.NewBasicSettings(validBasicSettings())
	if err != nil || got.Name != "Shop" || got.Phone != "+5511912345678" || got.Description != "Description\nsecond line" || got.Address.State != "SP" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, phone := range []string{"", "(11) 3123-4567", "+55 11 91234-5678"} {
		input := validBasicSettings()
		input.Phone = phone
		if _, err := domain.NewBasicSettings(input); err != nil {
			t.Fatal(phone, err)
		}
	}
	for _, change := range []func(*domain.BasicSettings){
		func(s *domain.BasicSettings) { s.Name = " " },
		func(s *domain.BasicSettings) { s.Name = strings.Repeat("x", 151) },
		func(s *domain.BasicSettings) { s.Name = "Shop\x00" },
		func(s *domain.BasicSettings) { s.Description = strings.Repeat("x", 1001) },
		func(s *domain.BasicSettings) { s.Description = "Description\x1b" },
		func(s *domain.BasicSettings) { s.Description = string([]byte{255}) },
		func(s *domain.BasicSettings) { s.Phone = "123" },
		func(s *domain.BasicSettings) { s.Phone = "+1 202-555-0123" },
		func(s *domain.BasicSettings) { s.Phone = "(11) 91234-5678 ext 2" },
		func(s *domain.BasicSettings) { s.Phone = strings.Repeat("1", 41) },
	} {
		input := validBasicSettings()
		change(&input)
		if _, err := domain.NewBasicSettings(input); !errors.Is(err, domain.ErrInvalidSettings) {
			t.Fatal(err)
		}
	}
	input := validBasicSettings()
	input.Address = domain.Address{}
	if _, err := domain.NewBasicSettings(input); !errors.Is(err, domain.ErrInvalidAddress) {
		t.Fatal("address not required", err)
	}
}
