//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/platform/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func requiredEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s must be set for integration tests", key)
	}
	return value
}

func TestPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, requiredEnv(t, "DATABASE_URL"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal("PostgreSQL ping failed")
	}
	var timezone string
	if err := pool.QueryRow(ctx, "SHOW timezone").Scan(&timezone); err != nil {
		t.Fatal(err)
	}
	if timezone != "UTC" {
		t.Fatalf("unexpected timezone %q", timezone)
	}
	var enabled bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'btree_gist')").Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("run migrations before integration tests")
	}
}

func TestRabbitMQRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := rabbitmq.Open(ctx, requiredEnv(t, "RABBITMQ_URL"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseDeadline(time.Now().Add(3 * time.Second))
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	queue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Confirm(false); err != nil {
		t.Fatal(err)
	}
	confirmations := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	deliveries, err := ch.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.PublishWithContext(ctx, "", queue.Name, true, false, amqp.Publishing{ContentType: "text/plain", Body: []byte("foundation-check")}); err != nil {
		t.Fatal(err)
	}
	select {
	case confirmation := <-confirmations:
		if !confirmation.Ack {
			t.Fatal("publication rejected")
		}
	case <-ctx.Done():
		t.Fatal("publisher confirm timed out")
	}
	select {
	case msg, ok := <-deliveries:
		if !ok || string(msg.Body) != "foundation-check" {
			t.Fatal("unexpected delivery")
		}
		if err := msg.Ack(false); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("delivery timed out")
	}
}
