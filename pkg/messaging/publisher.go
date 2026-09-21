package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher publishes persistent messages with broker confirms. It re-opens
// its channel after failures and retries a bounded number of times.
type Publisher struct {
	conn     *Connection
	attempts int

	mu sync.Mutex
	ch *amqp.Channel
}

// NewPublisher creates a Publisher. attempts <= 0 defaults to 3.
func NewPublisher(conn *Connection, attempts int) *Publisher {
	if attempts <= 0 {
		attempts = 3
	}
	return &Publisher{conn: conn, attempts: attempts}
}

// DeclareExchange idempotently declares a durable exchange.
func (p *Publisher) DeclareExchange(ctx context.Context, name, kind string) error {
	ch, err := p.conn.Channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.ExchangeDeclare(name, kind, true, false, false, false, nil)
}

// Publish sends body to exchange/routingKey and waits for the broker confirm.
// Optional headers are attached to the message.
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey, contentType string, body []byte, headers amqp.Table) error {
	var lastErr error
	for i := 0; i < p.attempts; i++ {
		if lastErr = p.publishOnce(ctx, exchange, routingKey, contentType, body, headers); lastErr == nil {
			return nil
		}
		if ctx.Err() != nil || errors.Is(lastErr, ErrClosed) {
			return lastErr
		}
		p.resetChannel()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 200 * time.Millisecond):
		}
	}
	return fmt.Errorf("messaging: publish failed after %d attempts: %w", p.attempts, lastErr)
}

func (p *Publisher) publishOnce(ctx context.Context, exchange, key, contentType string, body []byte, headers amqp.Table) error {
	ch, err := p.channel(ctx)
	if err != nil {
		return err
	}
	conf, err := ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, true, false, amqp.Publishing{
		ContentType:  contentType,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		return err
	}
	acked, err := conf.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !acked {
		return errors.New("messaging: broker nacked message")
	}
	return nil
}

func (p *Publisher) channel(ctx context.Context) (*amqp.Channel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil && !p.ch.IsClosed() {
		return p.ch, nil
	}
	ch, err := p.conn.Channel(ctx)
	if err != nil {
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, err
	}
	p.ch = ch
	return ch, nil
}

func (p *Publisher) resetChannel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
}

// Close releases the publisher's channel.
func (p *Publisher) Close() error {
	p.resetChannel()
	return nil
}
