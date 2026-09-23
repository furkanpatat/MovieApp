package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// MovieStorer makes sure a movie row exists, so library rows can reference it.
type MovieStorer interface {
	EnsureStored(ctx context.Context, id int) error
}

// Library is the per-user watchlist and ratings. The user id always comes
// from the authenticated request (the gateway's X-User-Id), never a body.
type Library struct {
	store  domain.LibraryStore
	movies MovieStorer
}

func NewLibrary(store domain.LibraryStore, movies MovieStorer) *Library {
	return &Library{store: store, movies: movies}
}

func (l *Library) AddToWatchlist(ctx context.Context, userID string, movieID int) (domain.WatchlistItem, error) {
	if err := validMovieID(movieID); err != nil {
		return domain.WatchlistItem{}, err
	}
	return withStoredMovie(ctx, l, movieID, func() (domain.WatchlistItem, error) {
		return l.store.AddToWatchlist(ctx, userID, movieID)
	})
}

func (l *Library) RemoveFromWatchlist(ctx context.Context, userID string, movieID int) error {
	if err := validMovieID(movieID); err != nil {
		return err
	}
	return l.store.RemoveFromWatchlist(ctx, userID, movieID)
}

func (l *Library) GetUserWatchlist(ctx context.Context, userID string) ([]domain.WatchlistItem, error) {
	return l.store.GetUserWatchlist(ctx, userID)
}

func (l *Library) RateMovie(ctx context.Context, userID string, movieID, rating int) (domain.UserRating, error) {
	if err := validMovieID(movieID); err != nil {
		return domain.UserRating{}, err
	}
	if rating < domain.MinRating || rating > domain.MaxRating {
		return domain.UserRating{}, fmt.Errorf("%w: rating must be between %d and %d", domain.ErrInvalidInput, domain.MinRating, domain.MaxRating)
	}
	return withStoredMovie(ctx, l, movieID, func() (domain.UserRating, error) {
		return l.store.UpsertUserRating(ctx, userID, movieID, rating)
	})
}

func (l *Library) GetUserRatings(ctx context.Context, userID string) ([]domain.UserRating, error) {
	return l.store.GetUserRatings(ctx, userID)
}

// withStoredMovie runs write; if the movie is not in the movies table yet
// (it was only ever seen in a list, never opened), stores it and retries once.
func withStoredMovie[T any](ctx context.Context, l *Library, movieID int, write func() (T, error)) (T, error) {
	v, err := write()
	if !errors.Is(err, domain.ErrMovieNotStored) {
		return v, err
	}
	if err := l.movies.EnsureStored(ctx, movieID); err != nil {
		var zero T
		return zero, err
	}
	return write()
}

func validMovieID(id int) error {
	if id < 1 {
		return fmt.Errorf("%w: movie id must be positive", domain.ErrInvalidInput)
	}
	return nil
}
