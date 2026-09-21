// Package domain holds the Interaction entities, events and ports.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MinScore      = 1
	MaxScore      = 10
	MaxCommentLen = 1000 // runes
	MaxUserIDLen  = 64
)

var ErrInvalidInput = errors.New("invalid input")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, a...))
}

// --- Events (the write side's contract; serialised as JSON onto RabbitMQ) ---

// RatingSubmitted: a user rated a movie. Re-rating replaces the earlier score.
type RatingSubmitted struct {
	EventID    string    `json:"event_id"`
	MovieID    int       `json:"movie_id"`
	UserID     string    `json:"user_id"`
	Score      int       `json:"score"`
	OccurredAt time.Time `json:"occurred_at"`
}

// CommentAdded: a user commented on a movie.
type CommentAdded struct {
	EventID    string    `json:"event_id"`
	MovieID    int       `json:"movie_id"`
	UserID     string    `json:"user_id"`
	Text       string    `json:"text"`
	OccurredAt time.Time `json:"occurred_at"`
}

func validateCommon(eventID string, movieID int, userID string) error {
	switch {
	case eventID == "":
		return invalid("event_id is required")
	case movieID < 1:
		return invalid("movie id must be positive")
	case strings.TrimSpace(userID) == "":
		return invalid("user_id is required")
	case utf8.RuneCountInString(userID) > MaxUserIDLen:
		return invalid("user_id must be at most %d characters", MaxUserIDLen)
	}
	return nil
}

func (e RatingSubmitted) Validate() error {
	if err := validateCommon(e.EventID, e.MovieID, e.UserID); err != nil {
		return err
	}
	if e.Score < MinScore || e.Score > MaxScore {
		return invalid("score must be between %d and %d", MinScore, MaxScore)
	}
	return nil
}

func (e CommentAdded) Validate() error {
	if err := validateCommon(e.EventID, e.MovieID, e.UserID); err != nil {
		return err
	}
	if strings.TrimSpace(e.Text) == "" {
		return invalid("text is required")
	}
	if utf8.RuneCountInString(e.Text) > MaxCommentLen {
		return invalid("text must be at most %d characters", MaxCommentLen)
	}
	return nil
}

// --- Read-side models ---

// RatingStats is the running aggregate of a movie's ratings.
type RatingStats struct {
	MovieID    int
	TotalScore int64
	VoteCount  int64
	Version    int64 // bumped on every change; orders read-model writes
}

func (s RatingStats) Average() float64 {
	if s.VoteCount == 0 {
		return 0
	}
	return float64(s.TotalScore) / float64(s.VoteCount)
}

type Comment struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Interactions is what GET /interactions returns.
type Interactions struct {
	MovieID        int       `json:"movie_id"`
	AverageRating  float64   `json:"average_rating"`
	TotalVotes     int64     `json:"total_votes"`
	RecentComments []Comment `json:"recent_comments"`
}

// NormalizeTime makes event timestamps identical after a JSON or Postgres
// round trip (UTC, microsecond precision), so the same comment always
// serialises to the same bytes.
func NormalizeTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// --- Ports ---

// Repository is the durable source of truth (PostgreSQL).
type Repository interface {
	// SaveRating upserts the user's rating and updates the aggregate atomically.
	// It is idempotent, and ignores events older than the stored rating.
	// It returns the aggregate as of this call.
	SaveRating(ctx context.Context, e RatingSubmitted) (RatingStats, error)
	// SaveComment stores the comment; redelivered event ids are ignored.
	SaveComment(ctx context.Context, e CommentAdded) error
	GetStats(ctx context.Context, movieID int) (RatingStats, error)
	RecentComments(ctx context.Context, movieID, limit int) ([]Comment, error)
}

// ReadModel is the materialised query view (Redis).
type ReadModel interface {
	// Get returns found=false when the movie has no materialised model.
	Get(ctx context.Context, movieID, limit int) (Interactions, bool, error)
	// ApplyRating updates the aggregate. applied=false means the model does
	// not exist yet and must be built with Init.
	ApplyRating(ctx context.Context, s RatingStats) (applied bool, err error)
	// AddComment appends a comment. applied=false: model missing, use Init.
	AddComment(ctx context.Context, movieID int, c Comment) (applied bool, err error)
	// Init builds the model from source-of-truth data. It never overwrites a
	// newer aggregate (by Version) and merges comments idempotently.
	Init(ctx context.Context, s RatingStats, recent []Comment) error
}

// --- Transactional outbox ---

const (
	EventTypeRatingSubmitted = "RatingSubmitted"
	EventTypeCommentAdded    = "CommentAdded"
)

// OutboxMessage is a serialised event waiting to be published.
type OutboxMessage struct {
	ID      string // the event id
	Type    string // EventType*
	Payload []byte // JSON
}

// NewOutboxMessage serialises an event.
func NewOutboxMessage(id, eventType string, event any) (OutboxMessage, error) {
	b, err := json.Marshal(event)
	if err != nil {
		return OutboxMessage{}, err
	}
	return OutboxMessage{ID: id, Type: eventType, Payload: b}, nil
}

// Outbox is the write path's only dependency: durably record an event.
// It is a single local INSERT, so it succeeds whether or not the broker is up.
type Outbox interface {
	Enqueue(ctx context.Context, m OutboxMessage) error
}

// OutboxStore is what the relay drives.
type OutboxStore interface {
	// PublishPending locks up to limit pending messages (oldest first, skipping
	// rows locked by other relays), calls publish for each in order, and marks
	// the successful ones published. It stops at the first publish error, which
	// it returns along with the number already published.
	PublishPending(ctx context.Context, limit int, publish func(context.Context, OutboxMessage) error) (int, error)
	// DeleteExpired removes up to limit published rows older than retention.
	DeleteExpired(ctx context.Context, retention time.Duration, limit int) (int64, error)
	PendingCount(ctx context.Context) (int64, error)
}

// OutboxPublisher sends one outbox message to the broker and returns once the
// broker has confirmed it.
type OutboxPublisher interface {
	Publish(ctx context.Context, m OutboxMessage) error
}
