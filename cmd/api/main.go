package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"

	catalogapp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	cataloghttp "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/http"
	catalogpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/infra/postgres"
	shopspostgres "github.com/Felipe-candido/Barber-chat/internal/modules/shops/infra/postgres"

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

func run(
	ctx context.Context,
	cfg config.Config,
	logger *slog.Logger,
) error {
	// Create one shared PostgreSQL pool.
	pool, err := postgres.Open(
		ctx,
		cfg.DatabaseURL,
		cfg.DBTimeout,
	)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Wire generated queries and concrete adapters.
	queries := db.New(pool)

	catalogRepository := catalogpostgres.NewRepository(queries)
	shopResolver := shopspostgres.NewResolver(queries)

	createService := catalogapp.NewCreateService(catalogRepository, shopResolver)
	listServices := catalogapp.NewListServices(catalogRepository, shopResolver)

	catalogHandler := cataloghttp.NewHandler(createService, listServices, cfg.DevShopSlug, cfg.DBTimeout, logger)
	if cfg.DevShopSlug != "" {
		logger.Warn("local catalog writes enabled; do not expose this listener through a proxy")
	}

	// Register HTTP adapters and dependency probes.
	handler := httpapi.NewHandler(
		pool.Ping,
		catalogHandler,
		cfg.DBTimeout,
		logger,
	)

	// Open the HTTP listener.
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	logger.Info(
		"HTTP listener opened",
		"address",
		listener.Addr().String(),
	)

	// Serve requests until shutdown.
	return httpapi.Serve(
		ctx,
		listener,
		handler,
		cfg.ShutdownTimeout,
		logger,
	)
}
