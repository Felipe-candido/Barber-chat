package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/platform/rabbitmq"
	"github.com/Felipe-candido/Barber-chat/internal/worker"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "worker")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).With("service", "worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, cfg, logger)
	stop()
	if err != nil {
		logger.Error("worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()
	conn, err := rabbitmq.Open(ctx, cfg.RabbitMQURL, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if !conn.IsClosed() {
			if err := conn.CloseDeadline(time.Now().Add(cfg.ShutdownTimeout)); err != nil {
				logger.Warn("RabbitMQ close failed")
			}
		}
	}()
	return worker.Run(ctx, pool.Ping, conn.NotifyClose(make(chan *amqp.Error, 1)), cfg.WorkerInterval, cfg.DBTimeout, logger)
}
