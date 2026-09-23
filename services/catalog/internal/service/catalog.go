// Package service holds the Catalog use cases (cache-aside over TMDB).
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/singleflight"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// maxPage is TMDB's hard limit on the page parameter.
const maxPage = 500

type Catalog struct {
	provider domain.MovieProvider
	cache    domain.Cache
	store    domain.MovieStore  // L2 cache for movies (optional)
	people   domain.PersonStore // L2 cache for people (optional)
	ratings  domain.RatingProvider
	imdb     domain.IMDbStore
	imdbTTL  time.Duration
	ttl      time.Duration
	log      *slog.Logger
	group    singleflight.Group
}

// Option configures optional Catalog features.
type Option func(*Catalog)

// WithPeople stores people in an L2 cache (Postgres), as movies are.
func WithPeople(store domain.PersonStore) Option { return func(s *Catalog) { s.people = store } }

// WithIMDb attaches IMDb ratings to movies. Ratings are read from store and
// refreshed from provider (nil: stored ratings only) once older than ttl.
func WithIMDb(provider domain.RatingProvider, store domain.IMDbStore, ttl time.Duration) Option {
	return func(s *Catalog) { s.ratings, s.imdb, s.imdbTTL = provider, store, ttl }
}

func NewCatalog(p domain.MovieProvider, c domain.Cache, s domain.MovieStore, ttl time.Duration, log *slog.Logger, opts ...Option) *Catalog {
	if log == nil {
		log = slog.Default()
	}
	cat := &Catalog{provider: p, cache: c, store: s, ttl: ttl, log: log}
	for _, o := range opts {
		o(cat)
	}
	if cat.imdbTTL <= 0 {
		cat.imdbTTL = 72 * time.Hour
	}
	return cat
}

func (s *Catalog) GetPopularMovies(ctx context.Context, page int) (domain.MoviePage, error) {
	if page < 1 || page > maxPage {
		return domain.MoviePage{}, fmt.Errorf("%w: page must be 1..%d", domain.ErrInvalidInput, maxPage)
	}
	p, err := cacheAside(ctx, s, popularKey(page), func(ctx context.Context) (domain.MoviePage, error) {
		return s.provider.GetPopularMovies(ctx, page)
	})
	if err != nil {
		return p, err
	}
	p.Results = s.withIMDbRatings(ctx, p.Results)
	return p, nil
}

// maxQueryLen bounds search input (runes); TMDB titles are far shorter.
const maxQueryLen = 100

// SearchMovies searches TMDB by title. Results are cached like popular pages.
func (s *Catalog) SearchMovies(ctx context.Context, query string, page int) (domain.MoviePage, error) {
	q := strings.Join(strings.Fields(query), " ") // trim and collapse whitespace
	switch {
	case q == "":
		return domain.MoviePage{}, fmt.Errorf("%w: query is required", domain.ErrInvalidInput)
	case utf8.RuneCountInString(q) > maxQueryLen:
		return domain.MoviePage{}, fmt.Errorf("%w: query must be at most %d characters", domain.ErrInvalidInput, maxQueryLen)
	case page < 1 || page > maxPage:
		return domain.MoviePage{}, fmt.Errorf("%w: page must be 1..%d", domain.ErrInvalidInput, maxPage)
	}
	p, err := cacheAside(ctx, s, searchKey(q, page), func(ctx context.Context) (domain.MoviePage, error) {
		return s.provider.SearchMovies(ctx, q, page)
	})
	if err != nil {
		return p, err
	}
	p.Results = s.withIMDbRatings(ctx, p.Results)
	return p, nil
}

func (s *Catalog) GetMovieDetails(ctx context.Context, id int) (domain.Movie, error) {
	if id < 1 {
		return domain.Movie{}, fmt.Errorf("%w: id must be positive", domain.ErrInvalidInput)
	}
	var l2 *tier[domain.Movie]
	if s.store != nil {
		l2 = &tier[domain.Movie]{
			get: func(ctx context.Context) (domain.Movie, time.Time, error) {
				m, at, err := s.store.GetMovie(ctx, id)
				if err == nil {
					// A stored movie can outlive its IMDb data (or predate it).
					s.attachIMDb(ctx, &m)
				}
				return m, at, err
			},
			put: s.store.UpsertMovie,
		}
	}
	return layered(ctx, s, movieKey(id), l2, func(ctx context.Context) (domain.Movie, error) {
		m, err := s.provider.GetMovieDetails(ctx, id)
		if err == nil {
			s.attachIMDb(ctx, &m)
		}
		return m, err
	})
}

// GetPerson returns a person and their movies, cached exactly like movie
// details: Redis (L1), then Postgres (L2), then TMDB.
func (s *Catalog) GetPerson(ctx context.Context, id int) (domain.Person, error) {
	if id < 1 {
		return domain.Person{}, fmt.Errorf("%w: id must be positive", domain.ErrInvalidInput)
	}
	var l2 *tier[domain.Person]
	if s.people != nil {
		l2 = &tier[domain.Person]{
			get: func(ctx context.Context) (domain.Person, time.Time, error) { return s.people.GetPerson(ctx, id) },
			put: s.people.UpsertPerson,
		}
	}
	p, err := layered(ctx, s, personKey(id), l2, func(ctx context.Context) (domain.Person, error) {
		return s.provider.GetPerson(ctx, id)
	})
	if err != nil {
		return p, err
	}
	// Credits are movies: attach IMDb ratings the same way lists get them.
	movies := make([]domain.Movie, len(p.Credits))
	for i, c := range p.Credits {
		movies[i] = c.Movie
	}
	movies = s.withIMDbRatings(ctx, movies)
	credits := make([]domain.Credit, len(p.Credits)) // copy: p may be shared (singleflight)
	for i, c := range p.Credits {
		c.Movie = movies[i]
		credits[i] = c
	}
	p.Credits = credits
	return p, nil
}

// l2Fresh is how long an L2 (Postgres) row is served without asking TMDB.
const l2Fresh = 7 * 24 * time.Hour

// tier is an L2 store for one entity.
type tier[T any] struct {
	get func(context.Context) (T, time.Time, error) // value and when it was fetched; ErrNotFound if absent
	put func(context.Context, T) error
}

// layered reads through L1 (Redis) -> fresh L2 (Postgres, may be nil) ->
// the provider (TMDB), hydrating both caches on a provider hit. When the
// provider fails it serves the stale L1 copy, then any L2 row. Concurrent
// misses for a key share one lookup; cache errors never fail a request.
func layered[T any](ctx context.Context, s *Catalog, key string, l2 *tier[T], fetch func(context.Context) (T, error)) (T, error) {
	var v T
	if hit, err := s.cache.Get(ctx, key, &v); err != nil {
		s.log.Warn("L1 cache read failed", "key", key, "error", err)
	} else if hit {
		return v, nil
	}

	res, err, _ := s.group.Do(key, func() (any, error) {
		lctx := context.WithoutCancel(ctx)

		if l2 != nil {
			v, fetchedAt, err := l2.get(lctx)
			if err == nil && time.Since(fetchedAt) < l2Fresh {
				_ = s.cache.Set(lctx, key, v, s.ttl) // backfill L1
				return v, nil
			}
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				s.log.Warn("L2 store read failed", "key", key, "error", err)
			}
		}

		v, err := fetch(lctx)
		if err == nil {
			if l2 != nil {
				if serr := l2.put(lctx, v); serr != nil {
					s.log.Warn("L2 store upsert failed", "key", key, "error", serr)
				}
			}
			if cerr := s.cache.Set(lctx, key, v, s.ttl); cerr != nil {
				s.log.Warn("L1 cache write failed", "key", key, "error", cerr)
			}
			return v, nil
		}
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}

		var stale T
		if hit, serr := s.cache.GetStale(lctx, key, &stale); serr == nil && hit {
			s.log.Warn("provider failed, serving stale L1 cache", "key", key, "error", err)
			return stale, nil
		}
		if l2 != nil {
			if v, _, serr := l2.get(lctx); serr == nil {
				s.log.Warn("provider failed, serving stale L2 store", "key", key, "error", err)
				return v, nil
			}
		}
		return nil, err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return res.(T), nil
}

// attachIMDb sets m's IMDb rating and OMDb details: the stored ones while fresh, otherwise a
// new one from the rating provider (stored for next time). It never fails:
// without a rating the movie is simply shown with TMDB's score.
func (s *Catalog) attachIMDb(ctx context.Context, m *domain.Movie) {
	if m.IMDbID == "" || s.imdb == nil {
		return
	}
	stored, err := s.imdb.GetIMDbRating(ctx, m.IMDbID)
	have := err == nil
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		s.log.Warn("IMDb rating read failed", "imdb_id", m.IMDbID, "error", err)
	}
	if s.ratings != nil && (!have || time.Since(stored.FetchedAt) >= s.imdbTTL) {
		fresh, err := s.ratings.GetIMDbRating(ctx, m.IMDbID)
		switch {
		case err == nil, errors.Is(err, domain.ErrNotFound): // not found: remember "no rating"
			if serr := s.imdb.SaveIMDbRating(ctx, m.IMDbID, fresh); serr != nil {
				s.log.Warn("IMDb rating save failed", "imdb_id", m.IMDbID, "error", serr)
			}
			stored, have = fresh, true
		default:
			s.log.Debug("IMDb rating fetch failed, keeping stored one", "imdb_id", m.IMDbID, "error", err)
		}
	}
	if have {
		m.IMDbRating, m.IMDbVotes, m.OMDbDetails = stored.Rating, stored.Votes, stored.OMDbDetails
	}
}

// withIMDbRatings returns a copy of movies with their stored IMDb ratings.
// List pages never call the rating provider (a page is 20 movies); ratings
// appear once a movie's details have been fetched or warmed.
func (s *Catalog) withIMDbRatings(ctx context.Context, movies []domain.Movie) []domain.Movie {
	if s.imdb == nil || len(movies) == 0 {
		return movies
	}
	ids := make([]int, len(movies))
	for i, m := range movies {
		ids[i] = m.ID
	}
	known, err := s.imdb.IMDbRatingsForMovies(ctx, ids)
	if err != nil {
		s.log.Warn("IMDb ratings lookup failed", "error", err)
		return movies
	}
	out := make([]domain.Movie, len(movies)) // the input may be shared (singleflight)
	for i, m := range movies {
		if r, ok := known[m.ID]; ok {
			m.IMDbRating, m.IMDbVotes = r.Rating, r.Votes
		}
		out[i] = m
	}
	return out
}

// EnsureStored makes sure the movie has a row in the store (L2), which the
// user library references. A movie known only from a list page is fetched
// (L1 or TMDB) and stored. ErrNotFound means TMDB has no such movie.
func (s *Catalog) EnsureStored(ctx context.Context, id int) error {
	if s.store == nil {
		return errors.New("no movie store configured")
	}
	if _, _, err := s.store.GetMovie(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		return err // nil when already stored
	}
	m, err := s.GetMovieDetails(ctx, id)
	if err != nil {
		return err
	}
	// An L1 hit never touches L2, so store explicitly.
	return s.store.UpsertMovie(ctx, m)
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
func personKey(id int) string    { return fmt.Sprintf("catalog:person:%d", id) }

// searchKey is case-insensitive: "Dune" and "dune" share one entry.
func searchKey(q string, page int) string {
	return fmt.Sprintf("catalog:search:%d:%s", page, strings.ToLower(q))
}
