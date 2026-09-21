// Package events adapts the domain to RabbitMQ: topology, publishing, consuming.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/furkanpatat/movieapp/pkg/messaging"
	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

const (
	Exchange = "interaction.events" // topic

	KeyRatingSubmitted = "rating.submitted"
	KeyCommentAdded    = "comment.added"

	// Each queue gets its own dead-letter setup from pkg/messaging:
	// <queue>.dlx (exchange) -> <queue>.dlq (queue).
	QueueRatings  = "interaction.ratings"
	QueueComments = "interaction.comments"
)

// Topology names the exchange and queues. The zero value is production; tests
// set a Prefix to get private copies that cannot collide with a running stack.
type Topology struct{ Prefix string }

func (t Topology) exchange() string { return t.Prefix + Exchange }
func (t Topology) Ratings() string  { return t.Prefix + QueueRatings }
func (t Topology) Comments() string { return t.Prefix + QueueComments }

// Settings tunes the consumers.
type Settings struct {
	Topology   Topology
	Prefetch   int
	MaxRetries int
}

func consumerConfigs(s Settings, rating, comment messaging.Handler) []messaging.ConsumerConfig {
	t := s.Topology
	return []messaging.ConsumerConfig{
		{Exchange: t.exchange(), Queue: t.Ratings(), RoutingKeys: []string{KeyRatingSubmitted},
			Prefetch: s.Prefetch, MaxRetries: s.MaxRetries, Handler: rating},
		{Exchange: t.exchange(), Queue: t.Comments(), RoutingKeys: []string{KeyCommentAdded},
			Prefetch: s.Prefetch, MaxRetries: s.MaxRetries, Handler: comment},
	}
}

// DeclareTopology creates the exchange, queues, bindings and DLQs. Call it
// before publishing: a message published while no queue is bound is dropped.
func DeclareTopology(ctx context.Context, conn *messaging.Connection, t Topology) error {
	ch, err := conn.Channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()
	for _, cfg := range consumerConfigs(Settings{Topology: t}, nil, nil) {
		if err := messaging.DeclareTopology(ch, cfg); err != nil {
			return fmt.Errorf("declare %s: %w", cfg.Queue, err)
		}
	}
	return nil
}

// --- Publisher (used by the outbox relay) ---

type Publisher struct {
	p        *messaging.Publisher
	exchange string
}

var _ domain.OutboxPublisher = (*Publisher)(nil)

func NewPublisher(p *messaging.Publisher, t Topology) *Publisher {
	return &Publisher{p: p, exchange: t.exchange()}
}

// Publish routes an outbox message by its event type and waits for the broker
// confirm. The payload is forwarded verbatim.
func (p *Publisher) Publish(ctx context.Context, m domain.OutboxMessage) error {
	var key string
	switch m.Type {
	case domain.EventTypeRatingSubmitted:
		key = KeyRatingSubmitted
	case domain.EventTypeCommentAdded:
		key = KeyCommentAdded
	default:
		return fmt.Errorf("unknown event type %q", m.Type)
	}
	return p.p.Publish(ctx, p.exchange, key, "application/json", m.Payload, amqp.Table{"event_type": m.Type})
}

// --- Consumers ---

// Projector is what the consumers drive (implemented by service.Projector).
type Projector interface {
	HandleRating(ctx context.Context, e domain.RatingSubmitted) error
	HandleComment(ctx context.Context, e domain.CommentAdded) error
}

// RatingHandler / CommentHandler decode, validate and dispatch one message.
// Undecodable or invalid payloads can never succeed, so they are marked
// Permanent and go straight to the DLQ; everything else is retried.
func RatingHandler(p Projector) messaging.Handler {
	return func(ctx context.Context, d amqp.Delivery) error {
		var e domain.RatingSubmitted
		if err := decode(d.Body, &e); err != nil {
			return messaging.Permanent(err)
		}
		if err := e.Validate(); err != nil {
			return messaging.Permanent(err)
		}
		return classify(p.HandleRating(ctx, e))
	}
}

func CommentHandler(p Projector) messaging.Handler {
	return func(ctx context.Context, d amqp.Delivery) error {
		var e domain.CommentAdded
		if err := decode(d.Body, &e); err != nil {
			return messaging.Permanent(err)
		}
		if err := e.Validate(); err != nil {
			return messaging.Permanent(err)
		}
		return classify(p.HandleComment(ctx, e))
	}
}

func decode(b []byte, v any) error {
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	return nil
}

func classify(err error) error {
	if errors.Is(err, domain.ErrInvalidInput) {
		return messaging.Permanent(err)
	}
	return err
}

// Consume starts one consumer per queue and returns when ctx is cancelled and
// all of them have stopped.
func Consume(ctx context.Context, conn *messaging.Connection, p Projector, s Settings, log *slog.Logger) {
	var wg sync.WaitGroup
	for _, cfg := range consumerConfigs(s, RatingHandler(p), CommentHandler(p)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := messaging.Consume(ctx, conn, cfg, log); err != nil {
				log.Error("consumer stopped", "queue", cfg.Queue, "error", err)
			}
		}()
	}
	wg.Wait()
}
