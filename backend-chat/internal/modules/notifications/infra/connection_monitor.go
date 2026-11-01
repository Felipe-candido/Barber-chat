package infra

import (
	"context"
	"log/slog"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
)

// This monitor is independent of booking and of the future message consumer.
func RunConnectionMonitor(ctx context.Context, uc *app.ManageConnections, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		check, cancel := context.WithTimeout(ctx, 12*time.Second)
		var err error
		for range 10 {
			var count int
			count, err = uc.ReconcileOne(check)
			if err != nil || count == 0 {
				break
			}
		}
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.Warn("WhatsApp connection reconciliation deferred; check provider, activation and migrations")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
