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
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeliveryTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) app.DeliveryTx
}

func NewDeliveryTransactor(pool *pgxpool.Pool, bind func(*db.Queries) app.DeliveryTx) *DeliveryTransactor {
	return &DeliveryTransactor{pool, bind}
}
func (t *DeliveryTransactor) WithinDeliveryTx(ctx context.Context, fn func(context.Context, app.DeliveryTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	bound := t.bind(db.New(tx))
	if bound.Store == nil || bound.Shops == nil || bound.Permissions == nil || bound.Bookings == nil {
		return app.ErrDeliveryUnavailable
	}
	if err := fn(ctx, bound); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type DeliveryStore struct{ q *db.Queries }

func NewDeliveryStore(q *db.Queries) *DeliveryStore { return &DeliveryStore{q} }
func (s *DeliveryStore) Reference(ctx context.Context, k app.DeliveryKey) (uuid.UUID, bool, error) {
	id, err := s.q.GetDeliveryReference(ctx, db.GetDeliveryReferenceParams{ShopID: k.ShopID, ID: k.JobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}
func (s *DeliveryStore) PolicyEnabled(ctx context.Context, shop uuid.UUID) (bool, error) {
	p, err := s.q.GetBookingNotificationPolicy(ctx, shop)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return p.Enabled, err
}
func (s *DeliveryStore) Connection(ctx context.Context, shop uuid.UUID) (domain.Connection, error) {
	c, err := s.q.LockDeliveryConnection(ctx, shop)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Connection{}, nil
	}
	return domain.Connection{ShopID: shop, InstanceName: c.InstanceName, State: domain.ConnectionState(c.State), DesiredConnected: c.DesiredConnected, VerifiedAt: c.VerifiedAt.Time.UTC(), ErrorCode: c.ErrorCode}, err
}
func (s *DeliveryStore) Lock(ctx context.Context, k app.DeliveryKey) (app.DeliveryRecord, bool, error) {
	r, err := s.q.LockNotificationDelivery(ctx, db.LockNotificationDeliveryParams{ShopID: k.ShopID, ID: k.JobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DeliveryRecord{}, false, nil
	}
	if err != nil {
		return app.DeliveryRecord{}, false, err
	}
	return app.DeliveryRecord{Key: k, AppointmentID: r.AppointmentID, Kind: r.Kind,
		Scope:   app.PayloadScope{ShopID: r.ShopID, AppointmentID: r.AppointmentID, ScheduleGeneration: r.ScheduleGeneration, ExpiresAt: r.PayloadExpiresAt.Time.UTC()},
		Payload: app.ProtectedPayload{Version: int(r.PayloadVersion.Int32), KeyID: r.PayloadKeyID.String, Ciphertext: r.EncryptedPayload},
		Window:  domain.Window{DueAt: r.DueAt.Time.UTC(), ExpiresAt: r.ExpiresAt.Time.UTC()}, IntentInvalidated: r.State != "pending",
		State: domain.DeliveryState(r.DeliveryState), Token: uuid.UUID(r.DeliveryToken.Bytes), LeaseUntil: r.DeliveryLeaseUntil.Time.UTC(), Reason: domain.DeliveryReason(r.DeliveryReason)}, true, nil
}
func (s *DeliveryStore) Now(ctx context.Context) (time.Time, error) {
	r, err := s.q.DeliveryDatabaseTime(ctx)
	return r.Time.UTC(), err
}
func (s *DeliveryStore) Claim(ctx context.Context, c app.DeliveryClaim) error {
	n, err := s.q.ClaimNotificationDelivery(ctx, db.ClaimNotificationDeliveryParams{ShopID: c.Key.ShopID, ID: c.Key.JobID, DeliveryToken: pgtype.UUID{Bytes: c.Token, Valid: true}})
	return changed(n, err)
}
func (s *DeliveryStore) Stop(ctx context.Context, k app.DeliveryKey, state domain.DeliveryState, reason domain.DeliveryReason) error {
	n, err := s.q.StopNotificationBeforeSending(ctx, db.StopNotificationBeforeSendingParams{ShopID: k.ShopID, ID: k.JobID, DeliveryState: string(state), DeliveryReason: string(reason)})
	return changed(n, err)
}
func (s *DeliveryStore) Start(ctx context.Context, c app.DeliveryClaim, instance string) error {
	n, err := s.q.StartNotificationDelivery(ctx, db.StartNotificationDeliveryParams{ShopID: c.Key.ShopID, ID: c.Key.JobID, DeliveryToken: pgtype.UUID{Bytes: c.Token, Valid: true}})
	if err = changed(n, err); err != nil {
		return err
	}
	return s.q.InsertNotificationAttempt(ctx, db.InsertNotificationAttemptParams{ShopID: c.Key.ShopID, ID: c.Key.JobID, DeliveryToken: pgtype.UUID{Bytes: c.Token, Valid: true}, InstanceName: instance})
}
func (s *DeliveryStore) Complete(ctx context.Context, c app.DeliveryClaim, outcome domain.SendOutcome) error {
	o := outcome.Normalize()
	// Lock the job before the attempt, matching Start and cancellation triggers.
	r, found, err := s.Lock(ctx, c.Key)
	if err != nil {
		return err
	}
	if !found || r.Token != c.Token {
		return app.ErrDeliveryLeaseLost
	}
	// Idempotent completion cannot regress accepted/failed, or unknown to failure.
	if r.State != domain.DeliverySending && !(r.State == domain.DeliveryUnknown && o.Disposition == domain.SendAccepted) {
		return nil
	}
	n, err := s.q.CompleteNotificationAttempt(ctx, db.CompleteNotificationAttemptParams{ShopID: c.Key.ShopID, JobID: c.Key.JobID, AttemptID: c.Token, Outcome: string(o.Disposition), ProviderMessageID: text(o.ProviderMessageID), Reason: string(o.Reason)})
	if err = changed(n, err); err != nil {
		return err
	}
	n, err = s.q.CompleteNotificationDelivery(ctx, db.CompleteNotificationDeliveryParams{ShopID: c.Key.ShopID, ID: c.Key.JobID, DeliveryToken: pgtype.UUID{Bytes: c.Token, Valid: true}, DeliveryState: string(o.State()), DeliveryReason: string(o.Reason)})
	return changed(n, err)
}
func changed(n int64, err error) error {
	if err != nil {
		return err
	}
	if n != 1 {
		return app.ErrDeliveryLeaseLost
	}
	return nil
}
