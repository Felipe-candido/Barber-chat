package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).With("service", "api")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, cfg, logger)
	stop()
	if err != nil {
		logger.Error("API stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("API stopped")
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	logger.Info("HTTP server started", "address", listener.Addr().String())
	return httpapi.Serve(ctx, listener, httpapi.NewHandler(pool.Ping, cfg.DBTimeout, logger), cfg.ShutdownTimeout, logger)
}
