package rabbitmq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	amqp "github.com/rabbitmq/amqp091-go"
)

var ErrBrokerUnavailable = errors.New("notification broker unavailable")
var ErrUnroutable = errors.New("notification dispatch unroutable")

type Names struct{ Exchange, Queue, DeadExchange, DeadQueue string }

func DefaultNames() Names {
	return Names{"barber.notifications", "barber.notifications.whatsapp", "barber.notifications.dead", "barber.notifications.dead.whatsapp"}
}

type Broker struct {
	pub, consumer *amqp.Channel
	Names         Names
	Deliveries    <-chan amqp.Delivery
	confirms      <-chan amqp.Confirmation
	returns       <-chan amqp.Return
	mu            sync.Mutex
}

func OpenBroker(conn *amqp.Connection, names Names) (*Broker, error) {
	pub, err := conn.Channel()
	if err != nil {
		return nil, ErrBrokerUnavailable
	}
	consumer, err := conn.Channel()
	if err != nil {
		_ = pub.Close()
		return nil, ErrBrokerUnavailable
	}
	b := &Broker{pub: pub, consumer: consumer, Names: names}
	failed := true
	defer func() {
		if failed {
			b.Close()
		}
	}()
	for _, name := range []string{names.Exchange, names.DeadExchange} {
		if err = pub.ExchangeDeclare(name, "direct", true, false, false, false, nil); err != nil {
			return nil, ErrBrokerUnavailable
		}
	}
	if _, err = pub.QueueDeclare(names.DeadQueue, true, false, false, false, amqp.Table{"x-queue-type": "quorum", "x-message-ttl": int32(604800000), "x-max-length": int32(10000)}); err != nil {
		return nil, ErrBrokerUnavailable
	}
	if err = pub.QueueBind(names.DeadQueue, "whatsapp.dead", names.DeadExchange, false, nil); err != nil {
		return nil, ErrBrokerUnavailable
	}
	args := amqp.Table{"x-queue-type": "quorum", "x-max-length": int32(10000), "x-overflow": "reject-publish", "x-delivery-limit": int32(20), "x-dead-letter-exchange": names.DeadExchange, "x-dead-letter-routing-key": "whatsapp.dead"}
	if _, err = pub.QueueDeclare(names.Queue, true, false, false, false, args); err != nil {
		return nil, ErrBrokerUnavailable
	}
	if err = pub.QueueBind(names.Queue, "whatsapp.send", names.Exchange, false, nil); err != nil {
		return nil, ErrBrokerUnavailable
	}
	if err = pub.Confirm(false); err != nil {
		return nil, ErrBrokerUnavailable
	}
	b.confirms = pub.NotifyPublish(make(chan amqp.Confirmation, 1))
	b.returns = pub.NotifyReturn(make(chan amqp.Return, 1))
	if err = consumer.Qos(1, 0, false); err != nil {
		return nil, ErrBrokerUnavailable
	}
	b.Deliveries, err = consumer.Consume(names.Queue, "", false, false, false, false, nil)
	if err != nil {
		return nil, ErrBrokerUnavailable
	}
	failed = false
	return b, nil
}
func (b *Broker) Close() { _ = b.consumer.Close(); _ = b.pub.Close() }
func (b *Broker) Publish(ctx context.Context, e app.DispatchEnvelope) error {
	if !e.Valid() {
		return app.ErrInvalidNotificationEnvelope
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := json.Marshal(e)
	if err != nil {
		return app.ErrInvalidNotificationEnvelope
	}
	tag := b.pub.GetNextPublishSeqNo()
	if err = b.pub.PublishWithContext(ctx, b.Names.Exchange, "whatsapp.send", true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: e.OutboxID.String(), Timestamp: time.Now().UTC(), Body: body}); err != nil {
		return ErrBrokerUnavailable
	}
	returned := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r, ok := <-b.returns:
			if !ok {
				return ErrBrokerUnavailable
			}
			if r.MessageId != e.OutboxID.String() {
				return ErrBrokerUnavailable
			}
			returned = true
		case confirmation, ok := <-b.confirms:
			if !ok || confirmation.DeliveryTag != tag {
				return ErrBrokerUnavailable
			}
			// basic.return precedes the confirm; drain the buffered return even when
			// select picked the confirm first. One publication is in flight per channel.
			select {
			case r, ok := <-b.returns:
				if !ok || r.MessageId != e.OutboxID.String() {
					return ErrBrokerUnavailable
				}
				returned = true
			default:
			}
			if returned {
				return ErrUnroutable
			}
			if !confirmation.Ack {
				return ErrBrokerUnavailable
			}
			return nil
		}
	}
}
func DecodeEnvelope(body []byte) (app.DispatchEnvelope, error) {
	if len(body) > 1024 {
		return app.DispatchEnvelope{}, app.ErrInvalidNotificationEnvelope
	}
	var e *app.DispatchEnvelope
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&e) != nil || e == nil || !e.Valid() || d.Decode(new(any)) != io.EOF {
		return app.DispatchEnvelope{}, app.ErrInvalidNotificationEnvelope
	}
	return *e, nil
}
func (b *Broker) ConsumeLoop(ctx context.Context, u *app.DispatchNotifications, timeout time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-b.Deliveries:
			if !ok {
				return ErrBrokerUnavailable
			}
			envelope, err := DecodeEnvelope(delivery.Body)
			if err != nil || delivery.ContentType != "application/json" || delivery.MessageId != envelope.OutboxID.String() {
				if err = delivery.Reject(false); err != nil {
					return ErrBrokerUnavailable
				}
				continue
			}
			operation, cancel := context.WithTimeout(ctx, timeout)
			err = u.Consume(operation, envelope)
			cancel()
			if errors.Is(err, app.ErrInvalidNotificationEnvelope) {
				if err = delivery.Reject(false); err != nil {
					return ErrBrokerUnavailable
				}
				continue
			}
			if err != nil {
				return err
			} // Closing the session returns this unacked delivery.
			if err = delivery.Ack(false); err != nil {
				return ErrBrokerUnavailable
			}
		}
	}
}
