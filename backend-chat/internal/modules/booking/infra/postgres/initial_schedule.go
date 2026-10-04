package postgres

import (
	"context"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/google/uuid"
)

// ConfigureInitialSchedule implements catalog's onboarding port. Booking owns
// the policy/calendar and validates them before persistence in the shared tx.
func (r *Repository) ConfigureInitialSchedule(ctx context.Context, input catalog.InitialSchedule) error {
	policy, err := domain.NewSlotPolicy(input.IntervalMinutes, input.BufferMinutes)
	if err != nil || input.ShopID == uuid.Nil || input.ProfessionalID == uuid.Nil || len(input.Weekdays) == 0 || len(input.Weekdays) > 7 {
		return catalog.ErrInvalidProfessionalSchedule
	}
	starts, err := policy.CandidateStartMinutes(input.OpensAtMinute, input.ClosesAtMinute, 1)
	if err != nil || len(starts) == 0 {
		return catalog.ErrInvalidProfessionalSchedule
	}
	seen := map[int]bool{}
	weekdays := make([]int32, 0, len(input.Weekdays))
	for _, day := range input.Weekdays {
		if day < 0 || day > 6 || seen[day] {
			return catalog.ErrInvalidProfessionalSchedule
		}
		seen[day] = true
		weekdays = append(weekdays, int32(day))
	}
	if err := r.LockProfessionalSchedule(ctx, input.ShopID, input.ProfessionalID); err != nil {
		return err
	}
	if err := r.queries.InsertInitialSlotPolicy(ctx, db.InsertInitialSlotPolicyParams{ShopID: input.ShopID, ProfessionalID: input.ProfessionalID, IntervalMinutes: int32(input.IntervalMinutes), BufferMinutes: int32(input.BufferMinutes)}); err != nil {
		return err
	}
	return r.queries.InsertInitialWorkingWeek(ctx, db.InsertInitialWorkingWeekParams{ShopID: input.ShopID, ProfessionalID: input.ProfessionalID,
		Weekdays: weekdays, OpensAtMinute: int32(input.OpensAtMinute), ClosesAtMinute: int32(input.ClosesAtMinute)})
}
