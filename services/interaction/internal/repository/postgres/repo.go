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
	stats := domain.RatingStats{MovieID: e.MovieID}

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			r.q(`INSERT INTO interaction.movie_rating_stats (movie_id) VALUES ($1) ON CONFLICT DO NOTHING`),
			e.MovieID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			r.q(`SELECT total_score, vote_count, version FROM interaction.movie_rating_stats
			  WHERE movie_id = $1 FOR UPDATE`), e.MovieID).
			Scan(&stats.TotalScore, &stats.VoteCount, &stats.Version); err != nil {
			return err
		}

		var oldScore int
		var oldAt time.Time
		err := tx.QueryRow(ctx,
			r.q(`SELECT score, occurred_at FROM interaction.ratings WHERE movie_id = $1 AND user_id = $2`),
			e.MovieID, e.UserID).Scan(&oldScore, &oldAt)

		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := tx.Exec(ctx,
				r.q(`INSERT INTO interaction.ratings (movie_id, user_id, score, event_id, occurred_at)
				 VALUES ($1, $2, $3, $4, $5)`),
				e.MovieID, e.UserID, e.Score, e.EventID, e.OccurredAt); err != nil {
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
				r.q(`UPDATE interaction.ratings SET score = $3, event_id = $4, occurred_at = $5
				  WHERE movie_id = $1 AND user_id = $2`),
				e.MovieID, e.UserID, e.Score, e.EventID, e.OccurredAt); err != nil {
				return err
			}
			stats.TotalScore += int64(e.Score - oldScore)
		}

		return tx.QueryRow(ctx,
			r.q(`UPDATE interaction.movie_rating_stats
			    SET total_score = $2, vote_count = $3, version = version + 1, updated_at = now()
			  WHERE movie_id = $1 RETURNING version`),
			e.MovieID, stats.TotalScore, stats.VoteCount).Scan(&stats.Version)
	})
	return stats, err
}

func (r *Repo) SaveComment(ctx context.Context, e domain.CommentAdded) error {
	_, err := r.pool.Exec(ctx,
		r.q(`INSERT INTO interaction.comments (event_id, movie_id, user_id, body, occurred_at)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (event_id) DO NOTHING`),
		e.EventID, e.MovieID, e.UserID, e.Text, e.OccurredAt)
	return err
}

func (r *Repo) GetStats(ctx context.Context, movieID int) (domain.RatingStats, error) {
	s := domain.RatingStats{MovieID: movieID}
	err := r.pool.QueryRow(ctx,
		r.q(`SELECT total_score, vote_count, version FROM interaction.movie_rating_stats WHERE movie_id = $1`),
		movieID).Scan(&s.TotalScore, &s.VoteCount, &s.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	return s, err
}

func (r *Repo) RecentComments(ctx context.Context, movieID, limit int) ([]domain.Comment, error) {
	rows, err := r.pool.Query(ctx,
		r.q(`SELECT event_id::text, user_id, body, occurred_at FROM interaction.comments
		  WHERE movie_id = $1 ORDER BY occurred_at DESC, event_id DESC LIMIT $2`), movieID, limit)
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
