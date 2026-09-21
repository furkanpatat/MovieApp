package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// RefreshOptions controls a cache warm-up run.
type RefreshOptions struct {
	Pages       int           // popular pages to refresh (1..maxPage)
	WarmDetails bool          // also refresh details of every movie on those pages
	Pause       time.Duration // delay between TMDB calls, to stay under its rate limit
}

// RefreshResult summarises a run.
type RefreshResult struct {
	Pages   int // popular pages refreshed
	Details int // movie details refreshed
	Skipped int // movies TMDB no longer knows about
}

// RefreshPopular bypasses the fresh cache and rewrites popular pages (and
// optionally movie details) with new TMDB data, so user requests keep hitting
// warm entries. It stops at the first upstream outage (breaker open, 5xx...)
// rather than hammering a struggling TMDB; entries already in Redis stay
// available through the stale fallback.
func (s *Catalog) RefreshPopular(ctx context.Context, opt RefreshOptions) (RefreshResult, error) {
	var res RefreshResult
	if opt.Pages < 1 || opt.Pages > maxPage {
		return res, fmt.Errorf("%w: pages must be 1..%d", domain.ErrInvalidInput, maxPage)
	}

	seen := map[int]bool{}
	var ids []int
	for page := 1; page <= opt.Pages; page++ {
		if err := pause(ctx, opt.Pause, page > 1); err != nil {
			return res, err
		}
		p, err := s.provider.GetPopularMovies(ctx, page)
		if err != nil {
			return res, fmt.Errorf("refresh popular page %d: %w", page, err)
		}
		if err := s.cache.Set(ctx, popularKey(page), p, s.ttl); err != nil {
			return res, fmt.Errorf("cache popular page %d: %w", page, err)
		}
		res.Pages++
		for _, m := range p.Results {
			if !seen[m.ID] {
				seen[m.ID] = true
				ids = append(ids, m.ID)
			}
		}
	}

	if !opt.WarmDetails {
		return res, nil
	}
	for _, id := range ids {
		if err := pause(ctx, opt.Pause, true); err != nil {
			return res, err
		}
		m, err := s.provider.GetMovieDetails(ctx, id)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			res.Skipped++
			continue
		case err != nil:
			return res, fmt.Errorf("refresh movie %d: %w", id, err)
		}
		if err := s.cache.Set(ctx, movieKey(id), m, s.ttl); err != nil {
			return res, fmt.Errorf("cache movie %d: %w", id, err)
		}
		res.Details++
	}
	return res, nil
}

func pause(ctx context.Context, d time.Duration, wait bool) error {
	if !wait || d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
