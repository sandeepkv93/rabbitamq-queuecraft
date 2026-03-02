package mq

import (
	"context"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type Consumer interface {
	Consume(ctx context.Context, queue string, handler func(context.Context, []byte) error) error
}

type TopologyConfig struct {
	QueueName      string
	MaxRetries     int
	RetryDelay     time.Duration
	ExchangeSuffix string
}

type RabbitMQ struct {
	conn *amqp.Connection
	cfg  TopologyConfig
}

func New(url string, cfg TopologyConfig) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial amqp: %w", err)
	}

	if cfg.QueueName == "" {
		_ = conn.Close()
		return nil, fmt.Errorf("queue name is required")
	}
	if cfg.MaxRetries < 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("max retries must be >= 0")
	}
	if cfg.RetryDelay <= 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("retry delay must be > 0")
	}
	if cfg.ExchangeSuffix == "" {
		cfg.ExchangeSuffix = ".exchange"
	}

	return &RabbitMQ{conn: conn, cfg: cfg}, nil
}

func (r *RabbitMQ) Close() error {
	if r.conn == nil || r.conn.IsClosed() {
		return nil
	}
	return r.conn.Close()
}

func (r *RabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	if queue != r.cfg.QueueName {
		return fmt.Errorf("unexpected queue name %q, expected %q", queue, r.cfg.QueueName)
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	if err := r.declareTopology(ch); err != nil {
		return err
	}

	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Headers:      amqp.Table{"x-retry-count": int32(0)},
		Body:         body,
	}

	if err := ch.PublishWithContext(ctx, r.exchangeName(), r.mainRoutingKey(), false, false, pub); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

func (r *RabbitMQ) Consume(ctx context.Context, queue string, handler func(context.Context, []byte) error) error {
	if queue != r.cfg.QueueName {
		return fmt.Errorf("unexpected queue name %q, expected %q", queue, r.cfg.QueueName)
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	if err := r.declareTopology(ch); err != nil {
		return err
	}

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(r.mainQueueName(), "", false, false, false, false, nil)
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
				if handleErr := r.handleDeliveryFailure(ctx, ch, d, err); handleErr != nil {
					_ = d.Nack(false, true)
					continue
				}
				_ = d.Ack(false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func (r *RabbitMQ) handleDeliveryFailure(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, processErr error) error {
	nextRetry := extractRetryCount(d.Headers) + 1
	pub := amqp.Publishing{
		ContentType:  d.ContentType,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         d.Body,
		Headers:      cloneHeaders(d.Headers),
	}
	pub.Headers["x-retry-count"] = int32(nextRetry)
	pub.Headers["x-last-error"] = processErr.Error()

	routingKey := r.retryRoutingKey()
	if nextRetry > r.cfg.MaxRetries {
		routingKey = r.deadRoutingKey()
	}

	if err := ch.PublishWithContext(ctx, r.exchangeName(), routingKey, false, false, pub); err != nil {
		return fmt.Errorf("republish failed message: %w", err)
	}
	return nil
}

func (r *RabbitMQ) declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(r.exchangeName(), "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	mainArgs := amqp.Table{
		"x-dead-letter-exchange":    r.exchangeName(),
		"x-dead-letter-routing-key": r.retryRoutingKey(),
	}
	if _, err := ch.QueueDeclare(r.mainQueueName(), true, false, false, false, mainArgs); err != nil {
		return fmt.Errorf("declare main queue: %w", err)
	}
	if err := ch.QueueBind(r.mainQueueName(), r.mainRoutingKey(), r.exchangeName(), false, nil); err != nil {
		return fmt.Errorf("bind main queue: %w", err)
	}

	retryArgs := amqp.Table{
		"x-message-ttl":             int32(r.cfg.RetryDelay.Milliseconds()),
		"x-dead-letter-exchange":    r.exchangeName(),
		"x-dead-letter-routing-key": r.mainRoutingKey(),
	}
	if _, err := ch.QueueDeclare(r.retryQueueName(), true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare retry queue: %w", err)
	}
	if err := ch.QueueBind(r.retryQueueName(), r.retryRoutingKey(), r.exchangeName(), false, nil); err != nil {
		return fmt.Errorf("bind retry queue: %w", err)
	}

	if _, err := ch.QueueDeclare(r.deadQueueName(), true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead queue: %w", err)
	}
	if err := ch.QueueBind(r.deadQueueName(), r.deadRoutingKey(), r.exchangeName(), false, nil); err != nil {
		return fmt.Errorf("bind dead queue: %w", err)
	}

	return nil
}

func (r *RabbitMQ) exchangeName() string {
	return r.cfg.QueueName + r.cfg.ExchangeSuffix
}

func (r *RabbitMQ) mainQueueName() string {
	return r.cfg.QueueName
}

func (r *RabbitMQ) retryQueueName() string {
	return r.cfg.QueueName + ".retry"
}

func (r *RabbitMQ) deadQueueName() string {
	return r.cfg.QueueName + ".dead"
}

func (r *RabbitMQ) mainRoutingKey() string {
	return r.cfg.QueueName
}

func (r *RabbitMQ) retryRoutingKey() string {
	return r.cfg.QueueName + ".retry"
}

func (r *RabbitMQ) deadRoutingKey() string {
	return r.cfg.QueueName + ".dead"
}

func extractRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}

	raw, ok := headers["x-retry-count"]
	if !ok {
		return 0
	}

	switch v := raw.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func cloneHeaders(headers amqp.Table) amqp.Table {
	if headers == nil {
		return amqp.Table{}
	}
	cloned := make(amqp.Table, len(headers))
	for k, v := range headers {
		cloned[k] = v
	}
	return cloned
}
