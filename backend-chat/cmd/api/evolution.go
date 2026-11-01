package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/Felipe-candido/Barber-chat/internal/config"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi"
	notificationshttp "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/http"
)

// An optional webhook-only listener lets Docker reach the host without changing
// the administrative API's existing loopback binding. No credentials in logs.
func serveWithEvolutionWebhook(ctx context.Context, listener net.Listener, handler http.Handler, connections *notificationshttp.ConnectionHandler, cfg config.Config, logger *slog.Logger) error {
	if cfg.EvolutionWebhookAddr == "" {
		return httpapi.Serve(ctx, listener, handler, cfg.HTTPTimeout, cfg.ShutdownTimeout, logger)
	}
	webhookListener, err := net.Listen("tcp", cfg.EvolutionWebhookAddr)
	if err != nil {
		return err
	}
	defer webhookListener.Close()
	webhookMux := http.NewServeMux()
	webhookMux.HandleFunc("POST /api/v1/webhooks/evolution", connections.Webhook)
	serveCtx, stop := context.WithCancel(ctx)
	defer stop()
	results := make(chan error, 2)
	go func() {
		results <- httpapi.Serve(serveCtx, listener, handler, cfg.HTTPTimeout, cfg.ShutdownTimeout, logger)
	}()
	go func() {
		results <- httpapi.Serve(serveCtx, webhookListener, webhookMux, cfg.HTTPTimeout, cfg.ShutdownTimeout, logger)
	}()
	logger.Info("Evolution webhook listener opened", "address", webhookListener.Addr().String())
	first := <-results
	stop()
	second := <-results
	return errors.Join(first, second)
}
