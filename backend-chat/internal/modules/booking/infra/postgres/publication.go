package postgres

import (
	"context"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/google/uuid"
)

// Readiness checks recurring configuration, NOT whether a slot is free today.
// An entirely occupied week must not unpublish an otherwise configured shop.
func (r *Repository) HasPublicationSchedule(ctx context.Context, id uuid.UUID, candidates []shops.PublicationCandidate) (bool, error) {
	durations := make(map[uuid.UUID]int, len(candidates))
	for _, candidate := range candidates {
		durations[candidate.ProfessionalID] = candidate.DurationMinutes
	}
	rows, err := r.queries.ListPublicationWorkingWindows(ctx, id)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		duration, exists := durations[row.ProfessionalID]
		if !exists {
			continue
		}
		policy, err := domain.NewSlotPolicy(int(row.IntervalMinutes), int(row.BufferMinutes))
		if err != nil {
			return false, err
		}
		fits, err := policy.AllowsStart(int(row.OpensAtMinute), int(row.OpensAtMinute), int(row.ClosesAtMinute), duration)
		if err != nil {
			return false, err
		}
		if fits {
			return true, nil
		}
	}
	return false, nil
}
