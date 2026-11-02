//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra"
	notificationmq "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/rabbitmq"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/infra/simulation"
	platformmq "github.com/Felipe-candido/Barber-chat/internal/platform/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func notificationBrokerURL(t *testing.T) string {
	t.Helper()
	raw := requiredEnv(t, "NOTIFICATIONS_TEST_RABBITMQ_URL")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "amqp" || u.Hostname() != "127.0.0.1" || u.Port() != "55442" {
		t.Fatal("requires isolated test RabbitMQ on 127.0.0.1:55442")
	}
	return raw
}
func isolatedBrokerNames() notificationmq.Names {
	p := "codex.notifications." + uuid.NewString()
	return notificationmq.Names{Exchange: p, Queue: p + ".whatsapp", DeadExchange: p + ".dead", DeadQueue: p + ".dead.whatsapp"}
}
func brokerFixture(t *testing.T) (*amqp.Connection, *notificationmq.Broker) {
	t.Helper()
	conn, err := platformmq.Open(context.Background(), notificationBrokerURL(t), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !conn.IsClosed() {
			_ = conn.CloseDeadline(time.Now().Add(2 * time.Second))
		}
	})
	broker, err := notificationmq.OpenBroker(conn, isolatedBrokerNames())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	return conn, broker
}
func receiveDispatch(t *testing.T, b *notificationmq.Broker) amqp.Delivery {
	t.Helper()
	select {
	case d, ok := <-b.Deliveries:
		if !ok {
			t.Fatal("broker delivery stream closed")
		}
		return d
	case <-time.After(10 * time.Second):
		t.Fatal("broker did not deliver dispatch")
	}
	return amqp.Delivery{}
}
func consumeDispatch(t *testing.T, p *app.DispatchNotifications, d amqp.Delivery) {
	t.Helper()
	e, err := notificationmq.DecodeEnvelope(d.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Consume(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}

type notificationPublisherFunc func(context.Context, app.DispatchEnvelope) error

func (f notificationPublisherFunc) Publish(ctx context.Context, e app.DispatchEnvelope) error {
	return f(ctx, e)
}

func TestNotificationRabbitMQ(t *testing.T) {
	t.Run("lost publisher confirmation republishes same envelope without duplicate send", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		sender := simulation.NewSender(domain.SendAccepted)
		pipeline := dispatchPipeline(f, store, sender)
		_, broker := brokerFixture(t)
		if _, err := pipeline.Schedule(f.ctx); err != nil {
			t.Fatal(err)
		}
		lost := notificationPublisherFunc(func(ctx context.Context, e app.DispatchEnvelope) error {
			if err := broker.Publish(ctx, e); err != nil {
				return err
			}
			return errors.New("injected loss after broker accepted publication")
		})
		if _, err := pipeline.RelayOne(f.ctx, lost); err == nil {
			t.Fatal("lost confirmation asserted success")
		}
		first := receiveDispatch(t, broker)
		var published bool
		if err := f.pool.QueryRow(f.ctx, "SELECT published_at IS NOT NULL FROM notification_outbox WHERE shop_id=$1", f.shop).Scan(&published); err != nil || published {
			t.Fatal("uncertain publish marked confirmed", err)
		}
		f.exec(t, "UPDATE notification_outbox SET next_publish_at=clock_timestamp() WHERE shop_id=$1", f.shop)
		if _, err := pipeline.RelayOne(f.ctx, broker); err != nil {
			t.Fatal(err)
		}
		consumeDispatch(t, pipeline, first)
		if err := first.Ack(false); err != nil {
			t.Fatal(err)
		}
		second := receiveDispatch(t, broker)
		if string(first.Body) != string(second.Body) {
			t.Fatal("republish changed durable envelope")
		}
		consumeDispatch(t, pipeline, second)
		if err := second.Ack(false); err != nil {
			t.Fatal(err)
		}
		if sender.Calls() != 1 || f.count(t, "notification_attempts") != 1 {
			t.Fatal("publish ambiguity duplicated send")
		}
	})
	t.Run("lost consumer ACK redelivers without repeating send", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		sender := simulation.NewSender(domain.SendAccepted)
		pipeline := dispatchPipeline(f, store, sender)
		conn, broker := brokerFixture(t)
		if _, err := pipeline.Schedule(f.ctx); err != nil {
			t.Fatal(err)
		}
		if found, err := pipeline.RelayOne(f.ctx, broker); err != nil || !found {
			t.Fatal("confirmed publication failed", err)
		}
		first := receiveDispatch(t, broker)
		consumeDispatch(t, pipeline, first)
		broker.Close() // No ACK: the broker must return the exact delivery.
		next, err := notificationmq.OpenBroker(conn, broker.Names)
		if err != nil {
			t.Fatal(err)
		}
		defer next.Close()
		repeated := receiveDispatch(t, next)
		if !repeated.Redelivered {
			t.Fatal("missing broker redelivery")
		}
		consumeDispatch(t, pipeline, repeated)
		if err = repeated.Ack(false); err != nil {
			t.Fatal(err)
		}
		if sender.Calls() != 1 || f.state(t) != "accepted" {
			t.Fatal("lost ACK duplicated external effect")
		}
	})
	t.Run("mandatory return is not marked published", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		pipeline := dispatchPipeline(f, store, simulation.NewSender(domain.SendAccepted))
		conn, broker := brokerFixture(t)
		admin, err := conn.Channel()
		if err != nil {
			t.Fatal(err)
		}
		defer admin.Close()
		if err = admin.QueueUnbind(broker.Names.Queue, "whatsapp.send", broker.Names.Exchange, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = pipeline.Schedule(f.ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = pipeline.RelayOne(f.ctx, broker); !errors.Is(err, notificationmq.ErrUnroutable) {
			t.Fatal("unroutable publish considered confirmed", err)
		}
		var published bool
		if err = f.pool.QueryRow(f.ctx, "SELECT published_at IS NOT NULL FROM notification_outbox WHERE shop_id=$1", f.shop).Scan(&published); err != nil || published {
			t.Fatal("unrouted outbox marked published", err)
		}
		if err = admin.QueueBind(broker.Names.Queue, "whatsapp.send", broker.Names.Exchange, false, nil); err != nil {
			t.Fatal(err)
		}
		f.exec(t, "UPDATE notification_outbox SET next_publish_at=clock_timestamp() WHERE shop_id=$1", f.shop)
		if _, err = pipeline.RelayOne(f.ctx, broker); err != nil {
			t.Fatal("channel did not drain returned confirm", err)
		}
		d := receiveDispatch(t, broker)
		consumeDispatch(t, pipeline, d)
		_ = d.Ack(false)
	})
	t.Run("invalid message goes to DLQ without a send", func(t *testing.T) {
		f, store := dispatchFixture(t, "automatic")
		sender := simulation.NewSender(domain.SendAccepted)
		pipeline := dispatchPipeline(f, store, sender)
		conn, broker := brokerFixture(t)
		ctx, cancel := context.WithCancel(f.ctx)
		done := make(chan error, 1)
		go func() { done <- broker.ConsumeLoop(ctx, pipeline, 5*time.Second) }()
		defer func() { cancel(); <-done }()
		admin, err := conn.Channel()
		if err != nil {
			t.Fatal(err)
		}
		defer admin.Close()
		if err = admin.PublishWithContext(f.ctx, broker.Names.Exchange, "whatsapp.send", true, false, amqp.Publishing{MessageId: "invalid", ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: []byte(`{"version":999}`)}); err != nil {
			t.Fatal(err)
		}
		waitNotification(t, 10*time.Second, func() bool { _, found, err := admin.Get(broker.Names.DeadQueue, true); return err == nil && found })
		if sender.Calls() != 0 {
			t.Fatal("invalid envelope reached sender")
		}
	})
	t.Run("handshake deadline does not close a healthy connection", func(t *testing.T) {
		conn, err := platformmq.Open(context.Background(), notificationBrokerURL(t), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		time.Sleep(1500 * time.Millisecond)
		channel, err := conn.Channel()
		if err != nil {
			t.Fatal("dial deadline leaked into connection lifetime", err)
		}
		_ = channel.Close()
	})
}

func waitNotification(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("notification condition did not converge")
}

// This proxy drops a real AMQP TCP session while keeping the address available
// for reconnection. No Docker commands or unrelated connections are touched.
func notificationProxy(t *testing.T, raw string) (string, func()) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	active := map[net.Conn]struct{}{}
	var wg sync.WaitGroup
	closeActive := func() {
		mu.Lock()
		connections := make([]net.Conn, 0, len(active))
		for c := range active {
			connections = append(connections, c)
		}
		mu.Unlock()
		for _, c := range connections {
			_ = c.Close()
		}
	}
	wg.Go(func() {
		for {
			downstream, err := listener.Accept()
			if err != nil {
				return
			}
			upstream, err := net.DialTimeout("tcp", u.Host, 2*time.Second)
			if err != nil {
				_ = downstream.Close()
				continue
			}
			mu.Lock()
			active[downstream] = struct{}{}
			active[upstream] = struct{}{}
			mu.Unlock()
			wg.Go(func() {
				defer func() {
					_ = downstream.Close()
					_ = upstream.Close()
					mu.Lock()
					delete(active, downstream)
					delete(active, upstream)
					mu.Unlock()
				}()
				copied := make(chan struct{})
				go func() { _, _ = io.Copy(upstream, downstream); _ = upstream.Close(); close(copied) }()
				_, _ = io.Copy(downstream, upstream)
				_ = downstream.Close()
				<-copied
			})
		}
	})
	t.Cleanup(func() { _ = listener.Close(); closeActive(); wg.Wait() })
	proxied := *u
	proxied.Host = listener.Addr().String()
	return proxied.String(), closeActive
}

func TestNotificationWorkerReconnectsAndStops(t *testing.T) {
	f, store := dispatchFixture(t, "automatic")
	sender := simulation.NewSender(domain.SendAccepted)
	pipeline := dispatchPipeline(f, store, sender)
	proxy, drop := notificationProxy(t, notificationBrokerURL(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	options := notifications.WorkerOptions{BrokerURL: proxy, DBTimeout: 2 * time.Second, OperationTimeout: 10 * time.Second, ShutdownTimeout: 3 * time.Second, Names: isolatedBrokerNames()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() { done <- notifications.RunNotificationWorker(ctx, options, pipeline, store, nil, logger) }()
	stopped := false
	defer func() {
		if !stopped {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("worker shutdown did not drain")
			}
		}
	}()
	waitNotification(t, 20*time.Second, func() bool { return f.state(t) == "accepted" })
	drop()
	next, _ := dispatchFixture(t, "automatic")
	waitNotification(t, 30*time.Second, func() bool { return next.state(t) == "accepted" })
	if sender.Calls() != 2 {
		t.Fatal("reconnection duplicated sends")
	}
	cancel()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop")
	}
}
