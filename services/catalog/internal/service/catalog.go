// Package service holds the Catalog use cases (cache-aside over TMDB).
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// maxPage is TMDB's hard limit on the page parameter.
const maxPage = 500

type Catalog struct {
	provider domain.MovieProvider
	cache    domain.Cache
	ttl      time.Duration
	log      *slog.Logger
	group    singleflight.Group
}

func NewCatalog(p domain.MovieProvider, c domain.Cache, ttl time.Duration, log *slog.Logger) *Catalog {
	if log == nil {
		log = slog.Default()
	}
	return &Catalog{provider: p, cache: c, ttl: ttl, log: log}
}

func (s *Catalog) GetPopularMovies(ctx context.Context, page int) (domain.MoviePage, error) {
	if page < 1 || page > maxPage {
		return domain.MoviePage{}, fmt.Errorf("%w: page must be 1..%d", domain.ErrInvalidInput, maxPage)
	}
	return cacheAside(ctx, s, popularKey(page), func(ctx context.Context) (domain.MoviePage, error) {
		return s.provider.GetPopularMovies(ctx, page)
	})
}

func (s *Catalog) GetMovieDetails(ctx context.Context, id int) (domain.Movie, error) {
	if id < 1 {
		return domain.Movie{}, fmt.Errorf("%w: id must be positive", domain.ErrInvalidInput)
	}
	return cacheAside(ctx, s, movieKey(id), func(ctx context.Context) (domain.Movie, error) {
		return s.provider.GetMovieDetails(ctx, id)
	})
}

// cacheAside: fresh cache -> provider (stored with TTL) -> stale cache when the
// provider fails. Concurrent misses for a key share one provider call.
// Cache errors never fail a request; they degrade to calling the provider.
func cacheAside[T any](ctx context.Context, s *Catalog, key string, load func(context.Context) (T, error)) (T, error) {
	var v T
	if hit, err := s.cache.Get(ctx, key, &v); err != nil {
		s.log.Warn("cache read failed", "key", key, "error", err)
	} else if hit {
		return v, nil
	}

	res, err, _ := s.group.Do(key, func() (any, error) {
		// Detach from the first caller's cancellation so it can't fail the others.
		lctx := context.WithoutCancel(ctx)

		val, err := load(lctx)
		if err == nil {
			if cerr := s.cache.Set(lctx, key, val, s.ttl); cerr != nil {
				s.log.Warn("cache write failed", "key", key, "error", cerr)
			}
			return val, nil
		}
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}

		var stale T
		if hit, serr := s.cache.GetStale(lctx, key, &stale); serr == nil && hit {
			s.log.Warn("provider failed, serving stale cache", "key", key, "error", err)
			return stale, nil
		}
		return nil, err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return res.(T), nil
}

func popularKey(page int) string { return fmt.Sprintf("catalog:popular:%d", page) }
func movieKey(id int) string     { return fmt.Sprintf("catalog:movie:%d", id) }
