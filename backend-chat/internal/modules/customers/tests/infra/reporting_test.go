package infra_test

import (
	"errors"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
)

func TestReportPhoneSearchUsesCanonicalContactNumbersWithoutCreatingContacts(t *testing.T) {
	n := infra.ContactNormalizer{}
	for _, tc := range []struct{ raw, country, want string }{
		{" (11) 91234-5678 ", "br", "+5511912345678"},
		{"+55 (11) 91234-5678", "", "+5511912345678"},
		{"+1 415 555 2671", "", "+14155552671"},
	} {
		got, err := n.NormalizeReportPhone(tc.raw, tc.country)
		if err != nil || got != tc.want {
			t.Fatal(got, err)
		}
	}
	for _, tc := range []struct{ raw, country string }{
		{"", "BR"}, {"11912345678", ""}, {"123", "BR"}, {"+55 11 91234-5678 ext 123", "BR"},
		{"+55 11 91234-5678", "BRA"}, {"+55 11 91234-5678", "1R"}, {"(11) 91234-5678", "ZZ"},
		{"+55\n11912345678", "BR"},
	} {
		if _, err := n.NormalizeReportPhone(tc.raw, tc.country); !errors.Is(err, application.ErrInvalidQuery) {
			t.Fatal("invalid report number accepted", tc, err)
		}
	}
}
