package domain

import (
	"errors"
	"time"
)

const (
	DefaultReminderLeadMinutes = 24 * 60
	MinReminderLeadMinutes     = 5
	MaxReminderLeadMinutes     = 7 * 24 * 60
	MessageValidity            = time.Hour
	ReminderSafetyMargin       = time.Minute
)

var (
	ErrInvalidPolicy   = errors.New("invalid notification policy")
	ErrInvalidSchedule = errors.New("invalid notification schedule")
)

// Policy is a persisted shop policy snapshot. Updating it affects new plans;
// disabling notifications must also be checked by the future delivery workflow.
type Policy struct {
	Enabled             bool
	ReminderLeadMinutes int
	Version             int64
}

// DefaultPolicy is deliberately disabled until the shop is ready for messaging.
func DefaultPolicy() Policy {
	return Policy{ReminderLeadMinutes: DefaultReminderLeadMinutes, Version: 1}
}

func (p Policy) Valid() bool {
	return p.Version > 0 && p.ReminderLeadMinutes >= MinReminderLeadMinutes &&
		p.ReminderLeadMinutes <= MaxReminderLeadMinutes
}

type SkipReason string

const (
	SkipPolicyDisabled    SkipReason = "policy_disabled"
	SkipPermissionMissing SkipReason = "permission_missing"
	SkipInsufficientLead  SkipReason = "insufficient_lead"
)

// Window is half-open: [DueAt, ExpiresAt). Eligibility is necessary but does not
// replace the delivery-time checks of appointment state, permission and settings.
type Window struct {
	DueAt     time.Time
	ExpiresAt time.Time
}

func (w Window) Valid() bool {
	return validInstant(w.DueAt) && validInstant(w.ExpiresAt) && w.ExpiresAt.After(w.DueAt)
}

func (w Window) EligibleAt(now time.Time) bool {
	return validInstant(now) && w.Valid() && !now.Before(w.DueAt) && now.Before(w.ExpiresAt)
}

type Schedule struct {
	PolicyVersion      int64
	Confirmation       Window
	Reminder           Window
	SkipReason         SkipReason
	ReminderSkipReason SkipReason
}

// RetainUntil is the last possible send deadline, not indefinite receipt storage.
func (s Schedule) RetainUntil() time.Time {
	if s.Reminder.ExpiresAt.After(s.Confirmation.ExpiresAt) {
		return s.Reminder.ExpiresAt
	}
	return s.Confirmation.ExpiresAt
}

// Schedule uses elapsed time, not a civil "previous day" or an in-memory timer.
// All instants are supplied by the caller; this rule never reads the wall clock.
func (p Policy) Schedule(createdAt, startsAt time.Time) (Schedule, error) {
	if !p.Valid() {
		return Schedule{}, ErrInvalidPolicy
	}
	if !p.Enabled {
		return Schedule{PolicyVersion: p.Version, SkipReason: SkipPolicyDisabled}, nil
	}
	// PostgreSQL timestamptz preserves microseconds. Normalize before calculating
	// authenticated retention metadata so a database round trip cannot change it.
	createdAt = createdAt.UTC().Truncate(time.Microsecond)
	startsAt = startsAt.UTC().Truncate(time.Microsecond)
	if !validInstant(createdAt) || !validInstant(startsAt) || !startsAt.After(createdAt) {
		return Schedule{}, ErrInvalidSchedule
	}
	confirmationExpiry := earlier(createdAt.Add(MessageValidity), startsAt)
	schedule := Schedule{
		PolicyVersion: p.Version,
		Confirmation:  Window{DueAt: createdAt, ExpiresAt: confirmationExpiry},
	}
	reminderDue := startsAt.Add(-time.Duration(p.ReminderLeadMinutes) * time.Minute)
	// Equality would send confirmation and reminder together; confirmation wins.
	if !reminderDue.After(createdAt) {
		schedule.ReminderSkipReason = SkipInsufficientLead
		return schedule, nil
	}
	schedule.Reminder = Window{
		DueAt:     reminderDue,
		ExpiresAt: earlier(reminderDue.Add(MessageValidity), startsAt.Add(-ReminderSafetyMargin)),
	}
	return schedule, nil
}

func validInstant(t time.Time) bool {
	return !t.IsZero() && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
