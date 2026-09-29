package service

import (
	"context"
	"log/slog"

	"golang.org/x/sync/singleflight"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// Query is the read side. Hot path: one Redis round trip. Postgres is only
// consulted when the read model is missing (cold start, eviction, flush).
type Query struct {
	repo  domain.Repository
	rm    domain.ReadModel
	keep  int
	log   *slog.Logger
	group singleflight.Group
}

func NewQuery(repo domain.Repository, rm domain.ReadModel, keepComments int, log *slog.Logger) *Query {
	if log == nil {
		log = slog.Default()
	}
	return &Query{repo: repo, rm: rm, keep: keepComments, log: log}
}

// GetInteractions returns the read model for a movie. limit <= 0 means "all retained".
func (q *Query) GetInteractions(ctx context.Context, t domain.Title, limit int) (domain.Interactions, error) {
	if err := t.Validate(); err != nil {
		return domain.Interactions{}, err
	}
	if limit < 1 || limit > q.keep {
		limit = q.keep
	}

	got, found, err := q.rm.Get(ctx, t, limit)
	if err != nil {
		q.log.Warn("read model unavailable, falling back to postgres", "title", t.String(), "error", err)
	} else if found {
		return got, nil
	}

	// Miss (or Redis down): rebuild once per movie however many readers arrive.
	res, err, _ := q.group.Do(t.String(), func() (any, error) {
		lctx := context.WithoutCancel(ctx)
		stats, comments, err := loadSource(lctx, q.repo, t, q.keep)
		if err != nil {
			return nil, err
		}
		// Best effort: if Redis refuses the write we still serve Postgres' answer.
		if err := q.rm.Init(lctx, stats, comments); err != nil {
			q.log.Warn("could not materialise read model", "title", t.String(), "error", err)
		}
		return domain.Interactions{MediaType: t.Media, MovieID: t.ID, AverageRating: stats.Average(), TotalVotes: stats.VoteCount, RecentComments: comments}, nil
	})
	if err != nil {
		return domain.Interactions{}, err
	}
	out := res.(domain.Interactions)
	if len(out.RecentComments) > limit {
		out.RecentComments = out.RecentComments[:limit]
	}
	return out, nil
}
