package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a lazy pool. Readiness checks connectivity separately.
func Open(ctx context.Context, databaseURL string, timeout time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	cfg.MaxConns = 5
	cfg.ConnConfig.ConnectTimeout = timeout
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not create PostgreSQL pool")
	}
	return pool, nil
}
