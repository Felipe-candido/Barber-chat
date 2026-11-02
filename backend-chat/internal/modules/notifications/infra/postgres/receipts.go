package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReceiptStore struct{ pool *pgxpool.Pool }

func NewReceiptStore(pool *pgxpool.Pool) *ReceiptStore { return &ReceiptStore{pool} }
func (s *ReceiptStore) RecordReceipt(ctx context.Context, r app.ProviderReceipt) error {
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
	reference, err := q.FindNotificationReceiptAttempt(ctx, db.FindNotificationReceiptAttemptParams{AttemptID: r.AttemptID, InstanceName: r.InstanceName})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	store := NewDeliveryStore(q)
	connection, err := store.Connection(ctx, reference.ShopID)
	if err != nil {
		return err
	}
	if connection.InstanceName != r.InstanceName {
		return app.ErrInvalidProviderReceipt
	}
	key := app.DeliveryKey{ShopID: reference.ShopID, JobID: reference.JobID}
	record, found, err := store.Lock(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err = q.RecordNotificationReceipt(ctx, db.RecordNotificationReceiptParams{ShopID: reference.ShopID, AttemptID: r.AttemptID, ReceiptState: r.Stage}); err != nil {
		return err
	}
	if record.Token == r.AttemptID && (record.State == domain.DeliverySending || record.State == domain.DeliveryUnknown) {
		id := strings.ToUpper(strings.ReplaceAll(r.AttemptID.String(), "-", ""))
		if err = store.Complete(ctx, app.DeliveryClaim{Key: key, Token: r.AttemptID}, domain.SendOutcome{Disposition: domain.SendAccepted, ProviderMessageID: id}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
