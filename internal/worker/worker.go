package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Run is a lifecycle scaffold; it does not consume or send business messages.
func Run(ctx context.Context, checkDB func(context.Context) error, brokerClosed <-chan *amqp.Error, interval, timeout time.Duration, logger *slog.Logger) error {
	check := func() error {
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return checkDB(checkCtx)
	}
	if err := check(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New("worker PostgreSQL check failed")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Info("worker started", "mode", "foundation")
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return nil
		case <-brokerClosed:
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("worker RabbitMQ connection closed; restart required")
		case <-ticker.C:
			if err := check(); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("worker PostgreSQL check failed; restart required")
			}
			logger.Debug("worker dependencies available")
		}
	}
}
