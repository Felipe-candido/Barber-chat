package domain

import (
	"errors"
	"time"
)

var ErrInvalidTimeOff = errors.New("invalid time off dates")

// TimeOffInterval includes both civil dates and respects 23/25-hour days.
// Scanning also handles zones whose daylight saving transition occurs at midnight.
func TimeOffInterval(from, through, timezone string) (Interval, error) {
	first, err := time.Parse("2006-01-02", from)
	if err != nil || first.Format("2006-01-02") != from || first.Year() < 1900 || first.Year() > 9997 {
		return Interval{}, ErrInvalidTimeOff
	}
	last, err := time.Parse("2006-01-02", through)
	if err != nil || last.Format("2006-01-02") != through || last.Before(first) || last.After(first.AddDate(0, 0, 365)) {
		return Interval{}, ErrInvalidTimeOff
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		return Interval{}, ErrInvalidTimeOff
	}
	boundary := func(day time.Time) time.Time {
		key := day.Format("2006-01-02")
		for t := day.Add(-24 * time.Hour); t.Before(day.Add(48 * time.Hour)); t = t.Add(time.Minute) {
			if t.In(loc).Format("2006-01-02") >= key {
				return t
			}
		}
		return time.Time{}
	}
	start, end := boundary(first), boundary(last.AddDate(0, 0, 1))
	if start.IsZero() || !end.After(start) || start.In(loc).Format("2006-01-02") != from {
		return Interval{}, ErrInvalidTimeOff
	}
	return Interval{StartsAt: start, EndsAt: end}, nil
}
