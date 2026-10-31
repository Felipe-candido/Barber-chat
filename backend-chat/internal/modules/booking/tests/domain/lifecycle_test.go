package domain_test

import (
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestStaffAppointmentTransitions(t *testing.T) {
	start := time.Date(2026, 10, 31, 9, 30, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	a := domain.Lifecycle{ID: uuid.New(), ShopID: uuid.New(), ProfessionalID: uuid.New(), StartsAt: start, EndsAt: end, UpdatedAt: start.Add(-time.Hour), Status: domain.StatusScheduled, Version: 1}
	actor := uuid.New()
	for _, tc := range []struct {
		name    string
		status  domain.Status
		now     time.Time
		version int64
		reason  string
		want    error
	}{
		{"complete before end", domain.StatusCompleted, end.Add(-time.Nanosecond), 1, "Finished", domain.ErrAppointmentTransitionTooEarly},
		{"complete at end", domain.StatusCompleted, end, 1, "Finished", nil},
		{"absence before start", domain.StatusNoShow, start.Add(-time.Nanosecond), 1, "Did not attend", domain.ErrAppointmentTransitionTooEarly},
		{"absence at start", domain.StatusNoShow, start, 1, "Did not attend", nil},
		{"cancel future", domain.StatusCancelled, start.Add(-time.Minute), 1, "Cancelled", nil},
		{"stale version", domain.StatusCancelled, start, 2, "Changed", domain.ErrAppointmentVersionConflict},
		{"missing reason", domain.StatusCancelled, start, 1, " ", domain.ErrInvalidStatusChange},
		{"control in reason", domain.StatusCancelled, start, 1, "Wrong\nitem", domain.ErrInvalidStatusChange},
		{"cannot revive future slot", domain.StatusScheduled, end, 1, "Restore", domain.ErrInvalidStatusChange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := a.ChangeByStaff(actor, tc.status, tc.reason, tc.version, tc.now)
			if !errors.Is(err, tc.want) {
				t.Fatal(err, tc.want)
			}
			if err == nil && (out.Source != domain.SourceStaff || out.ActorID != actor || out.Version != 2 || out.ID != a.ID || out.EndsAt != a.EndsAt) {
				t.Fatal("transition changed ownership or omitted audit", out)
			}
		})
	}
	completed, err := a.ChangeByStaff(actor, domain.StatusCompleted, "Recorded", 1, end)
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := completed.ChangeByStaff(actor, domain.StatusNoShow, "Wrong attendance", 2, end.Add(time.Minute))
	if err != nil || corrected.Version != 3 {
		t.Fatal(corrected, err)
	}
	if _, err := corrected.ChangeByStaff(actor, domain.StatusNoShow, "Again", 3, end.Add(time.Minute)); !errors.Is(err, domain.ErrAppointmentAlreadyInStatus) {
		t.Fatal(err)
	}
}
