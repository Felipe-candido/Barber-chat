package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
)

func TestReportingPeriodUsesCivilShopDates(t *testing.T) {
	for _, tc := range []struct {
		name, from, through, timezone, start, end string
	}{
		{"Brazil day", "2026-10-12", "2026-10-12", "America/Sao_Paulo", "2026-10-12T03:00:00Z", "2026-10-13T03:00:00Z"},
		{"month", "2026-10-01", "2026-10-31", "America/Sao_Paulo", "2026-10-01T03:00:00Z", "2026-11-01T03:00:00Z"},
		{"short DST day", "2026-03-08", "2026-03-08", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"},
		{"long DST day", "2026-11-01", "2026-11-01", "America/New_York", "2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z"},
		{"leap year", "2024-01-01", "2024-12-31", "UTC", "2024-01-01T00:00:00Z", "2025-01-01T00:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			period, err := domain.NewPeriod(tc.from, tc.through, tc.timezone)
			if err != nil || !period.Valid() || period.From().Format(time.RFC3339) != tc.start || period.Until().Format(time.RFC3339) != tc.end || period.Timezone() != tc.timezone {
				t.Fatal(period, err)
			}
		})
	}
}

func TestReportingPeriodRejectsInvalidOrUnboundedDates(t *testing.T) {
	for _, tc := range []struct{ from, through, timezone string }{
		{"", "2026-10-12", "UTC"},
		{"2026-10-12", "2026-10-12", ""},
		{"2026-10-12", "2026-10-12", "not-a-zone"},
		{"2026-02-30", "2026-03-01", "UTC"},
		{"2026-10-13", "2026-10-12", "UTC"},
		{"2026-10-1", "2026-10-12", "UTC"},
		{"2026-10-12T00:00:00Z", "2026-10-12", "UTC"},
		{"2025-01-01", "2026-01-02", "UTC"},
		{"2026-01-01", "2026-12-31", "Local"},
		{"1800-01-01", "1800-01-02", "UTC"},
		{"9999-12-31", "9999-12-31", "UTC"},
		{"2011-12-30", "2011-12-30", "Pacific/Apia"},
	} {
		if _, err := domain.NewPeriod(tc.from, tc.through, tc.timezone); !errors.Is(err, domain.ErrInvalidPeriod) {
			t.Fatal("invalid period accepted", tc, err)
		}
	}
	if (domain.Period{}).Valid() {
		t.Fatal("zero period accepted")
	}
}
