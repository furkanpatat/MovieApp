// Package postgres is the durable Interaction store.
package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

type Repo struct {
	pool   *pgxpool.Pool
	schema string
}

var _ domain.Repository = (*Repo)(nil)

const defaultSchema = "interaction"

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool, schema: defaultSchema} }

// NewInSchema targets another schema with the same tables (used by tests to
// stay isolated from a running stack). schema must be a trusted identifier.
func NewInSchema(pool *pgxpool.Pool, schema string) *Repo { return &Repo{pool: pool, schema: schema} }

// q points a query at this repo's schema. Queries are written against "interaction.".
func (r *Repo) q(sql string) string { return strings.ReplaceAll(sql, defaultSchema+".", r.schema+".") }

// SaveRating serialises writers per movie by locking the movie's stats row,
// which keeps the aggregate exact and its version monotonic.
func (r *Repo) SaveRating(ctx context.Context, e domain.RatingSubmitted) (domain.RatingStats, error) {
	t := e.Title()
	stats := domain.RatingStats{Title: t}

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			r.q(`INSERT INTO interaction.movie_rating_stats (media_type, movie_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`),
			t.Media, t.ID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			r.q(`SELECT total_score, vote_count, version FROM interaction.movie_rating_stats
			  WHERE media_type = $1 AND movie_id = $2 FOR UPDATE`), t.Media, t.ID).
			Scan(&stats.TotalScore, &stats.VoteCount, &stats.Version); err != nil {
			return err
		}

		var oldScore int
		var oldAt time.Time
		err := tx.QueryRow(ctx,
			r.q(`SELECT score, occurred_at FROM interaction.ratings WHERE media_type = $1 AND movie_id = $2 AND user_id = $3`),
			t.Media, t.ID, e.UserID).Scan(&oldScore, &oldAt)

		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := tx.Exec(ctx,
				r.q(`INSERT INTO interaction.ratings (media_type, movie_id, user_id, score, event_id, occurred_at)
				 VALUES ($1, $2, $3, $4, $5, $6)`),
				t.Media, t.ID, e.UserID, e.Score, e.EventID, e.OccurredAt); err != nil {
				return err
			}
			stats.TotalScore += int64(e.Score)
			stats.VoteCount++
		case err != nil:
			return err
		case e.OccurredAt.Before(oldAt):
			return nil // a newer rating from this user was already applied
		case oldScore == e.Score:
			return nil // redelivery or no-op re-rate: nothing changes
		default:
			if _, err := tx.Exec(ctx,
				r.q(`UPDATE interaction.ratings SET score = $4, event_id = $5, occurred_at = $6
				  WHERE media_type = $1 AND movie_id = $2 AND user_id = $3`),
				t.Media, t.ID, e.UserID, e.Score, e.EventID, e.OccurredAt); err != nil {
				return err
			}
			stats.TotalScore += int64(e.Score - oldScore)
		}

		return tx.QueryRow(ctx,
			r.q(`UPDATE interaction.movie_rating_stats
			    SET total_score = $3, vote_count = $4, version = version + 1, updated_at = now()
			  WHERE media_type = $1 AND movie_id = $2 RETURNING version`),
			t.Media, t.ID, stats.TotalScore, stats.VoteCount).Scan(&stats.Version)
	})
	return stats, err
}

func (r *Repo) SaveComment(ctx context.Context, e domain.CommentAdded) error {
	_, err := r.pool.Exec(ctx,
		r.q(`INSERT INTO interaction.comments (event_id, media_type, movie_id, user_id, body, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (event_id) DO NOTHING`),
		e.EventID, e.Title().Media, e.MovieID, e.UserID, e.Text, e.OccurredAt)
	return err
}

func (r *Repo) GetStats(ctx context.Context, t domain.Title) (domain.RatingStats, error) {
	s := domain.RatingStats{Title: t}
	err := r.pool.QueryRow(ctx,
		r.q(`SELECT total_score, vote_count, version FROM interaction.movie_rating_stats WHERE media_type = $1 AND movie_id = $2`),
		t.Media, t.ID).Scan(&s.TotalScore, &s.VoteCount, &s.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	return s, err
}

func (r *Repo) RecentComments(ctx context.Context, t domain.Title, limit int) ([]domain.Comment, error) {
	rows, err := r.pool.Query(ctx,
		r.q(`SELECT event_id::text, user_id, body, occurred_at FROM interaction.comments
		  WHERE media_type = $1 AND movie_id = $2 ORDER BY occurred_at DESC, event_id DESC LIMIT $3`), t.Media, t.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Comment{}
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = domain.NormalizeTime(c.CreatedAt)
		out = append(out, c)
	}
	return out, rows.Err()
}

// PurgeUser removes everything a user wrote (account deletion): their
// ratings (taken off each title's aggregate), their comments and their
// events still waiting in the outbox (which would bring them back). It
// returns the titles whose read model is now stale. Stats rows are locked in
// title order, as SaveRating locks one at a time, so purges can't deadlock.
func (r *Repo) PurgeUser(ctx context.Context, userID string) ([]domain.RatingStats, error) {
	var touched []domain.RatingStats
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		touched = touched[:0]
		rows, err := tx.Query(ctx,
			r.q(`SELECT media_type, movie_id, score FROM interaction.ratings
			  WHERE user_id = $1 ORDER BY media_type, movie_id FOR UPDATE`), userID)
		if err != nil {
			return err
		}
		type rating struct {
			t     domain.Title
			score int
		}
		var ratings []rating
		for rows.Next() {
			var x rating
			if err := rows.Scan(&x.t.Media, &x.t.ID, &x.score); err != nil {
				rows.Close()
				return err
			}
			ratings = append(ratings, x)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		seen := map[domain.Title]bool{}
		for _, x := range ratings {
			s := domain.RatingStats{Title: x.t}
			err := tx.QueryRow(ctx,
				r.q(`UPDATE interaction.movie_rating_stats
				    SET total_score = total_score - $3, vote_count = vote_count - 1, version = version + 1, updated_at = now()
				  WHERE media_type = $1 AND movie_id = $2
				  RETURNING total_score, vote_count, version`),
				x.t.Media, x.t.ID, x.score).Scan(&s.TotalScore, &s.VoteCount, &s.Version)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			touched = append(touched, s)
			seen[x.t] = true
		}
		if _, err := tx.Exec(ctx, r.q(`DELETE FROM interaction.ratings WHERE user_id = $1`), userID); err != nil {
			return err
		}

		crows, err := tx.Query(ctx,
			r.q(`DELETE FROM interaction.comments WHERE user_id = $1 RETURNING media_type, movie_id`), userID)
		if err != nil {
			return err
		}
		for crows.Next() {
			var t domain.Title
			if err := crows.Scan(&t.Media, &t.ID); err != nil {
				crows.Close()
				return err
			}
			if !seen[t] {
				seen[t] = true
				touched = append(touched, domain.RatingStats{Title: t})
			}
		}
		crows.Close()
		if err := crows.Err(); err != nil {
			return err
		}

		// Their reports and blocks, and blocks of them, go too. (Reports on
		// their comments went with the comments: ON DELETE CASCADE.)
		if _, err := tx.Exec(ctx, r.q(`DELETE FROM interaction.comment_reports WHERE reporter_id = $1`), userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, r.q(`DELETE FROM interaction.user_blocks WHERE blocker_id = $1 OR blocked_id = $1`), userID); err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			r.q(`DELETE FROM interaction.outbox_events WHERE status = 'pending' AND payload ->> 'user_id' = $1`), userID)
		return err
	})
	return touched, err
}
