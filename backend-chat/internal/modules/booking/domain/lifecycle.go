package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type StatusSource string

const (
	SourceLegacy    StatusSource = "legacy"
	SourceBooking   StatusSource = "booking"
	SourcePublic    StatusSource = "public"
	SourceStaff     StatusSource = "staff"
	SourceAutomatic StatusSource = "automatic"
)

var (
	ErrInvalidStatusChange           = errors.New("invalid appointment status change")
	ErrAppointmentVersionConflict    = errors.New("appointment version changed")
	ErrAppointmentAlreadyInStatus    = errors.New("appointment already in requested status")
	ErrAppointmentTransitionTooEarly = errors.New("appointment has not reached the required time")
)

// Lifecycle excludes contact data and immutable booking terms.
type Lifecycle struct {
	ID, ShopID, ProfessionalID  uuid.UUID
	StartsAt, EndsAt, UpdatedAt time.Time
	Status                      Status
	Version                     int64
	Source                      StatusSource
	Reason                      string
	ActorID                     uuid.UUID
}

func ValidStatusReason(raw string) bool {
	return utf8.ValidString(raw) && utf8.RuneCountInString(raw) >= 1 && utf8.RuneCountInString(raw) <= 500 && !strings.ContainsFunc(raw, unicode.IsControl)
}

func (a Lifecycle) ChangeByStaff(actorID uuid.UUID, status Status, reason string, expectedVersion int64, now time.Time) (Lifecycle, error) {
	reason = strings.TrimSpace(reason)
	if a.ID == uuid.Nil || a.ShopID == uuid.Nil || a.ProfessionalID == uuid.Nil || actorID == uuid.Nil || a.StartsAt.IsZero() || !a.EndsAt.After(a.StartsAt) || now.IsZero() || now.Before(a.UpdatedAt) || !ValidStatusReason(reason) ||
		(status != StatusCompleted && status != StatusNoShow && status != StatusCancelled) {
		return Lifecycle{}, ErrInvalidStatusChange
	}
	if a.Status != StatusScheduled && a.Status != StatusConfirmed && a.Status != StatusCompleted && a.Status != StatusNoShow && a.Status != StatusCancelled {
		return Lifecycle{}, ErrInvalidStatusChange
	}
	if expectedVersion < 1 || a.Version != expectedVersion || a.Version == math.MaxInt64 {
		return Lifecycle{}, ErrAppointmentVersionConflict
	}
	if a.Status == status {
		return Lifecycle{}, ErrAppointmentAlreadyInStatus
	}
	if (status == StatusCompleted && now.Before(a.EndsAt)) || (status == StatusNoShow && now.Before(a.StartsAt)) {
		return Lifecycle{}, ErrAppointmentTransitionTooEarly
	}
	a.Status = status
	a.Version++
	a.Source = SourceStaff
	a.Reason = reason
	a.ActorID = actorID
	a.UpdatedAt = now.UTC()
	return a, nil
}
