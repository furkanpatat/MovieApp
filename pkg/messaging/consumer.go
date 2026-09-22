package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Handler processes one message. Return nil to ack. Return an error to retry
// (up to MaxRetries) and then dead-letter. Wrap with Permanent to skip retries.
type Handler func(ctx context.Context, d amqp.Delivery) error

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks an error as non-retryable (e.g. malformed payload): the
// message goes straight to the DLQ.
func Permanent(err error) error { return permanentError{err} }

const retryHeader = "x-retry-count"

// ConsumerConfig describes a queue, its bindings and its dead-letter setup.
type ConsumerConfig struct {
	Exchange     string        // source exchange (declared if non-empty)
	ExchangeKind string        // default "topic"
	Queue        string        // main queue; DLQ is Queue+".dlq"
	RoutingKeys  []string      // bindings from Exchange to Queue
	Prefetch     int           // default 10
	MaxRetries   int           // redeliveries before dead-lettering; default 3
	RetryDelay   time.Duration // default 1s (linear per attempt)
	Handler      Handler
}

// DLX and DLQ names derived from the queue.
func dlxName(queue string) string { return queue + ".dlx" }
func dlqName(queue string) string { return queue + ".dlq" }

func (c *ConsumerConfig) defaults() error {
	if c.Queue == "" || c.Handler == nil {
		return errors.New("messaging: Queue and Handler are required")
	}
	if c.ExchangeKind == "" {
		c.ExchangeKind = amqp.ExchangeTopic
	}
	if c.Prefetch <= 0 {
		c.Prefetch = 10
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	} else if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = time.Second
	}
	return nil
}

// DeclareTopology declares the exchange, the main queue with its dead-letter
// exchange, the DLQ and all bindings. It is idempotent.
func DeclareTopology(ch *amqp.Channel, cfg ConsumerConfig) error {
	if cfg.Queue == "" {
		return errors.New("messaging: Queue is required")
	}
	if cfg.ExchangeKind == "" {
		cfg.ExchangeKind = amqp.ExchangeTopic
	}
	dlx, dlq := dlxName(cfg.Queue), dlqName(cfg.Queue)

	if err := ch.ExchangeDeclare(dlx, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dlx: %w", err)
	}
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dlq: %w", err)
	}
	if err := ch.QueueBind(dlq, cfg.Queue, dlx, false, nil); err != nil {
		return fmt.Errorf("bind dlq: %w", err)
	}

	if _, err := ch.QueueDeclare(cfg.Queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    dlx,
		"x-dead-letter-routing-key": cfg.Queue,
	}); err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	if cfg.Exchange != "" {
		if err := ch.ExchangeDeclare(cfg.Exchange, cfg.ExchangeKind, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange: %w", err)
		}
		for _, key := range cfg.RoutingKeys {
			if err := ch.QueueBind(cfg.Queue, key, cfg.Exchange, false, nil); err != nil {
				return fmt.Errorf("bind queue: %w", err)
			}
		}
	}
	return nil
}

// Consume runs the consumer until ctx is cancelled, re-establishing topology
// and the subscription after every connection loss. It blocks.
func Consume(ctx context.Context, conn *Connection, cfg ConsumerConfig, log *slog.Logger) error {
	if err := cfg.defaults(); err != nil {
		return err
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("queue", cfg.Queue)

	for {
		err := consumeSession(ctx, conn, cfg, log)
		select {
		case <-ctx.Done(): // shutting down: a clean stop, not a failure
			return nil
		default:
		}
		if errors.Is(err, ErrClosed) {
			return err
		}
		log.Warn("consumer session ended, restarting", "error", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func consumeSession(ctx context.Context, conn *Connection, cfg ConsumerConfig, log *slog.Logger) error {
	ch, err := conn.Channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := DeclareTopology(ch, cfg); err != nil {
		return err
	}
	if err := ch.Qos(cfg.Prefetch, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.ConsumeWithContext(ctx, cfg.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	closed := ch.NotifyClose(make(chan *amqp.Error, 1))

	log.Info("consuming")
	for {
		select {
		case <-ctx.Done():
			return nil
		case cerr := <-closed:
			if cerr == nil {
				return errors.New("channel closed")
			}
			return cerr
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			handle(ctx, ch, cfg, log, d)
		}
	}
}

func handle(ctx context.Context, ch *amqp.Channel, cfg ConsumerConfig, log *slog.Logger, d amqp.Delivery) {
	err := safeCall(ctx, cfg.Handler, d)
	if err == nil {
		_ = d.Ack(false)
		return
	}

	retries := retryCount(d)
	var perm permanentError
	if errors.As(err, &perm) || retries >= int64(cfg.MaxRetries) {
		log.Error("dead-lettering message", "error", err, "retries", retries, "message_id", d.MessageId)
		_ = d.Nack(false, false) // routed to <queue>.dlq via the DLX
		return
	}

	log.Warn("handler failed, retrying", "error", err, "attempt", retries+1, "max", cfg.MaxRetries)
	select {
	case <-ctx.Done():
		_ = d.Nack(false, true) // shutting down: give it back untouched
		return
	case <-time.After(time.Duration(retries+1) * cfg.RetryDelay):
	}

	headers := amqp.Table{}
	for k, v := range d.Headers {
		headers[k] = v
	}
	headers[retryHeader] = retries + 1

	// Republish to the queue via the default exchange, then ack the original.
	// If the republish fails, requeue the original so nothing is lost.
	pubErr := ch.PublishWithContext(ctx, "", cfg.Queue, false, false, amqp.Publishing{
		ContentType:     d.ContentType,
		ContentEncoding: d.ContentEncoding,
		DeliveryMode:    amqp.Persistent,
		CorrelationId:   d.CorrelationId,
		MessageId:       d.MessageId,
		Type:            d.Type,
		Timestamp:       d.Timestamp,
		Headers:         headers,
		Body:            d.Body,
	})
	if pubErr != nil {
		log.Error("retry republish failed, requeueing", "error", pubErr)
		_ = d.Nack(false, true)
		return
	}
	_ = d.Ack(false)
}

func retryCount(d amqp.Delivery) int64 {
	switch v := d.Headers[retryHeader].(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case int:
		return int64(v)
	}
	return 0
}

// safeCall converts handler panics into errors so one bad message cannot
// kill the consumer.
func safeCall(ctx context.Context, h Handler, d amqp.Delivery) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return h(ctx, d)
}
