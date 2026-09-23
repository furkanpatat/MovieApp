package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// movieSummary is the movie part of every library row: the fields a card
// renders, from the movies table as m and imdb_ratings as r (LEFT JOIN).
const movieSummary = `m.id, m.title, m.overview, m.poster_path, m.backdrop_path,
	m.release_date, m.vote_average, m.vote_count, r.rating`

const joinIMDb = ` LEFT JOIN imdb_ratings r ON r.imdb_id = m.imdb_id`

// AddToWatchlist adds the movie to the user's list. Re-adding is a no-op that
// returns the existing row (the DO UPDATE is what makes RETURNING yield it).
func (r *Repository) AddToWatchlist(ctx context.Context, userID string, movieID int) (domain.WatchlistItem, error) {
	query := `
		WITH w AS (
			INSERT INTO library.watchlists (user_id, movie_id) VALUES ($1, $2)
			ON CONFLICT (user_id, movie_id) DO UPDATE SET created_at = library.watchlists.created_at
			RETURNING movie_id, created_at
		)
		SELECT ` + movieSummary + `, w.created_at
		FROM w JOIN movies m ON m.id = w.movie_id` + joinIMDb

	var it domain.WatchlistItem
	err := scanMovie(r.pool.QueryRow(ctx, query, userID, movieID), &it.Movie, &it.AddedAt)
	return it, mapLibraryError(err)
}

func (r *Repository) RemoveFromWatchlist(ctx context.Context, userID string, movieID int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM library.watchlists WHERE user_id = $1 AND movie_id = $2`, userID, movieID)
	return err
}

func (r *Repository) GetUserWatchlist(ctx context.Context, userID string) ([]domain.WatchlistItem, error) {
	query := `
		SELECT ` + movieSummary + `, w.created_at
		FROM library.watchlists w JOIN movies m ON m.id = w.movie_id` + joinIMDb + `
		WHERE w.user_id = $1
		ORDER BY w.created_at DESC, w.movie_id`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	items := []domain.WatchlistItem{}
	for rows.Next() {
		var it domain.WatchlistItem
		if err := scanMovie(rows, &it.Movie, &it.AddedAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// UpsertUserRating stores the user's score, replacing any earlier one.
// created_at keeps the first rating's time; updated_at moves on every change.
func (r *Repository) UpsertUserRating(ctx context.Context, userID string, movieID, rating int) (domain.UserRating, error) {
	query := `
		WITH ur AS (
			INSERT INTO library.user_ratings (user_id, movie_id, rating) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, movie_id) DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
			RETURNING movie_id, rating, created_at, updated_at
		)
		SELECT ` + movieSummary + `, ur.rating, ur.created_at, ur.updated_at
		FROM ur JOIN movies m ON m.id = ur.movie_id` + joinIMDb

	var ur domain.UserRating
	err := scanMovie(r.pool.QueryRow(ctx, query, userID, movieID, rating), &ur.Movie, &ur.Rating, &ur.CreatedAt, &ur.UpdatedAt)
	return ur, mapLibraryError(err)
}

func (r *Repository) GetUserRatings(ctx context.Context, userID string) ([]domain.UserRating, error) {
	query := `
		SELECT ` + movieSummary + `, ur.rating, ur.created_at, ur.updated_at
		FROM library.user_ratings ur JOIN movies m ON m.id = ur.movie_id` + joinIMDb + `
		WHERE ur.user_id = $1
		ORDER BY ur.updated_at DESC, ur.movie_id`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	items := []domain.UserRating{}
	for rows.Next() {
		var ur domain.UserRating
		if err := scanMovie(rows, &ur.Movie, &ur.Rating, &ur.CreatedAt, &ur.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, ur)
	}
	return items, rows.Err()
}

// scanMovie scans movieSummary into m, then the remaining columns into rest.
func scanMovie(row pgx.Row, m *domain.Movie, rest ...any) error {
	var poster, backdrop, release *string
	var voteAverage *float64
	var voteCount *int32
	var imdbRating *float32
	dst := append([]any{&m.ID, &m.Title, &m.Overview, &poster, &backdrop, &release, &voteAverage, &voteCount, &imdbRating}, rest...)
	if err := row.Scan(dst...); err != nil {
		return err
	}
	m.PosterPath, m.BackdropPath, m.ReleaseDate = deref(poster), deref(backdrop), deref(release)
	if voteAverage != nil {
		m.VoteAverage = *voteAverage
	}
	if voteCount != nil {
		m.VoteCount = int(*voteCount)
	}
	if imdbRating != nil {
		m.IMDbRating = roundRating(*imdbRating)
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// mapLibraryError turns the foreign-key violations of a library write into
// domain errors the service can act on.
func mapLibraryError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" { // foreign_key_violation
		return err
	}
	switch pgErr.ConstraintName {
	case "watchlists_movie_fk", "user_ratings_movie_fk":
		return fmt.Errorf("%w: %s", domain.ErrMovieNotStored, pgErr.Detail)
	case "watchlists_user_fk", "user_ratings_user_fk":
		return domain.ErrUnknownUser
	}
	return err
}
