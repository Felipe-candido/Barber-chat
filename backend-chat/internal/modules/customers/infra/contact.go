package infra

import (
	"strings"
	"unicode"
	"unicode/utf8"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/nyaruka/phonenumbers/v2"
)

type ContactNormalizer struct{}

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
	if len(input.Phone) > 40 || len(input.PhoneCountry) != 2 || input.PhoneCountry[0] < 'A' || input.PhoneCountry[0] > 'Z' || input.PhoneCountry[1] < 'A' || input.PhoneCountry[1] > 'Z' {
		return booking.CustomerInput{}, booking.ErrInvalidCustomer
	}
	// Vanity numbers and extensions are not accepted by this contact form.
	if strings.ContainsFunc(input.Phone, func(r rune) bool {
		return !(r >= '0' && r <= '9') && r != '+' && r != ' ' && r != '-' && r != '(' && r != ')'
	}) {
		return booking.CustomerInput{}, booking.ErrInvalidCustomer
	}
	number, err := phonenumbers.Parse(input.Phone, input.PhoneCountry)
	if err != nil || !phonenumbers.IsValidNumber(number) || number.GetExtension() != "" {
		return booking.CustomerInput{}, booking.ErrInvalidCustomer
	}
	input.Phone = phonenumbers.Format(number, phonenumbers.E164)
	return input, nil
}
