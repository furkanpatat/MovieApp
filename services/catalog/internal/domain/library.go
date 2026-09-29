package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	MinRating = 1
	MaxRating = 10
)

var (
	// ErrMovieNotStored: the title is not in its table (movies or tv_shows)
	// yet, so a library row cannot reference it. The service stores it and
	// retries.
	ErrMovieNotStored = errors.New("movie not stored")
	// ErrUnknownUser: the authenticated user id has no account (deleted
	// after the token was issued).
	ErrUnknownUser = errors.New("unknown user")
)

// TitleRef names a title in a user's library: a movie or a TV series (TMDB
// numbers them separately, so the id alone is ambiguous).
type TitleRef struct {
	MediaType string // MediaMovie or MediaTV
	ID        int
}

// MovieRef is a movie's reference (what a bare id meant before series).
func MovieRef(id int) TitleRef { return TitleRef{MediaType: MediaMovie, ID: id} }

// ParseTitleRef reads a request's (media_type, id); an empty media_type is a
// movie, as before series.
func ParseTitleRef(mediaType string, id int) (TitleRef, error) {
	if mediaType == "" {
		mediaType = MediaMovie
	}
	ref := TitleRef{MediaType: mediaType, ID: id}
	switch {
	case mediaType != MediaMovie && mediaType != MediaTV:
		return ref, fmt.Errorf("%w: media_type must be %q or %q", ErrInvalidInput, MediaMovie, MediaTV)
	case id < 1:
		return ref, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return ref, nil
}

// WatchlistItem is one title on a user's list ("My List"); Movie.MediaType
// says whether it is a movie or a series.
type WatchlistItem struct {
	Movie   Movie     `json:"movie"`
	AddedAt time.Time `json:"added_at"`
}

// UserRating is the score a user gave a movie.
type UserRating struct {
	Movie     Movie     `json:"movie"`
	Rating    int       `json:"rating"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LibraryStore persists each user's watchlist and ratings (PostgreSQL).
// Returned movies carry the summary fields only (what a card renders).
type LibraryStore interface {
	// AddToWatchlist is idempotent: re-adding keeps the original added_at.
	AddToWatchlist(ctx context.Context, userID string, ref TitleRef) (WatchlistItem, error)
	// RemoveFromWatchlist is idempotent: removing an absent title is not an error.
	RemoveFromWatchlist(ctx context.Context, userID string, ref TitleRef) error
	// GetUserWatchlist returns the list, most recently added first.
	GetUserWatchlist(ctx context.Context, userID string) ([]WatchlistItem, error)
	// UpsertUserRating creates the rating or replaces its score.
	UpsertUserRating(ctx context.Context, userID string, ref TitleRef, rating int) (UserRating, error)
	// GetUserRatings returns the ratings, most recently changed first.
	GetUserRatings(ctx context.Context, userID string) ([]UserRating, error)
}

// WatchedItem is one title a user has watched.
type WatchedItem struct {
	Movie     Movie     `json:"movie"`
	WatchedAt time.Time `json:"watched_at"`
}

// PublicWatched is what anyone may see of a user: their name and what they
// watched (their list and ratings stay private).
type PublicWatched struct {
	Username string        `json:"username"`
	Items    []WatchedItem `json:"items"`
}

// WatchedStore persists what each user has watched (PostgreSQL).
type WatchedStore interface {
	// MarkWatched is idempotent: marking again keeps the first watched_at.
	MarkWatched(ctx context.Context, userID string, ref TitleRef) (WatchedItem, error)
	// UnmarkWatched is idempotent.
	UnmarkWatched(ctx context.Context, userID string, ref TitleRef) error
	// GetUserWatched returns the titles, most recently watched first.
	GetUserWatched(ctx context.Context, userID string) ([]WatchedItem, error)
	// GetWatchedByUsername is the public view of a user (username matched
	// case-insensitively); ErrNotFound when there is no such user.
	GetWatchedByUsername(ctx context.Context, username string, limit int) (PublicWatched, error)
}
