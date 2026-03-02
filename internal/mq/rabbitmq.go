package mq

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type Consumer interface {
	Consume(ctx context.Context, queue string, handler func(context.Context, []byte) error) error
}

type RabbitMQ struct {
	conn *amqp.Connection
}

func New(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial amqp: %w", err)
	}
	return &RabbitMQ{conn: conn}, nil
}

func (r *RabbitMQ) Close() error {
	if r.conn == nil || r.conn.IsClosed() {
		return nil
	}
	return r.conn.Close()
}

func (r *RabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	}

	if err := ch.PublishWithContext(ctx, "", queue, false, false, pub); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

func (r *RabbitMQ) Consume(ctx context.Context, queue string, handler func(context.Context, []byte) error) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("deliveries channel closed")
			}

			if err := handler(ctx, d.Body); err != nil {
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}
