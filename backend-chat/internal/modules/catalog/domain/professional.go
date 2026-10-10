package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrInvalidProfessional = errors.New("professional name must contain 1 to 100 printable characters")

// A bookable professional is not a Supabase account or an administrative role.
type Professional struct {
	ID, ShopID uuid.UUID
	Name       string
	Active     bool
}

func NewProfessional(shopID uuid.UUID, name string) (Professional, error) {
	if shopID == uuid.Nil {
		return Professional{}, ErrInvalidShopID
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return Professional{}, ErrInvalidProfessional
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Professional{}, err
	}
	return Professional{ID: id, ShopID: shopID, Name: name, Active: true}, nil
}
