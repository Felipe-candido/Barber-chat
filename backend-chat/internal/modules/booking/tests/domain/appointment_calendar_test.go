package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

func TestAppointmentLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	a := domain.Appointment{ID: uuid.New(), ShopID: uuid.New(), CustomerID: uuid.New(), ServiceID: uuid.New(), ProfessionalID: uuid.New(),
		Service:          domain.ServiceSnapshot{Name: "Haircut", DurationMinutes: 40, PriceCents: 5000, Currency: "BRL"},
		ProfessionalName: "Barber", ShopTimezone: "America/Sao_Paulo", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)}
	created, err := domain.NewAppointment(a, now)
	if err != nil || created.Status != domain.StatusScheduled || created.Version != 1 {
		t.Fatalf("creation: %+v %v", created, err)
	}
	for _, mutate := range []func(*domain.Appointment){
		func(a *domain.Appointment) { a.ShopID = uuid.Nil }, func(a *domain.Appointment) { a.EndsAt = a.StartsAt.Add(39 * time.Minute) },
		func(a *domain.Appointment) { a.StartsAt = now }, func(a *domain.Appointment) { a.StartsAt = a.StartsAt.Add(time.Second) },
		func(a *domain.Appointment) { a.ShopTimezone = "invalid/zone" }, func(a *domain.Appointment) { a.Service.PriceCents = -1 },
	} {
		invalid := a
		mutate(&invalid)
		if _, err := domain.NewAppointment(invalid, now); !errors.Is(err, domain.ErrInvalidAppointment) {
			t.Fatalf("invalid accepted: %v", err)
		}
	}
	cancelled, err := created.Cancel(now)
	if err != nil || cancelled.Status != domain.StatusCancelled || cancelled.Version != 2 {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	repeated, err := cancelled.Cancel(now.Add(24 * time.Hour))
	if err != nil || repeated != cancelled {
		t.Fatal("cancellation is not idempotent")
	}
	if _, err := created.Cancel(created.StartsAt); !errors.Is(err, domain.ErrCancellationClosed) {
		t.Fatalf("cancel at start: %v", err)
	}
	created.Status = domain.StatusCompleted
	if created.CanCancel(now) {
		t.Fatal("completed appointment can be cancelled")
	}
}

func TestCalendarReservedIntervalAndTimezone(t *testing.T) {
	policy, _ := domain.NewSlotPolicy(60, 0)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	calendar := domain.Calendar{Policy: policy, Windows: []domain.WorkingWindow{{OpensAtMinute: 570, ClosesAtMinute: 810}},
		Busy: []domain.Interval{{StartsAt: time.Date(2026, 10, 2, 13, 30, 0, 0, time.UTC), EndsAt: time.Date(2026, 10, 2, 14, 30, 0, 0, time.UTC)}}}
	slots, err := calendar.AvailableSlots("2026-10-02", "America/Sao_Paulo", 80, now)
	if err != nil || len(slots) != 1 || slots[0].StartsAt.Hour() != 14 || slots[0].StartsAt.Minute() != 30 || slots[0].EndsAt.Hour() != 16 {
		t.Fatalf("slots: %+v %v", slots, err)
	}
	calendar.Busy = nil
	slots, err = calendar.AvailableSlots("2026-10-02", "America/Sao_Paulo", 40, now)
	if err != nil || len(slots) != 4 || slots[0].StartsAt.Hour() != 12 {
		t.Fatalf("grid: %+v %v", slots, err)
	}
	slots, err = calendar.AvailableSlots("2026-10-02", "America/Sao_Paulo", 40, slots[0].StartsAt)
	if err != nil || len(slots) != 3 {
		t.Fatalf("past slot: %+v %v", slots, err)
	}
}

func TestCalendarRejectsDSTAmbiguityAndNonexistentTimes(t *testing.T) {
	policy, _ := domain.NewSlotPolicy(30, 0)
	calendar := domain.Calendar{Policy: policy, Windows: []domain.WorkingWindow{{OpensAtMinute: 60, ClosesAtMinute: 240}}}
	for _, date := range []string{"2026-03-08", "2026-11-01"} {
		slots, err := calendar.AvailableSlots(date, "America/New_York", 30, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		location, _ := time.LoadLocation("America/New_York")
		for _, slot := range slots {
			start, end := slot.StartsAt.In(location), slot.EndsAt.In(location)
			if date == "2026-03-08" && (start.Hour() == 2 || end.Hour() == 2 || end.Sub(start) != 30*time.Minute) {
				t.Fatalf("nonexistent time: %+v", slot)
			}
			if date == "2026-11-01" && (start.Hour() == 1 || end.Hour() == 1) {
				t.Fatalf("ambiguous time: %+v", slot)
			}
		}
		if len(slots) == 0 {
			t.Fatal("ordinary later slots missing")
		}
	}
}
