package infra

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	notificationmq "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/rabbitmq"
	platformmq "github.com/Felipe-candido/Barber-chat/internal/platform/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type WorkerOptions struct {
	BrokerURL                                    string
	DBTimeout, OperationTimeout, ShutdownTimeout time.Duration
	Names                                        notificationmq.Names
}

// RunNotificationWorker is a separately started process. API request-mode
// delivery never starts this loop or requires a RabbitMQ connection.
func RunNotificationWorker(ctx context.Context, options WorkerOptions, u *app.DispatchNotifications, store app.DispatchStore, connections *app.ManageConnections, logger *slog.Logger) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	workerID, err := uuid.NewRandom()
	if err != nil {
		return app.ErrDeliveryUnavailable
	}
	logger = logger.With("worker_id", workerID)
	var healthy atomic.Bool
	var background sync.WaitGroup
	background.Go(func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var maintained time.Time
		var measured time.Time
		for {
			operation, cancel := context.WithTimeout(ctx, options.OperationTimeout)
			_, err := u.Schedule(operation)
			if err == nil && time.Since(measured) >= time.Minute {
				if m, metricErr := store.Metrics(operation); metricErr == nil {
					logger.Info("notification worker metrics", "outbox_pending", m.OutboxPending, "uncertain", m.Uncertain, "sending", m.Sending, "oldest_due_seconds", m.OldestDueSeconds)
					measured = time.Now()
				}
			}
			if err == nil && time.Since(maintained) >= 5*time.Minute {
				if maintenanceErr := u.Maintain(operation); maintenanceErr != nil {
					logger.Warn("notification payload maintenance deferred")
				} else {
					maintained = time.Now()
				}
			}
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Warn("notification scheduler deferred")
			}
			heartbeat, cancel := context.WithTimeout(context.WithoutCancel(ctx), options.DBTimeout)
			_ = store.Heartbeat(heartbeat, workerID, healthy.Load() && err == nil)
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	if connections != nil {
		background.Go(func() { RunConnectionMonitor(ctx, connections, logger) })
	}
	defer func() {
		stop()
		healthy.Store(false)
		background.Wait()
		cleanup, cancel := context.WithTimeout(context.Background(), options.DBTimeout)
		defer cancel()
		_ = store.Heartbeat(cleanup, workerID, false)
	}()
	backoff := time.Second
	logger.Info("notification worker started")
	for ctx.Err() == nil {
		conn, err := platformmq.Open(ctx, options.BrokerURL, options.DBTimeout)
		if err == nil {
			err = runNotificationSession(ctx, conn, options, u, &healthy)
			healthy.Store(false)
			if !conn.IsClosed() {
				_ = conn.CloseDeadline(time.Now().Add(options.ShutdownTimeout))
			}
		}
		if ctx.Err() != nil {
			break
		}
		logger.Warn("notification broker session interrupted; reconnect scheduled")
		timer := time.NewTimer(backoff + time.Duration(rand.Int64N(int64(backoff/4+1))))
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	stop()
	logger.Info("notification worker stopped")
	return nil
}

func runNotificationSession(ctx context.Context, conn *amqp.Connection, o WorkerOptions, u *app.DispatchNotifications, healthy *atomic.Bool) error {
	broker, err := notificationmq.OpenBroker(conn, o.Names)
	if err != nil {
		return err
	}
	defer broker.Close()
	session, cancel := context.WithCancel(ctx)
	defer cancel()
	healthy.Store(true)
	results := make(chan error, 2)
	go func() { results <- broker.ConsumeLoop(session, u, o.OperationTimeout) }()
	go func() {
		for {
			operation, stop := context.WithTimeout(session, 10*time.Second)
			found, err := u.RelayOne(operation, broker)
			stop()
			if err != nil {
				results <- err
				return
			}
			if !found {
				timer := time.NewTimer(500 * time.Millisecond)
				select {
				case <-session.Done():
					timer.Stop()
					results <- nil
					return
				case <-timer.C:
				}
			}
			if session.Err() != nil {
				results <- nil
				return
			}
		}
	}()
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	var first error
	finished := 0
	select {
	case <-ctx.Done():
	case <-closed:
		first = notificationmq.ErrBrokerUnavailable
	case first = <-results:
		finished = 1
	}
	cancel()
	healthy.Store(false)
	if !conn.IsClosed() {
		_ = conn.CloseDeadline(time.Now().Add(o.ShutdownTimeout))
	}
	for finished < 2 {
		<-results
		finished++
	}
	return first
}
