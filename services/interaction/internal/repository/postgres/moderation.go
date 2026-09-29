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
