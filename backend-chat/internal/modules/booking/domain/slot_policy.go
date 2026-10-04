package domain

import "errors"

const minutesPerDay = 24 * 60

var (
	ErrInvalidSlotPolicy      = errors.New("slot interval must be between 1 and 1440 minutes and buffer between 0 and 1440 minutes")
	ErrInvalidServiceDuration = errors.New("bookable service duration must be between 1 and 1440 minutes")
	ErrInvalidWorkingWindow   = errors.New("working window must have 0 <= opening < closing <= 1440")
)

// SlotPolicy separates service duration from calendar occupancy. Starts follow
// a configurable grid anchored to the working window's opening, not midnight.
// Its fields are private; the zero value is invalid and methods reject it.
type SlotPolicy struct {
	intervalMinutes int
	bufferMinutes   int
}

// NewSlotPolicy receives server-owned calendar configuration. A 60-minute grid
// with buffer 0 already reserves 60 minutes for a 40-minute service. Buffer adds
// a minimum margin BEFORE rounding, so 60 minutes plus buffer 10 reserves 120.
func NewSlotPolicy(intervalMinutes, bufferMinutes int) (SlotPolicy, error) {
	policy := SlotPolicy{intervalMinutes: intervalMinutes, bufferMinutes: bufferMinutes}
	if err := policy.validate(); err != nil {
		return SlotPolicy{}, err
	}
	return policy, nil
}

func (p SlotPolicy) IntervalMinutes() int { return p.intervalMinutes }

func (p SlotPolicy) BufferMinutes() int { return p.bufferMinutes }

// ReservationMinutes rounds service plus buffer UP to whole grid blocks.
// This initial calendar supports same-day services; a result can exceed one day
// after padding, in which case it cannot fit a same-day working window.
func (p SlotPolicy) ReservationMinutes(serviceMinutes int) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	if serviceMinutes < 1 || serviceMinutes > minutesPerDay {
		return 0, ErrInvalidServiceDuration
	}

	required := serviceMinutes + p.bufferMinutes
	blocks := (required + p.intervalMinutes - 1) / p.intervalMinutes
	return blocks * p.intervalMinutes, nil
}

// CandidateStartMinutes returns civil minutes since midnight, not UTC instants
// or confirmed availability. For example, 09:30 is 570; closing may be 1440.
// The entire RESERVED interval must fit the window. Overlapping candidates are
// intentional: choosing one later makes conflicting candidates unavailable.
// Callers must resolve the shop's named timezone, handle ambiguous/nonexistent
// civil times, and check breaks, blocks and occupied intervals before publishing.
// Overnight windows and timezone conversion are deliberately not handled here.
func (p SlotPolicy) CandidateStartMinutes(opening, closing, serviceMinutes int) ([]int, error) {
	reserved, err := p.reservationForWindow(opening, closing, serviceMinutes)
	if err != nil {
		return nil, err
	}
	if reserved > closing-opening {
		return []int{}, nil
	}

	latest := closing - reserved
	starts := make([]int, 0, (latest-opening)/p.intervalMinutes+1)
	for start := opening; start <= latest; start += p.intervalMinutes {
		starts = append(starts, start)
	}
	return starts, nil
}

// AllowsStart checks the same civil grid and whole-window fit used by candidate
// generation. It does not check other appointments or reserve the selected slot.
// HTTP/application must separately reject seconds/subseconds and invalid dates.
func (p SlotPolicy) AllowsStart(start, opening, closing, serviceMinutes int) (bool, error) {
	reserved, err := p.reservationForWindow(opening, closing, serviceMinutes)
	if err != nil {
		return false, err
	}
	if reserved > closing-opening || start < opening || start > closing-reserved {
		return false, nil
	}
	return (start-opening)%p.intervalMinutes == 0, nil
}

func (p SlotPolicy) reservationForWindow(opening, closing, serviceMinutes int) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	if opening < 0 || opening >= closing || closing > minutesPerDay {
		return 0, ErrInvalidWorkingWindow
	}
	return p.ReservationMinutes(serviceMinutes)
}

func (p SlotPolicy) validate() error {
	if p.intervalMinutes < 1 || p.intervalMinutes > minutesPerDay ||
		p.bufferMinutes < 0 || p.bufferMinutes > minutesPerDay {
		return ErrInvalidSlotPolicy
	}
	return nil
}
