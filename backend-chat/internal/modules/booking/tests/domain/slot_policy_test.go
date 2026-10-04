package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
)

func TestNewSlotPolicy(t *testing.T) {
	for _, test := range []struct {
		name             string
		interval, buffer int
		valid            bool
	}{
		{"hourly", 60, 0, true},
		{"flexible grid", 45, 10, true},
		{"minimum interval", 1, 0, true},
		{"maximum configuration", 1440, 1440, true},
		{"zero interval", 0, 0, false},
		{"negative interval", -1, 0, false},
		{"interval beyond one day", 1441, 0, false},
		{"negative buffer", 60, -1, false},
		{"buffer beyond one day", 60, 1441, false},
		{"huge interval", int(^uint(0) >> 1), 0, false},
		{"huge buffer", 60, int(^uint(0) >> 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, err := domain.NewSlotPolicy(test.interval, test.buffer)
			if !test.valid {
				if !errors.Is(err, domain.ErrInvalidSlotPolicy) {
					t.Fatalf("got %v, want invalid slot policy", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if policy.IntervalMinutes() != test.interval || policy.BufferMinutes() != test.buffer {
				t.Fatal("policy did not preserve server configuration")
			}
		})
	}
}

func TestReservationMinutes(t *testing.T) {
	for _, test := range []struct {
		name                                string
		interval, buffer, service, reserved int
	}{
		{"short service occupies whole hour", 60, 0, 40, 60},
		{"exact hour occupies one block", 60, 0, 60, 60},
		{"long service occupies two blocks", 60, 0, 80, 120},
		{"exact two hours", 60, 0, 120, 120},
		{"one extra minute occupies third block", 60, 0, 121, 180},
		{"half-hour grid", 30, 0, 40, 60},
		{"forty-five-minute grid", 45, 0, 80, 90},
		{"explicit margin fits existing block", 60, 20, 40, 60},
		{"margin requires another block", 60, 21, 40, 120},
		{"full-hour service with margin", 60, 10, 60, 120},
		{"smallest service", 1, 0, 1, 1},
		{"whole-day service", 60, 0, 1440, 1440},
		{"padded result exceeds one day", 1440, 1440, 1440, 2880},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := newSlotPolicy(t, test.interval, test.buffer)
			reserved, err := policy.ReservationMinutes(test.service)
			if err != nil {
				t.Fatal(err)
			}
			if reserved != test.reserved {
				t.Fatalf("reserved %d minutes, want %d", reserved, test.reserved)
			}
		})
	}

	policy := newSlotPolicy(t, 60, 0)
	for _, duration := range []int{0, -1, 1441, int(^uint(0) >> 1)} {
		if _, err := policy.ReservationMinutes(duration); !errors.Is(err, domain.ErrInvalidServiceDuration) {
			t.Errorf("duration %d: got %v, want invalid service duration", duration, err)
		}
	}
}

func TestCandidateStartMinutes(t *testing.T) {
	for _, test := range []struct {
		name                                        string
		interval, buffer, opening, closing, service int
		want                                        []int
	}{
		{"hourly from 09:30", 60, 0, 570, 810, 40, []int{570, 630, 690, 750}},
		{"two-block service fits whole reservation", 60, 0, 570, 810, 80, []int{570, 630, 690}},
		{"exact fit at closing", 60, 0, 570, 630, 40, []int{570}},
		{"service fits but rounded reservation does not", 60, 0, 570, 615, 40, []int{}},
		{"long service cannot fit", 60, 0, 570, 630, 80, []int{}},
		{"buffer removes late starts", 60, 21, 570, 810, 40, []int{570, 630, 690}},
		{"flexible anchored grid", 45, 0, 570, 750, 40, []int{570, 615, 660, 705}},
		{"afternoon opening anchors its own window", 60, 0, 780, 960, 40, []int{780, 840, 900}},
		{"midnight closing", 60, 0, 1350, 1440, 40, []int{1350}},
		{"whole-day block", 1440, 0, 0, 1440, 40, []int{0}},
		{"padded result cannot fit one day", 1440, 1440, 0, 1440, 1440, []int{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := newSlotPolicy(t, test.interval, test.buffer)
			starts, err := policy.CandidateStartMinutes(test.opening, test.closing, test.service)
			if err != nil {
				t.Fatal(err)
			}
			if starts == nil || !slices.Equal(starts, test.want) {
				t.Fatalf("starts=%v, want non-nil %v", starts, test.want)
			}
		})
	}
}

func TestAllowsStart(t *testing.T) {
	policy := newSlotPolicy(t, 60, 0)
	for _, test := range []struct {
		name           string
		start, service int
		allowed        bool
	}{
		{"opening", 570, 40, true},
		{"next grid start", 630, 40, true},
		{"last complete block", 750, 40, true},
		{"before opening", 510, 40, false},
		{"off-grid start at 10:00", 600, 40, false},
		{"at closing", 810, 40, false},
		{"after closing", 870, 40, false},
		{"long service fits", 690, 80, true},
		{"long service crosses closing", 750, 80, false},
		{"invalid civil minute", -1, 40, false},
		{"huge start is safely rejected", int(^uint(0) >> 1), 40, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			allowed, err := policy.AllowsStart(test.start, 570, 810, test.service)
			if err != nil {
				t.Fatal(err)
			}
			if allowed != test.allowed {
				t.Fatalf("allowed=%t, want %t", allowed, test.allowed)
			}
		})
	}
}

func TestSlotPolicyRejectsInvalidWindowsAndDurations(t *testing.T) {
	policy := newSlotPolicy(t, 60, 0)
	for _, test := range []struct {
		name                      string
		opening, closing, service int
		want                      error
	}{
		{"negative opening", -1, 630, 40, domain.ErrInvalidWorkingWindow},
		{"closing beyond midnight", 570, 1441, 40, domain.ErrInvalidWorkingWindow},
		{"empty window", 570, 570, 40, domain.ErrInvalidWorkingWindow},
		{"overnight window", 1350, 60, 40, domain.ErrInvalidWorkingWindow},
		{"missing duration", 570, 810, 0, domain.ErrInvalidServiceDuration},
		{"negative duration", 570, 810, -40, domain.ErrInvalidServiceDuration},
		{"multi-day service", 570, 810, 1441, domain.ErrInvalidServiceDuration},
	} {
		t.Run(test.name, func(t *testing.T) {
			starts, err := policy.CandidateStartMinutes(test.opening, test.closing, test.service)
			if !errors.Is(err, test.want) || starts != nil {
				t.Fatalf("starts=%v err=%v, want nil and %v", starts, err, test.want)
			}
			allowed, err := policy.AllowsStart(test.opening, test.opening, test.closing, test.service)
			if !errors.Is(err, test.want) || allowed {
				t.Fatalf("allowed=%t err=%v, want false and %v", allowed, err, test.want)
			}
		})
	}
}

func TestZeroSlotPolicyIsNotUsable(t *testing.T) {
	var policy domain.SlotPolicy
	if _, err := policy.ReservationMinutes(40); !errors.Is(err, domain.ErrInvalidSlotPolicy) {
		t.Fatal("zero policy calculated a reservation", err)
	}
	if _, err := policy.CandidateStartMinutes(570, 810, 40); !errors.Is(err, domain.ErrInvalidSlotPolicy) {
		t.Fatal("zero policy generated candidates", err)
	}
	if _, err := policy.AllowsStart(570, 570, 810, 40); !errors.Is(err, domain.ErrInvalidSlotPolicy) {
		t.Fatal("zero policy accepted a start", err)
	}
}

func TestCandidateGenerationAndValidationAgree(t *testing.T) {
	for _, interval := range []int{1, 30, 45, 60, 90} {
		policy := newSlotPolicy(t, interval, 10)
		starts, err := policy.CandidateStartMinutes(570, 810, 80)
		if err != nil {
			t.Fatal(err)
		}
		for start := 0; start <= 1440; start++ {
			allowed, err := policy.AllowsStart(start, 570, 810, 80)
			if err != nil {
				t.Fatal(err)
			}
			if allowed != slices.Contains(starts, start) {
				t.Fatalf("interval=%d start=%d: generation and validation disagree", interval, start)
			}
		}
	}
}

func newSlotPolicy(t *testing.T, interval, buffer int) domain.SlotPolicy {
	t.Helper()
	policy, err := domain.NewSlotPolicy(interval, buffer)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
