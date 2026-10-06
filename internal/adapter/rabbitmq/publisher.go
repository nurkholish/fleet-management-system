package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"fleet-management-system/internal/domain"
)

var errPublisherClosed = errors.New("publisher closed")

type Publisher struct {
	url          string
	exchange     string
	exchangeType string
	queue        string
	routingKey   string
	dlx          string
	dlq          string

	mu     sync.Mutex
	conn   *amqp.Connection
	ch     *amqp.Channel
	closed atomic.Bool
}

// NewPublisher declares the exchange, main queue (with DLX), and DLQ.
func NewPublisher(url, exchange, exchangeType, queue, routingKey, dlx, dlq string) (*Publisher, error) {
	p := &Publisher{
		url:          url,
		exchange:     exchange,
		exchangeType: exchangeType,
		queue:        queue,
		routingKey:   routingKey,
		dlx:          dlx,
		dlq:          dlq,
	}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

// connect assumes the caller holds no lock and will set p.conn/p.ch on success.
func (p *Publisher) connect() error {
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("rabbit dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("rabbit channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("confirm mode: %w", err)
	}

	// DLX exchange (fanout).
	if err := ch.ExchangeDeclare(p.dlx, "fanout", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("dlx declare: %w", err)
	}
	if _, err := ch.QueueDeclare(p.dlq, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("dlq declare: %w", err)
	}
	if err := ch.QueueBind(p.dlq, "", p.dlx, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("dlq bind: %w", err)
	}

	// Main exchange — use configured exchange type.
	if err := ch.ExchangeDeclare(p.exchange, p.exchangeType, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("exchange declare: %w", err)
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    p.dlx,
		"x-dead-letter-routing-key": "dead",
	}
	if _, err := ch.QueueDeclare(p.queue, true, false, false, false, args); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("queue declare: %w", err)
	}
	if err := ch.QueueBind(p.queue, p.routingKey, p.exchange, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("queue bind: %w", err)
	}

	p.conn = conn
	p.ch = ch
	return nil
}

// PublishGeofenceEntry holds p.mu for the whole operation so that
func (p *Publisher) PublishGeofenceEntry(ctx context.Context, evt domain.GeofenceEvent) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed.Load() {
		return errPublisherClosed
	}

	// Reconnect if needed.
	if p.ch == nil || p.conn == nil || p.conn.IsClosed() {
		if p.ch != nil {
			_ = p.ch.Close()
			p.ch = nil
		}
		if p.conn != nil {
			_ = p.conn.Close()
			p.conn = nil
		}
		if err := p.connect(); err != nil {
			return err
		}
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	confirm, err := p.ch.PublishWithDeferredConfirmWithContext(
		pubCtx, p.exchange, p.routingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
			Body:         body,
		})
	if err != nil {
		// Force reconnect on next call.
		_ = p.ch.Close()
		p.ch = nil
		return fmt.Errorf("publish: %w", err)
	}

	select {
	case <-confirm.Done():
		if !confirm.Acked() {
			return errors.New("broker nack")
		}
	case <-pubCtx.Done():
		return fmt.Errorf("publish confirm timeout: %w", pubCtx.Err())
	}
	return nil
}

// IsHealthy is non-blocking on the fast path (atomic closed flag).
func (p *Publisher) IsHealthy() bool {
	if p.closed.Load() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn != nil && !p.conn.IsClosed() && p.ch != nil
}

func (p *Publisher) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	return nil
}
