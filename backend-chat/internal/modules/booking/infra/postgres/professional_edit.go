package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReadProfessionalSchedule(ctx context.Context, shop, id uuid.UUID) (catalog.ProfessionalSchedule, error) {
	s := catalog.ProfessionalSchedule{IntervalMinutes: 60, WorkingHours: make([]catalog.WorkingPeriod, 0)}
	policy, err := r.queries.GetSlotPolicy(ctx, db.GetSlotPolicyParams{ShopID: shop, ProfessionalID: id})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	if err == nil {
		s.IntervalMinutes = int(policy.IntervalMinutes)
		s.BufferMinutes = int(policy.BufferMinutes)
	}
	rows, err := r.queries.ReadProfessionalWorkingHours(ctx, db.ReadProfessionalWorkingHoursParams{ShopID: shop, ProfessionalID: id})
	if err != nil {
		return s, err
	}
	for _, w := range rows {
		s.WorkingHours = append(s.WorkingHours, catalog.WorkingPeriod{Weekday: int(w.Weekday), OpensAtMinute: int(w.OpensAtMinute), ClosesAtMinute: int(w.ClosesAtMinute)})
	}
	return s, nil
}

// Called after catalog has locked the professional in the shared transaction.
// Only weekly hours and policy change. Appointments and time-off blocks are untouched.
func (r *Repository) ReplaceProfessionalSchedule(ctx context.Context, shop, id uuid.UUID, s catalog.ProfessionalSchedule) error {
	if _, err := domain.NewSlotPolicy(s.IntervalMinutes, s.BufferMinutes); err != nil {
		return catalog.ErrInvalidProfessionalSchedule
	}
	if err := r.queries.ReplaceProfessionalSlotPolicy(ctx, db.ReplaceProfessionalSlotPolicyParams{ShopID: shop, ProfessionalID: id, IntervalMinutes: int32(s.IntervalMinutes), BufferMinutes: int32(s.BufferMinutes)}); err != nil {
		return err
	}
	if err := r.queries.RemoveProfessionalWorkingHours(ctx, db.RemoveProfessionalWorkingHoursParams{ShopID: shop, ProfessionalID: id}); err != nil {
		return err
	}
	for _, w := range s.WorkingHours {
		if err := r.queries.InsertInitialWorkingHours(ctx, db.InsertInitialWorkingHoursParams{ID: uuid.New(), ShopID: shop, ProfessionalID: id, Weekday: int32(w.Weekday), OpensAtMinute: int32(w.OpensAtMinute), ClosesAtMinute: int32(w.ClosesAtMinute)}); err != nil {
			return err
		}
	}
	return nil
}
