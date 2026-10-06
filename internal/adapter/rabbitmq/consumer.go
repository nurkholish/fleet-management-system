package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"

	"fleet-management-system/internal/domain"
)

type Consumer struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
	log   zerolog.Logger
}

// NewConsumer dials RabbitMQ, opens a channel, applies QoS, and DECLARES
func NewConsumer(
	url, exchange, exchangeType, queue, routingKey, dlx, dlq string,
	log zerolog.Logger,
) (*Consumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbit dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbit channel: %w", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("qos: %w", err)
	}

	// DLX (fanout).
	if err := ch.ExchangeDeclare(dlx, "fanout", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("dlx declare: %w", err)
	}
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("dlq declare: %w", err)
	}
	if err := ch.QueueBind(dlq, "", dlx, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("dlq bind: %w", err)
	}

	// Main exchange.
	if err := ch.ExchangeDeclare(exchange, exchangeType, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("exchange declare: %w", err)
	}

	// Main queue with DLX.
	args := amqp.Table{
		"x-dead-letter-exchange":    dlx,
		"x-dead-letter-routing-key": "dead",
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, args); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("queue declare: %w", err)
	}
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("queue bind: %w", err)
	}

	log.Info().
		Str("exchange", exchange).
		Str("queue", queue).
		Str("routing_key", routingKey).
		Msg("consumer topology declared")

	return &Consumer{conn: conn, ch: ch, queue: queue, log: log}, nil
}

// Consume returns nil on graceful context cancellation.
func (c *Consumer) Consume(ctx context.Context) error {
	msgs, err := c.ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	c.log.Info().Str("queue", c.queue).Msg("consumer started")

	for {
		select {
		case <-ctx.Done():
			c.log.Info().Msg("consumer context cancelled")
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("delivery channel closed")
			}
			c.handle(msg)
		}
	}
}

func (c *Consumer) handle(msg amqp.Delivery) {
	var evt domain.GeofenceEvent
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		c.log.Warn().
			Err(err).
			Str("body", string(msg.Body)).
			Msg("invalid event, routing to DLQ")
		_ = msg.Nack(false, false)
		return
	}

	c.log.Info().
		Str("vehicle", evt.VehicleID).
		Str("event", evt.Event).
		Str("geofence", evt.GeofenceID).
		Float64("lat", evt.Location.Latitude).
		Float64("lon", evt.Location.Longitude).
		Int64("ts", evt.Timestamp).
		Msg("geofence event consumed")

	if err := msg.Ack(false); err != nil {
		c.log.Warn().Err(err).Msg("ack failed")
	}
}

func (c *Consumer) Close() {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}
