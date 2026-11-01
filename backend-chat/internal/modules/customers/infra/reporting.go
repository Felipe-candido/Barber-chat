package infra

import "github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"

var _ application.PhoneNormalizer = ContactNormalizer{}

func (ContactNormalizer) NormalizeReportPhone(raw, country string) (string, error) {
	phone, err := normalizePhone(raw, country)
	if err != nil {
		return "", application.ErrInvalidQuery
	}
	return phone, nil
}
