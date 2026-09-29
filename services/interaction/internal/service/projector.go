package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// Projector is the worker side: persist to Postgres (source of truth), then
// update the Redis read model. Both steps are idempotent, so returning an error
// (and letting the broker redeliver, then dead-letter) is always safe.
type Projector struct {
	repo domain.Repository
	rm   domain.ReadModel
	keep int
	log  *slog.Logger
}

func NewProjector(repo domain.Repository, rm domain.ReadModel, keepComments int, log *slog.Logger) *Projector {
	if log == nil {
		log = slog.Default()
	}
	return &Projector{repo: repo, rm: rm, keep: keepComments, log: log}
}

func (p *Projector) HandleRating(ctx context.Context, e domain.RatingSubmitted) error {
	if err := e.Validate(); err != nil {
		return err
	}
	stats, err := p.repo.SaveRating(ctx, e)
	if err != nil {
		return fmt.Errorf("save rating: %w", err)
	}
	applied, err := p.rm.ApplyRating(ctx, stats)
	if err != nil {
		return fmt.Errorf("update read model: %w", err)
	}
	if !applied { // no model yet (or Redis was flushed): build the whole thing from Postgres
		return p.rebuild(ctx, e.Title())
	}
	return nil
}

func (p *Projector) HandleComment(ctx context.Context, e domain.CommentAdded) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := p.repo.SaveComment(ctx, e); err != nil {
		return fmt.Errorf("save comment: %w", err)
	}
	c := domain.Comment{ID: e.EventID, UserID: e.UserID, Text: e.Text, CreatedAt: domain.NormalizeTime(e.OccurredAt)}
	applied, err := p.rm.AddComment(ctx, e.Title(), c)
	if err != nil {
		return fmt.Errorf("update read model: %w", err)
	}
	if !applied {
		return p.rebuild(ctx, e.Title())
	}
	return nil
}

func (p *Projector) rebuild(ctx context.Context, t domain.Title) error {
	if _, _, err := buildFromSource(ctx, p.repo, p.rm, t, p.keep); err != nil {
		return fmt.Errorf("rebuild read model: %w", err)
	}
	return nil
}

// loadSource reads a movie's aggregate and recent comments from Postgres.
func loadSource(ctx context.Context, repo domain.Repository, t domain.Title, keep int) (domain.RatingStats, []domain.Comment, error) {
	stats, err := repo.GetStats(ctx, t)
	if err != nil {
		return stats, nil, err
	}
	comments, err := repo.RecentComments(ctx, t, keep)
	return stats, comments, err
}

// buildFromSource loads from Postgres and materialises in the read model.
func buildFromSource(ctx context.Context, repo domain.Repository, rm domain.ReadModel, t domain.Title, keep int) (domain.RatingStats, []domain.Comment, error) {
	stats, comments, err := loadSource(ctx, repo, t, keep)
	if err != nil {
		return stats, comments, err
	}
	return stats, comments, rm.Init(ctx, stats, comments)
}
