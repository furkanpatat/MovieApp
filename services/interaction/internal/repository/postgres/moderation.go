package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

var _ domain.ModerationStore = (*Repo)(nil)

func (r *Repo) ReportComment(ctx context.Context, commentID, reporterID string) error {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		r.q(`SELECT EXISTS (SELECT 1 FROM interaction.comments WHERE event_id = $1)`), commentID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.pool.Exec(ctx,
		r.q(`INSERT INTO interaction.comment_reports (comment_id, reporter_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`),
		commentID, reporterID)
	return err
}

func (r *Repo) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(ctx,
		r.q(`INSERT INTO interaction.user_blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`),
		blockerID, blockedID)
	return err
}

func (r *Repo) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(ctx,
		r.q(`DELETE FROM interaction.user_blocks WHERE blocker_id = $1 AND blocked_id = $2`), blockerID, blockedID)
	return err
}

func (r *Repo) BlockedUsers(ctx context.Context, blockerID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		r.q(`SELECT blocked_id FROM interaction.user_blocks WHERE blocker_id = $1 ORDER BY created_at, blocked_id`), blockerID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if errors.Is(err, pgx.ErrNoRows) {
		return []string{}, nil
	}
	if out == nil {
		out = []string{}
	}
	return out, err
}

func (r *Repo) ReportedComments(ctx context.Context, limit int) ([]domain.ReportedComment, error) {
	rows, err := r.pool.Query(ctx, r.q(`
		SELECT c.event_id::text, c.media_type, c.movie_id, c.user_id, c.body, c.occurred_at,
		       count(*), max(r.created_at)
		  FROM interaction.comment_reports r
		  JOIN interaction.comments c ON c.event_id = r.comment_id
		 GROUP BY c.event_id
		 ORDER BY count(*) DESC, max(r.created_at) DESC
		 LIMIT $1`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ReportedComment{}
	for rows.Next() {
		var c domain.ReportedComment
		if err := rows.Scan(&c.ID, &c.MediaType, &c.MovieID, &c.UserID, &c.Text, &c.CreatedAt, &c.Reports, &c.LastReportedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = domain.NormalizeTime(c.CreatedAt)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) DeleteComment(ctx context.Context, commentID string) (domain.Title, error) {
	var t domain.Title
	err := r.pool.QueryRow(ctx,
		r.q(`DELETE FROM interaction.comments WHERE event_id = $1 RETURNING media_type, movie_id`), commentID).Scan(&t.Media, &t.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, domain.ErrNotFound
	}
	return t, err
}

func (r *Repo) DismissReports(ctx context.Context, commentID string) error {
	_, err := r.pool.Exec(ctx, r.q(`DELETE FROM interaction.comment_reports WHERE comment_id = $1`), commentID)
	return err
}
