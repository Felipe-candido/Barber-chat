package domain_test

import (
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"testing"
	"time"
)

func TestTimeOffIncludesCivilDatesAndClockChanges(t *testing.T) {
	for _, tc := range []struct {
		from, through, zone string
		hours               int
	}{
		{"2026-11-20", "2026-11-22", "America/Sao_Paulo", 72},
		{"2027-03-14", "2027-03-14", "America/New_York", 23},
		{"2027-11-07", "2027-11-07", "America/New_York", 25},
		{"2018-11-04", "2018-11-04", "America/Sao_Paulo", 23},
	} {
		t.Run(tc.from+tc.zone, func(t *testing.T) {
			r, err := domain.TimeOffInterval(tc.from, tc.through, tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			if r.EndsAt.Sub(r.StartsAt) != time.Duration(tc.hours)*time.Hour {
				t.Fatalf("duration %s", r.EndsAt.Sub(r.StartsAt))
			}
			loc, _ := time.LoadLocation(tc.zone)
			if r.StartsAt.In(loc).Format("2006-01-02") != tc.from || r.EndsAt.Add(-time.Nanosecond).In(loc).Format("2006-01-02") != tc.through {
				t.Fatal("civil dates shifted")
			}
		})
	}
}
func TestTimeOffRejectsInvalidDates(t *testing.T) {
	for _, tc := range [][3]string{{"2026-02-30", "2026-03-01", "UTC"}, {"2026-11-20", "2026-11-19", "UTC"}, {"2026-11-20", "2027-11-21", "UTC"}, {"2026-11-20", "2026-11-20", ""}, {"2026-11-20", "2026-11-20", "invalid"}, {"2026-1-1", "2026-01-02", "UTC"}} {
		if _, err := domain.TimeOffInterval(tc[0], tc[1], tc[2]); err == nil {
			t.Fatal("accepted", tc)
		}
	}
}
