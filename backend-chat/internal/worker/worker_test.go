package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	check := func(context.Context) error { cancel(); return nil }
	if err := Run(ctx, check, make(chan *amqp.Error), time.Hour, time.Second, slog.New(slog.NewJSONHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyFailures(t *testing.T) {
	for _, failDB := range []bool{true, false} {
		closed := make(chan *amqp.Error)
		close(closed)
		check := func(context.Context) error {
			if failDB {
				return errors.New("offline")
			}
			return nil
		}
		if err := Run(context.Background(), check, closed, time.Hour, time.Second, slog.New(slog.NewJSONHandler(io.Discard, nil))); err == nil {
			t.Fatal("expected dependency failure")
		}
	}
}
