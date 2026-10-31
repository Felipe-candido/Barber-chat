package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

var (
	ErrAppointmentAccessDenied = errors.New("appointment staff access denied")
	ErrInvalidStatusCommandKey = errors.New("invalid status command key")
	ErrStatusCommandConflict   = errors.New("status command key conflict")
	ErrStatusCommandPending    = errors.New("status command incomplete")
)

type LifecycleScope struct{ ActorID, ShopID uuid.UUID }
type ChangeAppointmentStatusInput struct {
	Scope                  LifecycleScope
	AppointmentID          uuid.UUID
	Status                 domain.Status
	ExpectedVersion        int64
	Reason, IdempotencyKey string
}
type StatusCommandKey struct{ Digest, Fingerprint [32]byte }
type StatusEvent struct {
	Version                int64
	PreviousStatus, Status domain.Status
	Source                 domain.StatusSource
	ActorID                uuid.UUID
	Reason                 string
	OccurredAt             time.Time
}
type AppointmentLifecycleOutput struct {
	Appointment      AdminAppointment
	History          []StatusEvent
	HistoryTruncated bool
	Replayed         bool
	ResultVersion    int64
}
type AppointmentLifecycleAccess interface {
	// Called after locking the appointment's professional; owner covers the
	// whole active shop, barber only its own active professional.
	RequireAppointmentAccess(context.Context, LifecycleScope, uuid.UUID) error
}
type AppointmentLifecycleStore interface {
	ReadReference(ctx context.Context, shopID, appointmentID uuid.UUID) (professionalID uuid.UUID, found bool, err error)
	LockProfessional(ctx context.Context, shopID, professionalID uuid.UUID) error
	LockAppointment(ctx context.Context, shopID, appointmentID uuid.UUID) (domain.Lifecycle, bool, error)
	Now(context.Context) (time.Time, error)
	SaveStaffStatus(context.Context, domain.Lifecycle, int64) (bool, error)
	ReadDetail(ctx context.Context, shopID, appointmentID uuid.UUID) (AppointmentLifecycleOutput, error)
	ClaimStatusCommand(context.Context, LifecycleScope, StatusCommandKey) (appointmentID uuid.UUID, resultVersion int64, replayed bool, err error)
	CompleteStatusCommand(context.Context, LifecycleScope, StatusCommandKey, uuid.UUID, int64) error
}
type AppointmentLifecycleTx struct {
	Access AppointmentLifecycleAccess
	Store  AppointmentLifecycleStore
}
type AppointmentLifecycleTransactor interface {
	WithinAppointmentLifecycleTx(context.Context, func(context.Context, AppointmentLifecycleTx) error) error
}
type DueAppointmentCompleter interface {
	CompleteDue(context.Context, int) (int, error)
}

type ManageAppointmentStatus struct {
	tx AppointmentLifecycleTransactor
}

func NewManageAppointmentStatus(tx AppointmentLifecycleTransactor) *ManageAppointmentStatus {
	return &ManageAppointmentStatus{tx}
}

func statusKey(input ChangeAppointmentStatusInput) (StatusCommandKey, error) {
	if input.Scope.ActorID == uuid.Nil || input.Scope.ShopID == uuid.Nil || input.AppointmentID == uuid.Nil || input.ExpectedVersion < 1 || !domain.ValidStatusReason(input.Reason) ||
		(input.Status != domain.StatusCompleted && input.Status != domain.StatusNoShow && input.Status != domain.StatusCancelled) {
		return StatusCommandKey{}, domain.ErrInvalidStatusChange
	}
	raw, err := hex.DecodeString(input.IdempotencyKey)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != input.IdempotencyKey {
		return StatusCommandKey{}, ErrInvalidStatusCommandKey
	}
	canonical := struct {
		Protocol                       int
		ActorID, ShopID, AppointmentID uuid.UUID
		Status                         domain.Status
		ExpectedVersion                int64
		Reason                         string
	}{1, input.Scope.ActorID, input.Scope.ShopID, input.AppointmentID, input.Status, input.ExpectedVersion, input.Reason}
	data, err := json.Marshal(canonical)
	if err != nil {
		return StatusCommandKey{}, err
	}
	mac := hmac.New(sha256.New, raw)
	_, _ = mac.Write(data)
	key := StatusCommandKey{Digest: sha256.Sum256(raw)}
	copy(key.Fingerprint[:], mac.Sum(nil))
	return key, nil
}
func authorizeAppointment(ctx context.Context, tx AppointmentLifecycleTx, scope LifecycleScope, id uuid.UUID) error {
	professional, found, err := tx.Store.ReadReference(ctx, scope.ShopID, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrAppointmentNotFound
	}
	if professional == uuid.Nil {
		return ErrInconsistentBookingData
	}
	if err := tx.Store.LockProfessional(ctx, scope.ShopID, professional); err != nil {
		return err
	}
	return tx.Access.RequireAppointmentAccess(ctx, scope, professional)
}
func (uc *ManageAppointmentStatus) Detail(ctx context.Context, scope LifecycleScope, id uuid.UUID) (AppointmentLifecycleOutput, error) {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil {
		return AppointmentLifecycleOutput{}, ErrAppointmentAccessDenied
	}
	if id == uuid.Nil {
		return AppointmentLifecycleOutput{}, ErrAppointmentNotFound
	}
	var out AppointmentLifecycleOutput
	err := uc.tx.WithinAppointmentLifecycleTx(ctx, func(ctx context.Context, tx AppointmentLifecycleTx) error {
		if err := authorizeAppointment(ctx, tx, scope, id); err != nil {
			return err
		}
		var err error
		out, err = tx.Store.ReadDetail(ctx, scope.ShopID, id)
		return err
	})
	if err != nil {
		return AppointmentLifecycleOutput{}, err
	}
	return out, nil
}
func (uc *ManageAppointmentStatus) Change(ctx context.Context, input ChangeAppointmentStatusInput) (AppointmentLifecycleOutput, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	key, err := statusKey(input)
	if err != nil {
		return AppointmentLifecycleOutput{}, err
	}
	var out AppointmentLifecycleOutput
	err = uc.tx.WithinAppointmentLifecycleTx(ctx, func(ctx context.Context, tx AppointmentLifecycleTx) error {
		// Authorize before a new claim can be written. The professional lock also
		// serializes public cancellations and automatic completion for this row.
		if err := authorizeAppointment(ctx, tx, input.Scope, input.AppointmentID); err != nil {
			return err
		}
		id, resultVersion, replayed, err := tx.Store.ClaimStatusCommand(ctx, input.Scope, key)
		if err != nil {
			return err
		}
		if replayed {
			if id != input.AppointmentID {
				return ErrInconsistentBookingData
			}
			out, err = tx.Store.ReadDetail(ctx, input.Scope.ShopID, id)
			if err != nil {
				return err
			}
			out.Replayed = true
			out.ResultVersion = resultVersion
			return nil
		}
		current, found, err := tx.Store.LockAppointment(ctx, input.Scope.ShopID, input.AppointmentID)
		if err != nil {
			return err
		}
		if !found {
			return ErrAppointmentNotFound
		}
		if current.ID != input.AppointmentID || current.ShopID != input.Scope.ShopID {
			return ErrInconsistentBookingData
		}
		now, err := tx.Store.Now(ctx)
		if err != nil {
			return err
		}
		changed, err := current.ChangeByStaff(input.Scope.ActorID, input.Status, input.Reason, input.ExpectedVersion, now)
		if err != nil {
			return err
		}
		saved, err := tx.Store.SaveStaffStatus(ctx, changed, current.Version)
		if err != nil {
			return err
		}
		if !saved {
			return domain.ErrAppointmentVersionConflict
		}
		if err := tx.Store.CompleteStatusCommand(ctx, input.Scope, key, current.ID, changed.Version); err != nil {
			return err
		}
		out, err = tx.Store.ReadDetail(ctx, input.Scope.ShopID, current.ID)
		out.ResultVersion = changed.Version
		return err
	})
	if err != nil {
		return AppointmentLifecycleOutput{}, err
	}
	return out, nil
}

// A timer merely wakes this use case; PostgreSQL holds the durable deadline and
// cutover. A restart or another API replica can recover the remaining backlog.
type AutoCompleteAppointments struct{ due DueAppointmentCompleter }

func NewAutoCompleteAppointments(due DueAppointmentCompleter) *AutoCompleteAppointments {
	return &AutoCompleteAppointments{due}
}
func (uc *AutoCompleteAppointments) Execute(ctx context.Context) (int, error) {
	return uc.due.CompleteDue(ctx, 100)
}
