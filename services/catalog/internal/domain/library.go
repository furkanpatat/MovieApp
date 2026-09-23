package domain

import (
	"context"
	"errors"
	"time"
)

const (
	MinRating = 1
	MaxRating = 10
)

var (
	// ErrMovieNotStored: the movie is not in the movies table yet, so a
	// library row cannot reference it. The service stores it and retries.
	ErrMovieNotStored = errors.New("movie not stored")
	// ErrUnknownUser: the authenticated user id has no account (deleted
	// after the token was issued).
	ErrUnknownUser = errors.New("unknown user")
)

// WatchlistItem is one movie on a user's list ("My List").
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
	AddToWatchlist(ctx context.Context, userID string, movieID int) (WatchlistItem, error)
	// RemoveFromWatchlist is idempotent: removing an absent movie is not an error.
	RemoveFromWatchlist(ctx context.Context, userID string, movieID int) error
	// GetUserWatchlist returns the list, most recently added first.
	GetUserWatchlist(ctx context.Context, userID string) ([]WatchlistItem, error)
	// UpsertUserRating creates the rating or replaces its score.
	UpsertUserRating(ctx context.Context, userID string, movieID, rating int) (UserRating, error)
	// GetUserRatings returns the ratings, most recently changed first.
	GetUserRatings(ctx context.Context, userID string) ([]UserRating, error)
}
