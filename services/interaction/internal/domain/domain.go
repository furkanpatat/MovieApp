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

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
)

// Media types. TMDB numbers movies and TV series separately (movie 1399 and
// tv 1399 are different titles), so a title is identified by both.
const (
	MediaMovie = "movie"
	MediaTV    = "tv"
)

// Title is what gets rated and commented on: a movie or a TV series.
type Title struct {
	Media string // MediaMovie or MediaTV
	ID    int    // TMDB id
}

// Movie is the title of a movie (what an id meant before series).
func Movie(id int) Title { return Title{Media: MediaMovie, ID: id} }

func (t Title) Validate() error {
	switch {
	case t.Media != MediaMovie && t.Media != MediaTV:
		return invalid("media_type must be %q or %q", MediaMovie, MediaTV)
	case t.ID < 1:
		return invalid("id must be positive")
	}
	return nil
}

// String is "movie:27205" / "tv:1399": unique across media types.
func (t Title) String() string { return fmt.Sprintf("%s:%d", t.Media, t.ID) }

// titleOf reads an event's (media_type, movie_id); events from before
// series have no media_type and are movies.
func titleOf(media string, id int) Title {
	if media == "" {
		media = MediaMovie
	}
	return Title{Media: media, ID: id}
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, a...))
}

// --- Events (the write side's contract; serialised as JSON onto RabbitMQ) ---

// RatingSubmitted: a user rated a movie. Re-rating replaces the earlier score.
type RatingSubmitted struct {
	EventID string `json:"event_id"`
	// MediaType and MovieID name the title (movie_id is the TMDB id of a
	// movie or a series; the name predates series). Empty MediaType: movie.
	MediaType  string    `json:"media_type,omitempty"`
	MovieID    int       `json:"movie_id"`
	UserID     string    `json:"user_id"`
	Score      int       `json:"score"`
	OccurredAt time.Time `json:"occurred_at"`
}

// CommentAdded: a user commented on a movie.
type CommentAdded struct {
	EventID    string    `json:"event_id"`
	MediaType  string    `json:"media_type,omitempty"` // see RatingSubmitted
	MovieID    int       `json:"movie_id"`
	UserID     string    `json:"user_id"`
	Text       string    `json:"text"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e RatingSubmitted) Title() Title { return titleOf(e.MediaType, e.MovieID) }
func (e CommentAdded) Title() Title    { return titleOf(e.MediaType, e.MovieID) }

func validateCommon(eventID string, title Title, userID string) error {
	if eventID == "" {
		return invalid("event_id is required")
	}
	if err := title.Validate(); err != nil {
		return err
	}
	switch {
	case strings.TrimSpace(userID) == "":
		return invalid("user_id is required")
	case utf8.RuneCountInString(userID) > MaxUserIDLen:
		return invalid("user_id must be at most %d characters", MaxUserIDLen)
	}
	return nil
}

func (e RatingSubmitted) Validate() error {
	if err := validateCommon(e.EventID, e.Title(), e.UserID); err != nil {
		return err
	}
	if e.Score < MinScore || e.Score > MaxScore {
		return invalid("score must be between %d and %d", MinScore, MaxScore)
	}
	return nil
}

func (e CommentAdded) Validate() error {
	if err := validateCommon(e.EventID, e.Title(), e.UserID); err != nil {
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
	Title      Title
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
	MediaType      string    `json:"media_type"`
	MovieID        int       `json:"movie_id"` // the TMDB id (of a movie or a series)
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
	GetStats(ctx context.Context, t Title) (RatingStats, error)
	RecentComments(ctx context.Context, t Title, limit int) ([]Comment, error)
	// PurgeUser deletes all ratings, comments, and unpublished events for a user.
	// It decrements the stats of affected movies and returns their updated stats.
	PurgeUser(ctx context.Context, userID string) ([]RatingStats, error)
}

// ModerationStore keeps comment reports and user blocks (PostgreSQL).
type ModerationStore interface {
	// ReportComment records that reporterID reported a comment. Reporting
	// twice is fine; a comment that doesn't exist is ErrNotFound.
	ReportComment(ctx context.Context, commentID, reporterID string) error
	// BlockUser makes blockerID hide blockedID's comments. Idempotent.
	BlockUser(ctx context.Context, blockerID, blockedID string) error
	UnblockUser(ctx context.Context, blockerID, blockedID string) error
	// BlockedUsers lists the users blockerID has blocked.
	BlockedUsers(ctx context.Context, blockerID string) ([]string, error)

	// For the admin: the reported comments, most reported first.
	ReportedComments(ctx context.Context, limit int) ([]ReportedComment, error)
	// DeleteComment removes a comment (and, with it, its reports) and says
	// which title it was on. ErrNotFound if there is no such comment.
	DeleteComment(ctx context.Context, commentID string) (Title, error)
	// DismissReports clears a comment's reports and keeps the comment. Idempotent.
	DismissReports(ctx context.Context, commentID string) error
}

// ReportedComment is a comment somebody reported.
type ReportedComment struct {
	Comment
	MediaType      string    `json:"media_type"`
	MovieID        int       `json:"movie_id"`
	Reports        int       `json:"reports"`
	LastReportedAt time.Time `json:"last_reported_at"`
}

// ReadModel is the materialised query view (Redis).
type ReadModel interface {
	// Get returns found=false when the movie has no materialised model.
	Get(ctx context.Context, t Title, limit int) (Interactions, bool, error)
	// ApplyRating updates the aggregate. applied=false means the model does
	// not exist yet and must be built with Init.
	ApplyRating(ctx context.Context, s RatingStats) (applied bool, err error)
	// AddComment appends a comment. applied=false: model missing, use Init.
	AddComment(ctx context.Context, t Title, c Comment) (applied bool, err error)
	// Init builds the model from source-of-truth data. It never overwrites a
	// newer aggregate (by Version) and merges comments idempotently.
	Init(ctx context.Context, s RatingStats, recent []Comment) error
	// Delete removes the materialised view, forcing a rebuild on the next read.
	Delete(ctx context.Context, t Title) error
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
