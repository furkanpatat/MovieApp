package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// MovieStorer makes sure a movie row exists, so library rows can reference it.
// TitleStorer makes sure a title has its row (movies or tv_shows), which
// library rows reference.
type TitleStorer interface {
	EnsureTitleStored(ctx context.Context, ref domain.TitleRef) error
}

// Library is each user's watchlist and ratings, of movies and series.
type Library struct {
	store  domain.LibraryStore
	titles TitleStorer
}

func NewLibrary(store domain.LibraryStore, titles TitleStorer) *Library {
	return &Library{store: store, titles: titles}
}

func (l *Library) AddToWatchlist(ctx context.Context, userID string, ref domain.TitleRef) (domain.WatchlistItem, error) {
	if _, err := domain.ParseTitleRef(ref.MediaType, ref.ID); err != nil {
		return domain.WatchlistItem{}, err
	}
	return withStoredTitle(ctx, l, ref, func() (domain.WatchlistItem, error) {
		return l.store.AddToWatchlist(ctx, userID, ref)
	})
}

func (l *Library) RemoveFromWatchlist(ctx context.Context, userID string, ref domain.TitleRef) error {
	if _, err := domain.ParseTitleRef(ref.MediaType, ref.ID); err != nil {
		return err
	}
	return l.store.RemoveFromWatchlist(ctx, userID, ref)
}

func (l *Library) GetUserWatchlist(ctx context.Context, userID string) ([]domain.WatchlistItem, error) {
	return l.store.GetUserWatchlist(ctx, userID)
}

// Rate stores the user's score for a movie or a series.
func (l *Library) Rate(ctx context.Context, userID string, ref domain.TitleRef, rating int) (domain.UserRating, error) {
	if _, err := domain.ParseTitleRef(ref.MediaType, ref.ID); err != nil {
		return domain.UserRating{}, err
	}
	if rating < domain.MinRating || rating > domain.MaxRating {
		return domain.UserRating{}, fmt.Errorf("%w: rating must be between %d and %d", domain.ErrInvalidInput, domain.MinRating, domain.MaxRating)
	}
	return withStoredTitle(ctx, l, ref, func() (domain.UserRating, error) {
		return l.store.UpsertUserRating(ctx, userID, ref, rating)
	})
}

func (l *Library) GetUserRatings(ctx context.Context, userID string) ([]domain.UserRating, error) {
	return l.store.GetUserRatings(ctx, userID)
}

// withStoredTitle runs a library write; when the title has no row yet
// (known only from a list page), it stores it and tries once more.
func withStoredTitle[T any](ctx context.Context, l *Library, ref domain.TitleRef, write func() (T, error)) (T, error) {
	v, err := write()
	if !errors.Is(err, domain.ErrMovieNotStored) {
		return v, err
	}
	if err := l.titles.EnsureTitleStored(ctx, ref); err != nil {
		var zero T
		return zero, err
	}
	return write()
}
