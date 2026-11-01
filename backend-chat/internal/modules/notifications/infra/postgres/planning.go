package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type PlanningStore struct{ queries *db.Queries }

func NewPlanningStore(q *db.Queries) *PlanningStore { return &PlanningStore{q} }

func (s *PlanningStore) Policy(ctx context.Context, shopID uuid.UUID) (domain.Policy, error) {
	row, err := s.queries.GetBookingNotificationPolicy(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultPolicy(), nil
	}
	if err != nil {
		return domain.Policy{}, err
	}
	return domain.Policy{Enabled: row.Enabled, ReminderLeadMinutes: int(row.ReminderLeadMinutes), Version: row.Version}, nil
}

func (s *PlanningStore) Save(ctx context.Context, input app.PrepareBookingNotificationsInput, prepared app.PreparedBookingNotifications) error {
	params := db.InsertBookingNotificationPlanParams{
		ShopID: input.ShopID, AppointmentID: input.AppointmentID, ScheduleGeneration: input.ScheduleGeneration,
		PolicyVersion: prepared.Schedule.PolicyVersion, CreatedAt: stamp(input.CreatedAt),
		SkipReason: text(string(prepared.Schedule.SkipReason)), ReminderSkipReason: text(string(prepared.Schedule.ReminderSkipReason)),
	}
	if prepared.Schedule.SkipReason == "" {
		if !prepared.Scope.Valid() || prepared.Scope.ShopID != input.ShopID || prepared.Scope.AppointmentID != input.AppointmentID ||
			prepared.Scope.ScheduleGeneration != input.ScheduleGeneration || !prepared.Scope.ExpiresAt.Equal(prepared.Schedule.RetainUntil()) ||
			!prepared.Schedule.Confirmation.Valid() || prepared.Payload.Version != 1 || prepared.Payload.KeyID == "" || len(prepared.Payload.Ciphertext) == 0 {
			return app.ErrInvalidPreparation
		}
		params.PayloadVersion = pgtype.Int4{Int32: 1, Valid: true}
		params.PayloadKeyID = text(prepared.Payload.KeyID)
		params.EncryptedPayload = prepared.Payload.Ciphertext
		params.PayloadExpiresAt = stamp(prepared.Scope.ExpiresAt)
	}
	if err := s.queries.InsertBookingNotificationPlan(ctx, params); err != nil {
		return err
	}
	for _, job := range []struct {
		kind   string
		window domain.Window
	}{
		{"booking_confirmation", prepared.Schedule.Confirmation}, {"booking_reminder", prepared.Schedule.Reminder},
	} {
		if !job.window.Valid() {
			continue
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		if err := s.queries.InsertBookingNotificationJob(ctx, db.InsertBookingNotificationJobParams{
			ID: id, ShopID: input.ShopID, AppointmentID: input.AppointmentID, ScheduleGeneration: input.ScheduleGeneration,
			Kind: job.kind, DueAt: stamp(job.window.DueAt), ExpiresAt: stamp(job.window.ExpiresAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func stamp(at time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: at.UTC().Truncate(time.Microsecond), Valid: true}
}
func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
