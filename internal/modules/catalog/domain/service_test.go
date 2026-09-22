package domain

import (
	"errors"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestNewService(t *testing.T) {
	shop := uuid.New()
	service, err := NewService(shop, "  Corte  ", "  Tesoura  ", 30, 3500)
	if err != nil {
		t.Fatal(err)
	}
	if service.ID == uuid.Nil || service.ShopID != shop || service.Name != "Corte" || service.Description != "Tesoura" || !service.Active || service.Currency != "BRL" {
		t.Fatalf("unexpected service: %+v", service)
	}
	for _, tc := range []struct {
		name     string
		shop     uuid.UUID
		title    string
		duration int
		price    int64
		want     error
	}{
		{"missing shop", uuid.Nil, "Corte", 30, 3500, ErrInvalidShopID},
		{"blank name", shop, " \t", 30, 3500, ErrInvalidName},
		{"long name", shop, strings.Repeat("é", 101), 30, 3500, ErrInvalidName},
		{"zero duration", shop, "Corte", 0, 3500, ErrInvalidDuration},
		{"negative duration", shop, "Corte", -1, 3500, ErrInvalidDuration},
		{"negative price", shop, "Corte", 30, -1, ErrInvalidPrice},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(tc.shop, tc.title, "", tc.duration, tc.price)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	// Use a runtime conversion so the test also compiles on 32-bit systems.
	large := int64(1)<<32 | 1
	if int64(int(large)) == large {
		if _, err := NewService(shop, "Corte", "", int(large), 0); !errors.Is(err, ErrInvalidDuration) {
			t.Fatal("duration overflow accepted")
		}
	}
	if _, err := NewService(shop, strings.Repeat("é", 100), "", 30, 0); err != nil {
		t.Fatal("valid Unicode name or zero price rejected", err)
	}
}
