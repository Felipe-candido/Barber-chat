package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrInvalidShop = errors.New("invalid shop")
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type Shop struct {
	ID                   uuid.UUID
	Name, Slug, Timezone string
	Active               bool
	Address              Address
}

func NewShop(id uuid.UUID, name, slug, timezone string) (Shop, error) {
	name, slug, timezone = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(slug)), strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "America/Sao_Paulo"
	}
	if id == uuid.Nil || !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 150 ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 || len(slug) < 3 || len(slug) > 100 || !slugPattern.MatchString(slug) ||
		len(timezone) > 100 || timezone == "Local" {
		return Shop{}, ErrInvalidShop
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Shop{}, ErrInvalidShop
	}
	return Shop{ID: id, Name: name, Slug: slug, Timezone: timezone, Active: true}, nil
}
