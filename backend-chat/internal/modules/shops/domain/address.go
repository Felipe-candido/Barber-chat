package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidAddress = errors.New("invalid shop address")

var postalCodePattern = regexp.MustCompile(`^[0-9]{8}$`)

// Address is a physical business address, not a customer's home address.
// Brazil is the only supported country in the initial onboarding flow.
type Address struct {
	PostalCode, Street, Number, Complement, District, City, State string
}

func NewAddress(input Address) (Address, error) {
	input.PostalCode = strings.ReplaceAll(strings.TrimSpace(input.PostalCode), "-", "")
	input.Street = strings.TrimSpace(input.Street)
	input.Number = strings.TrimSpace(input.Number)
	input.Complement = strings.TrimSpace(input.Complement)
	input.District = strings.TrimSpace(input.District)
	input.City = strings.TrimSpace(input.City)
	input.State = strings.ToUpper(strings.TrimSpace(input.State))
	if !postalCodePattern.MatchString(input.PostalCode) ||
		!addressText(input.Street, 150, false) || !addressText(input.Number, 20, false) ||
		!addressText(input.Complement, 100, true) || !addressText(input.District, 100, false) ||
		!addressText(input.City, 100, false) || !validState(input.State) {
		return Address{}, ErrInvalidAddress
	}
	return input, nil
}

func addressText(value string, max int, optional bool) bool {
	return utf8.ValidString(value) && (optional || value != "") &&
		utf8.RuneCountInString(value) <= max && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validState(state string) bool {
	switch state {
	case "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO":
		return true
	default:
		return false
	}
}
