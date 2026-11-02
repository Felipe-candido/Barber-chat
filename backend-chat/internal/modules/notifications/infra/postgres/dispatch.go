package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DispatchStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewDispatchStore(pool *pgxpool.Pool) *DispatchStore { return &DispatchStore{pool, db.New(pool)} }

func (s *DispatchStore) Maintenance(ctx context.Context) error {
	if _, err := s.q.PurgeExpiredNotificationPayloads(ctx); err != nil {
		return err
	}
	if _, err := s.q.RetireIneligibleNotificationOutbox(ctx); err != nil {
		return err
	}
	if _, err := s.q.PruneConsumedNotificationOutbox(ctx); err != nil {
		return err
	}
	return s.q.PruneNotificationHeartbeats(ctx)
}

func (s *DispatchStore) Metrics(ctx context.Context) (app.DispatchMetrics, error) {
	m, err := s.q.NotificationOperationalMetrics(ctx)
	return app.DispatchMetrics{OutboxPending: m.OutboxPending, Uncertain: m.Uncertain, Sending: m.Sending, OldestDueSeconds: m.OldestDueSeconds}, err
}
func (s *DispatchStore) Materialize(ctx context.Context) (int, error) {
	rows, err := s.q.MaterializeNotificationDispatches(ctx)
	return len(rows), err
}
func (s *DispatchStore) Claim(ctx context.Context) (app.DispatchLease, bool, error) {
	token, err := uuid.NewRandom()
	if err != nil {
		return app.DispatchLease{}, false, err
	}
	r, err := s.q.ClaimNotificationOutbox(ctx, pgtype.UUID{Bytes: token, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DispatchLease{}, false, nil
	}
	return app.DispatchLease{Envelope: app.DispatchEnvelope{Version: 1, OutboxID: r.ID, ShopID: r.ShopID, JobID: r.JobID, Sequence: r.DispatchSeq}, Token: token}, err == nil, err
}
func (s *DispatchStore) Published(ctx context.Context, l app.DispatchLease) error {
	n, err := s.q.CompleteNotificationPublish(ctx, db.CompleteNotificationPublishParams{ID: l.Envelope.OutboxID, OperationToken: pgtype.UUID{Bytes: l.Token, Valid: true}})
	return changed(n, err)
}
func (s *DispatchStore) PublishDeferred(ctx context.Context, l app.DispatchLease) error {
	n, err := s.q.DeferNotificationPublish(ctx, db.DeferNotificationPublishParams{ID: l.Envelope.OutboxID, OperationToken: pgtype.UUID{Bytes: l.Token, Valid: true}})
	return changed(n, err)
}
func (s *DispatchStore) Validate(ctx context.Context, e app.DispatchEnvelope) (bool, error) {
	return s.q.ValidateNotificationEnvelope(ctx, db.ValidateNotificationEnvelopeParams{ID: e.OutboxID, ShopID: e.ShopID, JobID: e.JobID, DispatchSeq: e.Sequence})
}
func (s *DispatchStore) Finish(ctx context.Context, e app.DispatchEnvelope) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	q := db.New(tx)
	if err = q.FinishNotificationDispatch(ctx, db.FinishNotificationDispatchParams{ShopID: e.ShopID, ID: e.JobID, DispatchSeq: e.Sequence}); err != nil {
		return err
	}
	if err = q.RetireNotificationDispatch(ctx, db.RetireNotificationDispatchParams{ID: e.OutboxID, ShopID: e.ShopID, JobID: e.JobID, DispatchSeq: e.Sequence}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *DispatchStore) RecoveryKeys(ctx context.Context) ([]app.DeliveryKey, error) {
	rows, err := s.q.ListNotificationRecoveryKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]app.DeliveryKey, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, app.DeliveryKey{ShopID: r.ShopID, JobID: r.ID})
	}
	return keys, nil
}
func (s *DispatchStore) Heartbeat(ctx context.Context, id uuid.UUID, healthy bool) error {
	return s.q.TouchNotificationWorker(ctx, db.TouchNotificationWorkerParams{WorkerID: id, Healthy: healthy})
}
