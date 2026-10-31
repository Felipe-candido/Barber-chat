package domain_test

import (
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestNewShop(t *testing.T) {
	id := uuid.New()
	got, err := domain.NewShop(id, " Barbearia São João ", " SAO-JOAO ", "")
	if err != nil || got.ID != id || got.Name != "Barbearia São João" || got.Slug != "sao-joao" || got.Timezone != "America/Sao_Paulo" || !got.Active {
		t.Fatalf("%+v %v", got, err)
	}
	for _, test := range []struct{ name, slug, zone string }{
		{"", "valid", "UTC"}, {strings.Repeat("ã", 151), "valid", "UTC"}, {"Name\x00", "valid", "UTC"},
		{"\xff", "valid", "UTC"}, {"Name", "ab", "UTC"}, {"Name", "--invalid", "UTC"},
		{"Name", "has space", "UTC"}, {"Name", "a/b", "UTC"}, {"Name", "são-joão", "UTC"},
		{"Name", strings.Repeat("a", 101), "UTC"}, {"Name", "valid", "Local"}, {"Name", "valid", "UTC-3"},
		{"Name", "valid", "../etc/passwd"},
	} {
		if _, err := domain.NewShop(id, test.name, test.slug, test.zone); !errors.Is(err, domain.ErrInvalidShop) {
			t.Fatalf("accepted %+v: %v", test, err)
		}
	}
	if _, err := domain.NewShop(uuid.Nil, "Name", "valid", "UTC"); !errors.Is(err, domain.ErrInvalidShop) {
		t.Fatal(err)
	}
}
