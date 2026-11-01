package infra

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/nyaruka/phonenumbers/v2"
)

type ContactNormalizer struct{}

var errInvalidPhone = errors.New("invalid phone")

func (ContactNormalizer) Normalize(input booking.CustomerInput) (booking.CustomerInput, error) {
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Phone = strings.TrimSpace(input.Phone)
	input.PhoneCountry = strings.ToUpper(strings.TrimSpace(input.PhoneCountry))
	for _, name := range []string{input.FirstName, input.LastName} {
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
			return booking.CustomerInput{}, booking.ErrInvalidCustomer
		}
	}
	if len(input.PhoneCountry) != 2 {
		return booking.CustomerInput{}, booking.ErrInvalidCustomer
	}
	phone, err := normalizePhone(input.Phone, input.PhoneCountry)
	if err != nil {
		return booking.CustomerInput{}, booking.ErrInvalidCustomer
	}
	input.Phone = phone
	return input, nil
}

// Shared local parsing for booking contacts and read-only report search. A
// country is optional only when the number explicitly includes its calling code.
func normalizePhone(raw, country string) (string, error) {
	raw = strings.TrimSpace(raw)
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(raw) == 0 || len(raw) > 40 ||
		(country != "" && (len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z')) ||
		(country == "" && !strings.HasPrefix(raw, "+")) {
		return "", errInvalidPhone
	}
	// Vanity numbers and extensions are not accepted by this contact form.
	if strings.ContainsFunc(raw, func(r rune) bool {
		return !(r >= '0' && r <= '9') && r != '+' && r != ' ' && r != '-' && r != '(' && r != ')'
	}) {
		return "", errInvalidPhone
	}
	number, err := phonenumbers.Parse(raw, country)
	if err != nil || !phonenumbers.IsValidNumber(number) || number.GetExtension() != "" {
		return "", errInvalidPhone
	}
	return phonenumbers.Format(number, phonenumbers.E164), nil
}
