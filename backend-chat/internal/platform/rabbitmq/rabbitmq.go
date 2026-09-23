package rabbitmq

import (
	"context"
	"errors"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Open(ctx context.Context, connectionURL string, timeout time.Duration) (*amqp.Connection, error) {
	if connectionURL == "" {
		return nil, errors.New("RABBITMQ_URL is required for the worker")
	}
	conn, err := amqp.DialConfig(connectionURL, amqp.Config{
		Heartbeat: 10 * time.Second,
		Dial: func(network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: timeout}
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			deadline := time.Now().Add(timeout)
			if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
				deadline = d
			}
			if err := conn.SetDeadline(deadline); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		},
	})
	if err != nil {
		return nil, errors.New("could not connect to RabbitMQ")
	}
	return conn, nil
}
