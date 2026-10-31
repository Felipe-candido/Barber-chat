package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LifecycleStore struct{ q *db.Queries }

func NewLifecycleStore(q *db.Queries) *LifecycleStore { return &LifecycleStore{q} }

var _ application.AppointmentLifecycleStore = (*LifecycleStore)(nil)
var _ application.DueAppointmentCompleter = (*LifecycleStore)(nil)

func (s *LifecycleStore) ReadReference(ctx context.Context, shopID, id uuid.UUID) (uuid.UUID, bool, error) {
	r, err := s.q.ReadAppointmentReference(ctx, db.ReadAppointmentReferenceParams{ShopID: shopID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return r.ProfessionalID, true, nil
}
func (s *LifecycleStore) LockProfessional(ctx context.Context, shopID, id uuid.UUID) error {
	_, err := s.q.LockProfessionalCalendar(ctx, db.LockProfessionalCalendarParams{ShopID: shopID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrAppointmentNotFound
	}
	return err
}
func (s *LifecycleStore) LockAppointment(ctx context.Context, shopID, id uuid.UUID) (domain.Lifecycle, bool, error) {
	r, err := s.q.LockLifecycleAppointment(ctx, db.LockLifecycleAppointmentParams{ShopID: shopID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Lifecycle{}, false, nil
	}
	if err != nil {
		return domain.Lifecycle{}, false, err
	}
	a := domain.Lifecycle{ID: r.ID, ShopID: r.ShopID, ProfessionalID: r.ProfessionalID, StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(), Status: domain.Status(r.Status), Version: r.Version, Source: domain.StatusSource(r.StatusSource), Reason: r.StatusReason}
	if r.StatusActorID.Valid {
		a.ActorID = r.StatusActorID.Bytes
	}
	return a, true, nil
}
func (s *LifecycleStore) Now(ctx context.Context) (time.Time, error) {
	r, err := s.q.ReadLifecycleClock(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if !r.Valid || r.InfinityModifier != pgtype.Finite {
		return time.Time{}, application.ErrInconsistentBookingData
	}
	return r.Time.UTC(), nil
}
func (s *LifecycleStore) SaveStaffStatus(ctx context.Context, a domain.Lifecycle, expected int64) (bool, error) {
	count, err := s.q.SetStaffAppointmentStatus(ctx, db.SetStaffAppointmentStatusParams{ShopID: a.ShopID, AppointmentID: a.ID, Status: string(a.Status), Reason: a.Reason, ActorID: pgtype.UUID{Bytes: a.ActorID, Valid: a.ActorID != uuid.Nil}, ChangedAt: timestamp(a.UpdatedAt), ExpectedVersion: expected})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return false, application.ErrSlotUnavailable
	}
	return count == 1, err
}
func (s *LifecycleStore) ReadDetail(ctx context.Context, shopID, id uuid.UUID) (application.AppointmentLifecycleOutput, error) {
	r, err := s.q.GetAdminAppointmentDetail(ctx, db.GetAdminAppointmentDetailParams{ShopID: shopID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.AppointmentLifecycleOutput{}, application.ErrAppointmentNotFound
	}
	if err != nil {
		return application.AppointmentLifecycleOutput{}, err
	}
	a := application.AdminAppointment{Appointment: application.AppointmentReceipt{ShopName: r.ShopName, AppointmentID: r.ID, Status: domain.Status(r.Status), Service: domain.ServiceSnapshot{Name: r.ServiceName, DurationMinutes: int(r.DurationMinutes), PriceCents: r.PriceCents, Currency: r.Currency}, ProfessionalID: r.ProfessionalID, ProfessionalName: r.ProfessionalName, StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), ShopTimezone: r.ShopTimezone}, CustomerName: r.CustomerName, CustomerPhone: r.PhoneE164, Version: r.Version, StatusSource: domain.StatusSource(r.StatusSource), StatusReason: r.StatusReason, UpdatedAt: r.UpdatedAt.Time.UTC()}
	if r.StatusActorID.Valid {
		a.StatusActorID = r.StatusActorID.Bytes
	}
	rows, err := s.q.ListAppointmentStatusEvents(ctx, db.ListAppointmentStatusEventsParams{ShopID: shopID, AppointmentID: id})
	if err != nil {
		return application.AppointmentLifecycleOutput{}, err
	}
	out := application.AppointmentLifecycleOutput{Appointment: a, History: make([]application.StatusEvent, 0, len(rows)), ResultVersion: r.Version}
	if len(rows) > 100 {
		out.HistoryTruncated = true
		rows = rows[:100]
	}
	for _, e := range rows {
		event := application.StatusEvent{Version: e.Version, PreviousStatus: domain.Status(e.PreviousStatus), Status: domain.Status(e.Status), Source: domain.StatusSource(e.Source), Reason: e.Reason, OccurredAt: e.OccurredAt.Time.UTC()}
		if e.ActorID.Valid {
			event.ActorID = e.ActorID.Bytes
		}
		out.History = append(out.History, event)
	}
	return out, nil
}
func (s *LifecycleStore) ClaimStatusCommand(ctx context.Context, scope application.LifecycleScope, key application.StatusCommandKey) (uuid.UUID, int64, bool, error) {
	count, err := s.q.ClaimAppointmentStatusCommand(ctx, db.ClaimAppointmentStatusCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, KeyDigest: key.Digest[:], Fingerprint: key.Fingerprint[:]})
	if err != nil {
		return uuid.Nil, 0, false, err
	}
	if count == 1 {
		return uuid.Nil, 0, false, nil
	}
	r, err := s.q.LockAppointmentStatusCommand(ctx, db.LockAppointmentStatusCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, KeyDigest: key.Digest[:]})
	if err != nil {
		return uuid.Nil, 0, false, err
	}
	if subtle.ConstantTimeCompare(r.Fingerprint, key.Fingerprint[:]) != 1 {
		return uuid.Nil, 0, false, application.ErrStatusCommandConflict
	}
	if !r.Completed {
		return uuid.Nil, 0, false, application.ErrStatusCommandPending
	}
	if !r.AppointmentID.Valid || !r.ResultVersion.Valid {
		return uuid.Nil, 0, false, application.ErrInconsistentBookingData
	}
	return r.AppointmentID.Bytes, r.ResultVersion.Int64, true, nil
}
func (s *LifecycleStore) CompleteStatusCommand(ctx context.Context, scope application.LifecycleScope, key application.StatusCommandKey, id uuid.UUID, version int64) error {
	count, err := s.q.CompleteAppointmentStatusCommand(ctx, db.CompleteAppointmentStatusCommandParams{ShopID: scope.ShopID, ActorID: scope.ActorID, KeyDigest: key.Digest[:], Fingerprint: key.Fingerprint[:], AppointmentID: pgtype.UUID{Bytes: id, Valid: true}, ResultVersion: pgtype.Int8{Int64: version, Valid: true}})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrInconsistentBookingData
	}
	return nil
}
func (s *LifecycleStore) CompleteDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, domain.ErrInvalidStatusChange
	}
	rows, err := s.q.CompleteDueAppointments(ctx, int32(limit))
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

type LifecycleTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.AppointmentLifecycleTx
}

func NewLifecycleTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.AppointmentLifecycleTx) *LifecycleTransactor {
	return &LifecycleTransactor{pool, bind}
}
func (t *LifecycleTransactor) WithinAppointmentLifecycleTx(ctx context.Context, fn func(context.Context, application.AppointmentLifecycleTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
