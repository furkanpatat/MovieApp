package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

var _ domain.WatchedStore = (*Repository)(nil)

// MarkWatched records that the user watched the title. Marking again is a
// no-op that returns the existing row (DO UPDATE makes RETURNING yield it).
func (r *Repository) MarkWatched(ctx context.Context, userID string, ref domain.TitleRef) (domain.WatchedItem, error) {
	query := `
		WITH l AS (
			INSERT INTO library.watched (user_id, media_type, movie_id) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, media_type, movie_id) DO UPDATE SET watched_at = library.watched.watched_at
			RETURNING media_type, movie_id, movie_ref, tv_ref, watched_at
		)
		SELECT ` + titleSummary + `, l.watched_at
		FROM l` + joinTitle

	var it domain.WatchedItem
	err := scanTitle(r.pool.QueryRow(ctx, query, userID, ref.MediaType, ref.ID), &it.Movie, &it.WatchedAt)
	return it, mapLibraryError(err)
}

func (r *Repository) UnmarkWatched(ctx context.Context, userID string, ref domain.TitleRef) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM library.watched WHERE user_id = $1 AND media_type = $2 AND movie_id = $3`,
		userID, ref.MediaType, ref.ID)
	return err
}

func (r *Repository) GetUserWatched(ctx context.Context, userID string) ([]domain.WatchedItem, error) {
	return r.watchedOf(ctx, userID, 0)
}

// GetWatchedByUsername reads auth.users, the account table the library's
// foreign keys already point at, to find the user by name.
func (r *Repository) GetWatchedByUsername(ctx context.Context, username string, limit int) (domain.PublicWatched, error) {
	var out domain.PublicWatched
	var userID string
	err := r.pool.QueryRow(ctx, `SELECT id::text, username FROM auth.users WHERE lower(username) = lower($1)`, username).
		Scan(&userID, &out.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	out.Items, err = r.watchedOf(ctx, userID, limit)
	return out, err
}

// watchedOf is the user's watched titles, newest first (limit 0: all).
func (r *Repository) watchedOf(ctx context.Context, userID string, limit int) ([]domain.WatchedItem, error) {
	query := `
		SELECT ` + titleSummary + `, l.watched_at
		FROM library.watched l` + joinTitle + `
		WHERE l.user_id = $1
		ORDER BY l.watched_at DESC, l.media_type, l.movie_id
		LIMIT NULLIF($2, 0)`

	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	items := []domain.WatchedItem{}
	for rows.Next() {
		var it domain.WatchedItem
		if err := scanTitle(rows, &it.Movie, &it.WatchedAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
