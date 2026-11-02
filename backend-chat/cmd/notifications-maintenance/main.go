package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	notificationspg "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
)

// Explicit one-shot maintenance for deployments without a continuously running
// worker. Does not send messages, connect to a broker or delete appointments.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "notifications-maintenance")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		logger.Error("notification database unavailable")
		os.Exit(1)
	}
	defer pool.Close()
	if err = notificationspg.NewDispatchStore(pool).Maintenance(ctx); err != nil {
		logger.Error("notification maintenance failed; check migrations and database")
		os.Exit(1)
	}
	logger.Info("expired notification payloads and old transport metadata maintained")
}
