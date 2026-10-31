package domain

import (
	"sort"
	"time"
)

// Coverage is a confirmed paid interval, not the next invoicing period.
type Coverage struct {
	From  time.Time `json:"from"`
	Until time.Time `json:"until"`
}

func (p Coverage) Valid() bool { return !p.From.IsZero() && p.Until.After(p.From) }

// CoveredUntil coalesces touching intervals but never bridges unpaid gaps.
// It copies before sorting so evaluating access cannot mutate a shared snapshot.
func CoveredUntil(periods []Coverage, now time.Time) time.Time {
	ordered := append([]Coverage(nil), periods...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].From.Before(ordered[j].From) })
	var group Coverage
	for _, p := range ordered {
		if group.From.IsZero() || p.From.After(group.Until) {
			if !group.From.IsZero() && !now.Before(group.From) && now.Before(group.Until) {
				return group.Until.UTC()
			}
			group = p
		} else if p.Until.After(group.Until) {
			group.Until = p.Until
		}
	}
	if !group.From.IsZero() && !now.Before(group.From) && now.Before(group.Until) {
		return group.Until.UTC()
	}
	return time.Time{}
}
