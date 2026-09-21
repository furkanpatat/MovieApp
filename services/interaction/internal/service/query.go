package service

import (
	"context"
	"fmt"
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
func (q *Query) GetInteractions(ctx context.Context, movieID, limit int) (domain.Interactions, error) {
	if movieID < 1 {
		return domain.Interactions{}, fmt.Errorf("%w: movie id must be positive", domain.ErrInvalidInput)
	}
	if limit < 1 || limit > q.keep {
		limit = q.keep
	}

	got, found, err := q.rm.Get(ctx, movieID, limit)
	if err != nil {
		q.log.Warn("read model unavailable, falling back to postgres", "movie_id", movieID, "error", err)
	} else if found {
		return got, nil
	}

	// Miss (or Redis down): rebuild once per movie however many readers arrive.
	res, err, _ := q.group.Do(fmt.Sprint(movieID), func() (any, error) {
		lctx := context.WithoutCancel(ctx)
		stats, comments, err := loadSource(lctx, q.repo, movieID, q.keep)
		if err != nil {
			return nil, err
		}
		// Best effort: if Redis refuses the write we still serve Postgres' answer.
		if err := q.rm.Init(lctx, stats, comments); err != nil {
			q.log.Warn("could not materialise read model", "movie_id", movieID, "error", err)
		}
		return domain.Interactions{MovieID: movieID, AverageRating: stats.Average(), TotalVotes: stats.VoteCount, RecentComments: comments}, nil
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
