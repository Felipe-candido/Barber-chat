package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository struct{ queries *db.Queries }

func NewRepository(queries *db.Queries) *Repository { return &Repository{queries} }
func timestamp(t time.Time) pgtype.Timestamptz      { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func (r *Repository) LockProfessionalSchedule(ctx context.Context, shopID, professionalID uuid.UUID) error {
	_, err := r.queries.LockProfessionalCalendar(ctx, db.LockProfessionalCalendarParams{ShopID: shopID, ID: professionalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrBookingOptionNotFound
	}
	return err
}

func (r *Repository) LoadCalendar(ctx context.Context, q application.AvailabilityQuery) (domain.Calendar, bool, error) {
	settings, err := r.queries.GetSlotPolicy(ctx, db.GetSlotPolicyParams{ShopID: q.ShopID, ProfessionalID: q.ProfessionalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Calendar{}, false, nil
	}
	if err != nil {
		return domain.Calendar{}, false, err
	}
	policy, err := domain.NewSlotPolicy(int(settings.IntervalMinutes), int(settings.BufferMinutes))
	if err != nil {
		return domain.Calendar{}, false, err
	}
	// Use a generous UTC envelope around the civil date, not a fixed 24-hour
	// shop day. The domain performs exact local-minute and DST checks afterward.
	day, err := time.Parse("2006-01-02", q.LocalDate)
	if err != nil {
		return domain.Calendar{}, false, err
	}
	hours, err := r.queries.ListWorkingHours(ctx, db.ListWorkingHoursParams{ShopID: q.ShopID, ProfessionalID: q.ProfessionalID, Weekday: int32(day.Weekday())})
	if err != nil {
		return domain.Calendar{}, false, err
	}
	busy, err := r.queries.ListBusyIntervals(ctx, db.ListBusyIntervalsParams{ShopID: q.ShopID, ProfessionalID: q.ProfessionalID,
		EndsAt: timestamp(day.Add(-24 * time.Hour)), StartsAt: timestamp(day.Add(48 * time.Hour))})
	if err != nil {
		return domain.Calendar{}, false, err
	}
	calendar := domain.Calendar{Policy: policy, Windows: make([]domain.WorkingWindow, 0, len(hours)), Busy: make([]domain.Interval, 0, len(busy))}
	for _, row := range hours {
		calendar.Windows = append(calendar.Windows, domain.WorkingWindow{OpensAtMinute: int(row.OpensAtMinute), ClosesAtMinute: int(row.ClosesAtMinute)})
	}
	for _, row := range busy {
		calendar.Busy = append(calendar.Busy, domain.Interval{StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC()})
	}
	return calendar, true, nil
}

func (r *Repository) Insert(ctx context.Context, a domain.Appointment, codeHash [32]byte) (domain.Appointment, error) {
	err := r.queries.InsertAppointment(ctx, db.InsertAppointmentParams{ID: a.ID, ShopID: a.ShopID, CustomerID: a.CustomerID, ServiceID: a.ServiceID, ProfessionalID: a.ProfessionalID,
		ServiceName: a.Service.Name, DurationMinutes: int32(a.Service.DurationMinutes), PriceCents: a.Service.PriceCents, Currency: a.Service.Currency,
		ProfessionalName: a.ProfessionalName, ShopTimezone: a.ShopTimezone, StartsAt: timestamp(a.StartsAt), EndsAt: timestamp(a.EndsAt), Status: string(a.Status),
		Version: a.Version, CodeHash: codeHash[:], CreatedAt: timestamp(a.CreatedAt), UpdatedAt: timestamp(a.UpdatedAt)})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return domain.Appointment{}, application.ErrSlotUnavailable
	}
	if err != nil {
		return domain.Appointment{}, err
	}
	return a, nil
}

func (r *Repository) FindByCode(ctx context.Context, shopID uuid.UUID, digest [32]byte) (domain.Appointment, bool, error) {
	row, err := r.queries.FindAppointmentByCode(ctx, db.FindAppointmentByCodeParams{ShopID: shopID, CodeHash: digest[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Appointment{}, false, nil
	}
	if err != nil {
		return domain.Appointment{}, false, err
	}
	return domain.Appointment{ID: row.ID, ShopID: row.ShopID, CustomerID: row.CustomerID, ServiceID: row.ServiceID, ProfessionalID: row.ProfessionalID,
		Service:          domain.ServiceSnapshot{Name: row.ServiceName, DurationMinutes: int(row.DurationMinutes), PriceCents: row.PriceCents, Currency: row.Currency},
		ProfessionalName: row.ProfessionalName, ShopTimezone: row.ShopTimezone, StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(),
		Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, true, nil
}

func (r *Repository) SaveCancellation(ctx context.Context, a domain.Appointment) error {
	count, err := r.queries.CancelAppointment(ctx, db.CancelAppointmentParams{ShopID: a.ShopID, ID: a.ID, Version: a.Version, UpdatedAt: timestamp(a.UpdatedAt)})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrInconsistentBookingData
	}
	return nil
}

func (r *Repository) ClaimCreateAppointment(ctx context.Context, claim application.IdempotencyClaim) (application.ClaimedReceipt, bool, error) {
	keyHash := sha256.Sum256([]byte(claim.Key))
	count, err := r.queries.ClaimBookingRequest(ctx, db.ClaimBookingRequestParams{ShopID: claim.ShopID, KeyHash: keyHash[:], Fingerprint: claim.RequestFingerprint[:], ExpiresAt: timestamp(claim.ExpiresAt)})
	if err != nil {
		return application.ClaimedReceipt{}, false, err
	}
	if count == 1 {
		return application.ClaimedReceipt{}, false, nil
	}
	row, err := r.queries.GetBookingRequest(ctx, db.GetBookingRequestParams{ShopID: claim.ShopID, KeyHash: keyHash[:]})
	if err != nil {
		return application.ClaimedReceipt{}, false, err
	}
	if !row.ExpiresAt.Time.After(claim.RequestedAt) {
		return application.ClaimedReceipt{}, false, application.ErrIdempotencyExpired
	}
	if !hmac.Equal(row.Fingerprint, claim.RequestFingerprint[:]) {
		return application.ClaimedReceipt{}, false, application.ErrIdempotencyConflict
	}
	plaintext, err := security.OpenReceipt(claim.Key, row.EncryptedReceipt, receiptAAD(claim.ShopID, keyHash))
	if err != nil {
		return application.ClaimedReceipt{}, false, err
	}
	var receipt application.ClaimedReceipt
	if err := json.Unmarshal(plaintext, &receipt); err != nil {
		return application.ClaimedReceipt{}, false, err
	}
	if receipt.Appointment.AppointmentID == uuid.Nil || receipt.Code == "" {
		return application.ClaimedReceipt{}, false, application.ErrInconsistentBookingData
	}
	return receipt, true, nil
}

func (r *Repository) CompleteCreateAppointment(ctx context.Context, shopID uuid.UUID, key string, receipt application.ClaimedReceipt) error {
	plaintext, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	keyHash := sha256.Sum256([]byte(key))
	sealed, err := security.SealReceipt(key, plaintext, receiptAAD(shopID, keyHash))
	if err != nil {
		return err
	}
	count, err := r.queries.CompleteBookingRequest(ctx, db.CompleteBookingRequestParams{ShopID: shopID, KeyHash: keyHash[:], EncryptedReceipt: sealed})
	if err != nil {
		return err
	}
	if count != 1 {
		return application.ErrInconsistentBookingData
	}
	return nil
}

func receiptAAD(shopID uuid.UUID, keyHash [32]byte) []byte {
	return append(append([]byte("booking-receipt-v1/"), shopID[:]...), keyHash[:]...)
}
