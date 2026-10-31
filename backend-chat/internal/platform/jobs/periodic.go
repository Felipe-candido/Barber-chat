package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Run serializes bounded attempts. Persisted business deadlines, not this timer,
// determine eligibility; missed ticks and process restarts do not lose work.
func Run(ctx context.Context, interval, timeout time.Duration, execute func(context.Context) (int, error), logger *slog.Logger) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	run := func() {
		jobCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		count, err := execute(jobCtx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Warn("appointment completion deferred; check migrations and database availability")
			return
		}
		if count > 0 {
			logger.Info("appointments completed automatically", "count", count)
		}
	}
	if ctx.Err() != nil {
		return
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
