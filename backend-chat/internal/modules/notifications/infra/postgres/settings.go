package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/jackc/pgx/v5"
)

// Reuse the connection repository's short owner-authorized transactions.
type Settings struct{ owner *Connections }

func NewSettings(owner *Connections) *Settings { return &Settings{owner} }

func (s *Settings) WorkerAvailable(ctx context.Context) (bool, error) {
	return db.New(s.owner.pool).NotificationWorkerAvailable(ctx)
}
func readSettings(ctx context.Context, q *db.Queries, s app.ConnectionScope) (app.NotificationSettings, error) {
	p, err := q.GetBookingNotificationPolicy(ctx, s.ShopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.NotificationSettings{DeliveryMode: "request", ReminderLeadMinutes: 1440}, nil
	}
	return app.NotificationSettings{Enabled: p.Enabled, DeliveryMode: p.DeliveryMode, ReminderLeadMinutes: int(p.ReminderLeadMinutes), Version: p.Version}, err
}
func (s *Settings) Read(ctx context.Context, scope app.ConnectionScope) (app.NotificationSettings, error) {
	var result app.NotificationSettings
	err := s.owner.withOwner(ctx, scope, func(q *db.Queries) error { var err error; result, err = readSettings(ctx, q, scope); return err })
	return result, err
}
func (s *Settings) Save(ctx context.Context, scope app.ConnectionScope, input app.NotificationSettings) (app.NotificationSettings, error) {
	var result app.NotificationSettings
	err := s.owner.withOwner(ctx, scope, func(q *db.Queries) error {
		var n int64
		var err error
		if input.Version == 0 {
			n, err = q.CreateNotificationPolicy(ctx, db.CreateNotificationPolicyParams{ShopID: scope.ShopID, Enabled: input.Enabled, ReminderLeadMinutes: int32(input.ReminderLeadMinutes), DeliveryMode: input.DeliveryMode})
		} else {
			n, err = q.UpdateNotificationPolicy(ctx, db.UpdateNotificationPolicyParams{ShopID: scope.ShopID, Enabled: input.Enabled, ReminderLeadMinutes: int32(input.ReminderLeadMinutes), DeliveryMode: input.DeliveryMode, Version: input.Version})
		}
		if err != nil {
			return err
		}
		if n != 1 {
			return app.ErrNotificationSettingsConflict
		}
		result, err = readSettings(ctx, q, scope)
		return err
	})
	return result, err
}
