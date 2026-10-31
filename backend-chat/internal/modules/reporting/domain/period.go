package domain

import "time"

// MaxPeriodDays bounds synchronous reads in the first reporting increment.
// Larger exports need a separate, bounded workflow rather than unlimited queries.
const MaxPeriodDays = 366

// Period is a half-open interval of UTC instants resolved from civil shop dates.
// The zero value is invalid. Use NewPeriod instead of constructing raw bounds.
type Period struct {
	from, until time.Time
	timezone    string
}

// NewPeriod includes both requested dates. Timezone must come from the shop,
// not the browser. Advancing a civil date is not adding a fixed 24 hours.
func NewPeriod(fromDate, throughDate, timezone string) (Period, error) {
	if timezone == "" || timezone == "Local" {
		return Period{}, ErrInvalidPeriod
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Period{}, ErrInvalidPeriod
	}
	from, err := time.ParseInLocation(time.DateOnly, fromDate, location)
	if err != nil || from.Format(time.DateOnly) != fromDate {
		return Period{}, ErrInvalidPeriod
	}
	through, err := time.ParseInLocation(time.DateOnly, throughDate, location)
	if err != nil || through.Format(time.DateOnly) != throughDate || through.Before(from) ||
		from.Year() < 1900 || through.Year() > 9998 {
		return Period{}, ErrInvalidPeriod
	}
	// Count calendar dates in UTC; actual shop days can last 23 or 25 hours.
	firstDate, _ := time.Parse(time.DateOnly, fromDate)
	lastDate, _ := time.Parse(time.DateOnly, throughDate)
	days := int(lastDate.Sub(firstDate)/(24*time.Hour)) + 1
	if days > MaxPeriodDays {
		return Period{}, ErrInvalidPeriod
	}
	until := through.AddDate(0, 0, 1)
	if !until.After(from) {
		return Period{}, ErrInvalidPeriod
	}
	return Period{from: from.UTC(), until: until.UTC(), timezone: timezone}, nil
}

func (p Period) From() time.Time  { return p.from }
func (p Period) Until() time.Time { return p.until }
func (p Period) Timezone() string { return p.timezone }
func (p Period) Valid() bool {
	return p.timezone != "" && !p.from.IsZero() && p.until.After(p.from)
}
