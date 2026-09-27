// Package service holds the Interaction use cases: command, projector, query.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// Command is the write side. It validates, builds an event and records it in
// the outbox (one local INSERT). It never talks to the broker or the read
// model: the relay publishes the event and the projector applies it later,
// so a RabbitMQ outage cannot fail a request.
type Command struct {
	out   domain.Outbox
	newID func() string
	now   func() time.Time
}

func NewCommand(out domain.Outbox) *Command {
	return &Command{
		out:   out,
		newID: uuid.NewString,
		now:   func() time.Time { return domain.NormalizeTime(time.Now()) },
	}
}

// SubmitRating returns the event id once the event is durably stored.
func (c *Command) SubmitRating(ctx context.Context, t domain.Title, userID string, score int) (string, error) {
	e := domain.RatingSubmitted{EventID: c.newID(), MediaType: t.Media, MovieID: t.ID, UserID: userID, Score: score, OccurredAt: c.now()}
	if err := e.Validate(); err != nil {
		return "", err
	}
	return e.EventID, c.enqueue(ctx, e.EventID, domain.EventTypeRatingSubmitted, e)
}

func (c *Command) SubmitComment(ctx context.Context, t domain.Title, userID, text string) (string, error) {
	e := domain.CommentAdded{EventID: c.newID(), MediaType: t.Media, MovieID: t.ID, UserID: userID, Text: text, OccurredAt: c.now()}
	if err := e.Validate(); err != nil {
		return "", err
	}
	return e.EventID, c.enqueue(ctx, e.EventID, domain.EventTypeCommentAdded, e)
}

func (c *Command) enqueue(ctx context.Context, id, typ string, event any) error {
	m, err := domain.NewOutboxMessage(id, typ, event)
	if err != nil {
		return err
	}
	if err := c.out.Enqueue(ctx, m); err != nil {
		return fmt.Errorf("store %s: %w", typ, err)
	}
	return nil
}
