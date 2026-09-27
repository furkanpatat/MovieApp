package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// titleSummary is the title part of every library row: the fields a card
// renders, from the movies (m) or tv_shows (t) row the library row (alias
// l) points at, and imdb_ratings as r. See joinTitle.
const titleSummary = `l.media_type, l.movie_id, COALESCE(m.title, t.name), COALESCE(m.overview, t.overview),
	COALESCE(m.poster_path, t.poster_path), COALESCE(m.backdrop_path, t.backdrop_path),
	COALESCE(m.release_date, t.first_air_date), COALESCE(m.vote_average, t.vote_average),
	COALESCE(m.vote_count, t.vote_count), r.rating`

// joinTitle joins library row l to its movie or series (the foreign keys on
// movie_ref / tv_ref guarantee exactly one of them exists) and its IMDb rating.
const joinTitle = ` LEFT JOIN movies m ON l.movie_ref = m.id
	LEFT JOIN tv_shows t ON l.tv_ref = t.id
	LEFT JOIN imdb_ratings r ON r.imdb_id = COALESCE(m.imdb_id, t.imdb_id)`

// AddToWatchlist adds the title to the user's list. Re-adding is a no-op that
// returns the existing row (the DO UPDATE is what makes RETURNING yield it).
func (r *Repository) AddToWatchlist(ctx context.Context, userID string, ref domain.TitleRef) (domain.WatchlistItem, error) {
	query := `
		WITH l AS (
			INSERT INTO library.watchlists (user_id, media_type, movie_id) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, media_type, movie_id) DO UPDATE SET created_at = library.watchlists.created_at
			RETURNING media_type, movie_id, movie_ref, tv_ref, created_at
		)
		SELECT ` + titleSummary + `, l.created_at
		FROM l` + joinTitle

	var it domain.WatchlistItem
	err := scanTitle(r.pool.QueryRow(ctx, query, userID, ref.MediaType, ref.ID), &it.Movie, &it.AddedAt)
	return it, mapLibraryError(err)
}

func (r *Repository) RemoveFromWatchlist(ctx context.Context, userID string, ref domain.TitleRef) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM library.watchlists WHERE user_id = $1 AND media_type = $2 AND movie_id = $3`,
		userID, ref.MediaType, ref.ID)
	return err
}

func (r *Repository) GetUserWatchlist(ctx context.Context, userID string) ([]domain.WatchlistItem, error) {
	query := `
		SELECT ` + titleSummary + `, l.created_at
		FROM library.watchlists l` + joinTitle + `
		WHERE l.user_id = $1
		ORDER BY l.created_at DESC, l.media_type, l.movie_id`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	items := []domain.WatchlistItem{}
	for rows.Next() {
		var it domain.WatchlistItem
		if err := scanTitle(rows, &it.Movie, &it.AddedAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// UpsertUserRating stores the user's score, replacing any earlier one.
// created_at keeps the first rating's time; updated_at moves on every change.
func (r *Repository) UpsertUserRating(ctx context.Context, userID string, ref domain.TitleRef, rating int) (domain.UserRating, error) {
	query := `
		WITH l AS (
			INSERT INTO library.user_ratings (user_id, media_type, movie_id, rating) VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, media_type, movie_id) DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
			RETURNING media_type, movie_id, movie_ref, tv_ref, rating, created_at, updated_at
		)
		SELECT ` + titleSummary + `, l.rating, l.created_at, l.updated_at
		FROM l` + joinTitle

	var ur domain.UserRating
	err := scanTitle(r.pool.QueryRow(ctx, query, userID, ref.MediaType, ref.ID, rating), &ur.Movie, &ur.Rating, &ur.CreatedAt, &ur.UpdatedAt)
	return ur, mapLibraryError(err)
}

func (r *Repository) GetUserRatings(ctx context.Context, userID string) ([]domain.UserRating, error) {
	query := `
		SELECT ` + titleSummary + `, l.rating, l.created_at, l.updated_at
		FROM library.user_ratings l` + joinTitle + `
		WHERE l.user_id = $1
		ORDER BY l.updated_at DESC, l.media_type, l.movie_id`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	items := []domain.UserRating{}
	for rows.Next() {
		var ur domain.UserRating
		if err := scanTitle(rows, &ur.Movie, &ur.Rating, &ur.CreatedAt, &ur.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, ur)
	}
	return items, rows.Err()
}

// scanTitle scans titleSummary into m, then the remaining columns into rest.
func scanTitle(row pgx.Row, m *domain.Movie, rest ...any) error {
	var poster, backdrop, release *string
	var voteAverage *float64
	var voteCount *int32
	var imdbRating *float32
	dst := append([]any{&m.MediaType, &m.ID, &m.Title, &m.Overview, &poster, &backdrop, &release, &voteAverage, &voteCount, &imdbRating}, rest...)
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
	case "watchlists_movie_fk", "user_ratings_movie_fk", "watchlists_tv_fk", "user_ratings_tv_fk":
		return fmt.Errorf("%w: %s", domain.ErrMovieNotStored, pgErr.Detail)
	case "watchlists_user_fk", "user_ratings_user_fk":
		return domain.ErrUnknownUser
	}
	return err
}
