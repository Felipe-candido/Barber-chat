package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nyaruka/phonenumbers/v2"
)

var ErrInvalidSettings = errors.New("invalid shop settings")

// BasicSettings deliberately excludes slug, timezone, access and billing state.
// Their changes require separate policies rather than a general-purpose update.
type BasicSettings struct {
	Name, Phone, Description string
	Address                  Address
}

func NewBasicSettings(input BasicSettings) (BasicSettings, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Description = strings.TrimSpace(strings.ReplaceAll(input.Description, "\r\n", "\n"))
	if !addressText(input.Name, 150, false) || !utf8.ValidString(input.Description) ||
		utf8.RuneCountInString(input.Description) > 1000 || strings.ContainsFunc(input.Description, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}) {
		return BasicSettings{}, ErrInvalidSettings
	}
	if input.Phone != "" {
		// Reuse the installed, pure parser; no Auth or messaging provider is called.
		if len(input.Phone) > 40 || strings.ContainsFunc(input.Phone, func(r rune) bool {
			return !(r >= '0' && r <= '9') && !strings.ContainsRune("+ -()", r)
		}) {
			return BasicSettings{}, ErrInvalidSettings
		}
		number, err := phonenumbers.Parse(input.Phone, "BR")
		if err != nil || !phonenumbers.IsValidNumber(number) || number.GetCountryCode() != 55 || number.GetExtension() != "" {
			return BasicSettings{}, ErrInvalidSettings
		}
		input.Phone = phonenumbers.Format(number, phonenumbers.E164)
	}
	address, err := NewAddress(input.Address)
	if err != nil {
		return BasicSettings{}, err
	}
	input.Address = address
	return input, nil
}
