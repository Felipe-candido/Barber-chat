package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
)

func TestScheduleConfirmationReminderAndRetention(t *testing.T) {
	created := time.Date(2026, 11, 1, 12, 30, 0, 0, time.FixedZone("shop", -3*3600))
	starts := created.Add(72 * time.Hour)
	p := domain.DefaultPolicy()
	if !p.Valid() || p.Enabled || p.ReminderLeadMinutes != 1440 {
		t.Fatal("default policy must be valid and disabled")
	}
	p.Enabled, p.Version = true, 3
	s, err := p.Schedule(created, starts)
	if err != nil || s.PolicyVersion != 3 || s.SkipReason != "" || s.ReminderSkipReason != "" {
		t.Fatal(s, err)
	}
	if !s.Confirmation.DueAt.Equal(created) || !s.Confirmation.ExpiresAt.Equal(created.Add(time.Hour)) ||
		!s.Reminder.DueAt.Equal(starts.Add(-24*time.Hour)) || !s.Reminder.ExpiresAt.Equal(starts.Add(-23*time.Hour)) ||
		!s.RetainUntil().Equal(s.Reminder.ExpiresAt) {
		t.Fatal("incorrect dispatch or retention window", s)
	}
	for _, instant := range []time.Time{s.Confirmation.DueAt, s.Confirmation.ExpiresAt, s.Reminder.DueAt, s.Reminder.ExpiresAt} {
		if instant.Location() != time.UTC {
			t.Fatal("planned instants must be UTC")
		}
	}
}

func TestScheduleDoesNotSendConfirmationAndReminderTogether(t *testing.T) {
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	p := domain.Policy{Enabled: true, ReminderLeadMinutes: 1440, Version: 1}
	for _, ahead := range []time.Duration{time.Minute, 3 * time.Hour, 24 * time.Hour} {
		t.Run(ahead.String(), func(t *testing.T) {
			s, err := p.Schedule(created, created.Add(ahead))
			if err != nil || !s.Confirmation.Valid() || s.Reminder.Valid() || s.ReminderSkipReason != domain.SkipInsufficientLead {
				t.Fatal(s, err)
			}
			if s.Confirmation.ExpiresAt.After(created.Add(ahead)) || !s.RetainUntil().Equal(s.Confirmation.ExpiresAt) {
				t.Fatal("confirmation outlives the appointment start")
			}
		})
	}
}

func TestConfigurableLeadIncludesMinimumAndMaximum(t *testing.T) {
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	starts := created.Add(10 * 24 * time.Hour)
	for _, minutes := range []int{5, 60, 1440, 7 * 24 * 60} {
		t.Run((time.Duration(minutes) * time.Minute).String(), func(t *testing.T) {
			p := domain.Policy{Enabled: true, ReminderLeadMinutes: minutes, Version: 1}
			s, err := p.Schedule(created, starts)
			if err != nil || !s.Reminder.Valid() || !s.Reminder.DueAt.Equal(starts.Add(-time.Duration(minutes)*time.Minute)) {
				t.Fatal(s, err)
			}
			if !s.Reminder.ExpiresAt.Before(starts) || s.Reminder.ExpiresAt.Sub(s.Reminder.DueAt) > time.Hour {
				t.Fatal("reminder deadline outside permitted window")
			}
			if minutes == 5 && s.Reminder.ExpiresAt.Sub(s.Reminder.DueAt) != 4*time.Minute {
				t.Fatal("minimum lead must retain a usable delivery window")
			}
		})
	}
}

func TestWindowBoundariesAndExpiredWork(t *testing.T) {
	due := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	w := domain.Window{DueAt: due, ExpiresAt: due.Add(time.Hour)}
	for _, tc := range []struct {
		now      time.Time
		eligible bool
	}{
		{time.Time{}, false}, {due.Add(-time.Nanosecond), false}, {due, true},
		{w.ExpiresAt.Add(-time.Nanosecond), true}, {w.ExpiresAt, false}, {due.Add(48 * time.Hour), false},
	} {
		if w.EligibleAt(tc.now) != tc.eligible {
			t.Fatal("incorrect half-open dispatch window", tc)
		}
	}
	for _, invalid := range []domain.Window{{}, {DueAt: due}, {ExpiresAt: due}, {DueAt: due, ExpiresAt: due}, {DueAt: due, ExpiresAt: due.Add(-time.Hour)}} {
		if invalid.Valid() || invalid.EligibleAt(due) {
			t.Fatal("invalid window became eligible")
		}
	}
}

func TestReminderUsesElapsedTimeAcrossDST(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	starts := time.Date(2026, 11, 1, 9, 30, 0, 0, zone)
	p := domain.Policy{Enabled: true, ReminderLeadMinutes: 1440, Version: 1}
	s, err := p.Schedule(starts.Add(-72*time.Hour), starts)
	if err != nil || starts.Sub(s.Reminder.DueAt) != 24*time.Hour || s.Reminder.DueAt.In(zone).Hour() != 10 {
		t.Fatal("reminder must use elapsed hours even when civil offsets change", s, err)
	}
}

func TestScheduleMatchesPostgresTimestampPrecision(t *testing.T) {
	created := time.Date(2026, 11, 1, 12, 0, 0, 123456789, time.UTC)
	p := domain.Policy{Enabled: true, ReminderLeadMinutes: 1440, Version: 1}
	s, err := p.Schedule(created, created.Add(72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{s.Confirmation.DueAt, s.Confirmation.ExpiresAt, s.Reminder.DueAt, s.RetainUntil()} {
		if at.Nanosecond()%1000 != 0 {
			t.Fatal("scope would change after a PostgreSQL timestamp round trip")
		}
	}
}

func TestScheduleRejectsInvalidPoliciesAndDates(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for _, p := range []domain.Policy{{}, {Enabled: true, ReminderLeadMinutes: 4, Version: 1},
		{ReminderLeadMinutes: 10081, Version: 1}, {ReminderLeadMinutes: 1440, Version: -1}} {
		if _, err := p.Schedule(now, now.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidPolicy) {
			t.Fatal("invalid policy accepted", err)
		}
	}
	p := domain.Policy{Enabled: true, ReminderLeadMinutes: 1440, Version: 1}
	for _, dates := range [][2]time.Time{{{}, now}, {now, {}}, {now, now}, {now, now.Add(-time.Hour)},
		{now, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}} {
		if _, err := p.Schedule(dates[0], dates[1]); !errors.Is(err, domain.ErrInvalidSchedule) {
			t.Fatal("invalid schedule accepted", err)
		}
	}
	s, err := domain.DefaultPolicy().Schedule(time.Time{}, time.Time{})
	if err != nil || s.SkipReason != domain.SkipPolicyDisabled || s.Confirmation.Valid() || !s.RetainUntil().IsZero() {
		t.Fatal("disabled policy must not plan work", err)
	}
}
