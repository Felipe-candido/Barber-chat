package domain

import (
	"errors"
	"sort"
	"time"
)

var ErrInvalidCalendar = errors.New("invalid calendar")

type WorkingWindow struct{ OpensAtMinute, ClosesAtMinute int }
type Interval struct{ StartsAt, EndsAt time.Time }
type Calendar struct {
	Policy  SlotPolicy
	Windows []WorkingWindow
	Busy    []Interval
}

// AvailableSlots translates the local grid to UTC and filters the whole reserved
// interval. Ambiguous/nonexistent civil minutes and DST-crossing reservations are
// deliberately omitted instead of silently shifting a customer's appointment.
func (c Calendar) AvailableSlots(date, timezone string, serviceMinutes int, now time.Time) ([]Interval, error) {
	day, err := time.Parse("2006-01-02", date)
	if err != nil || timezone == "" {
		return nil, ErrInvalidCalendar
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, ErrInvalidCalendar
	}
	reserved, err := c.Policy.ReservationMinutes(serviceMinutes)
	if err != nil {
		return nil, err
	}
	// A bounded scan also detects repeated local minutes when clocks turn back.
	// Contemporary IANA offsets fit within this UTC range around the civil date.
	instants := make(map[int]time.Time, 1441)
	ambiguous := make(map[int]bool)
	nextDate := day.AddDate(0, 0, 1).Format("2006-01-02")
	for instant := day.Add(-24 * time.Hour); !instant.After(day.Add(48 * time.Hour)); instant = instant.Add(time.Minute) {
		local := instant.In(location)
		if local.Second() != 0 {
			continue
		}
		minute := local.Hour()*60 + local.Minute()
		localDate := local.Format("2006-01-02")
		if localDate == nextDate && minute == 0 {
			minute = 1440
		} else if localDate != date {
			continue
		}
		if _, exists := instants[minute]; exists {
			ambiguous[minute] = true
		}
		instants[minute] = instant
	}
	result := make([]Interval, 0)
	seen := make(map[time.Time]bool)
	for _, window := range c.Windows {
		starts, err := c.Policy.CandidateStartMinutes(window.OpensAtMinute, window.ClosesAtMinute, serviceMinutes)
		if err != nil {
			return nil, err
		}
		for _, minute := range starts {
			start, startOK := instants[minute]
			end, endOK := instants[minute+reserved]
			if !startOK || !endOK || ambiguous[minute] || ambiguous[minute+reserved] ||
				!start.After(now) || end.Sub(start) != time.Duration(reserved)*time.Minute || seen[start] {
				continue
			}
			free := true
			for _, busy := range c.Busy {
				if !busy.EndsAt.After(busy.StartsAt) {
					return nil, ErrInvalidCalendar
				}
				if start.Before(busy.EndsAt) && busy.StartsAt.Before(end) {
					free = false
					break
				}
			}
			if free {
				result = append(result, Interval{StartsAt: start, EndsAt: end})
				seen[start] = true
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartsAt.Before(result[j].StartsAt) })
	return result, nil
}
