package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

type fakeTV struct {
	calls     atomic.Int32
	discovers []domain.DiscoverFilter
	mu        sync.Mutex
}

func (f *fakeTV) GetPopularTV(_ context.Context, page int) (domain.MoviePage, error) {
	f.calls.Add(1)
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1399, MediaType: domain.MediaTV, Title: "GoT"}}}, nil
}

func (f *fakeTV) GetTVDetails(_ context.Context, id int) (domain.Movie, error) {
	f.calls.Add(1)
	if id == 404 {
		return domain.Movie{}, domain.ErrNotFound
	}
	return domain.Movie{ID: id, MediaType: domain.MediaTV, Title: "GoT",
		TVDetails: domain.TVDetails{NumberOfSeasons: 8}}, nil
}

func (f *fakeTV) DiscoverTV(_ context.Context, fl domain.DiscoverFilter) (domain.MoviePage, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.discovers = append(f.discovers, fl)
	f.mu.Unlock()
	return domain.MoviePage{Page: fl.Page, Results: []domain.Movie{{ID: fl.GenreID*1000 + fl.Page, MediaType: domain.MediaTV}}}, nil
}

func (f *fakeTV) SearchTV(_ context.Context, q string, page int) (domain.MoviePage, error) {
	f.calls.Add(1)
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1, MediaType: domain.MediaTV, Title: q}}}, nil
}

func (f *fakeTV) DiscoverMovies(_ context.Context, fl domain.DiscoverFilter) (domain.MoviePage, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.discovers = append(f.discovers, fl)
	f.mu.Unlock()
	return domain.MoviePage{Page: fl.Page, Results: []domain.Movie{{ID: fl.GenreID*1000 + fl.Page}}}, nil
}

// fakeTVStore is an in-memory TVStore.
type fakeTVStore struct {
	mu    sync.Mutex
	shows map[int]domain.Movie
}

func (s *fakeTVStore) UpsertTV(_ context.Context, m domain.Movie) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shows == nil {
		s.shows = map[int]domain.Movie{}
	}
	s.shows[m.ID] = m
	return nil
}

func (s *fakeTVStore) GetTV(_ context.Context, id int) (domain.Movie, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.shows[id]
	if !ok {
		return domain.Movie{}, time.Time{}, domain.ErrNotFound
	}
	return m, time.Now(), nil
}

func TestTVDetailsAreLayeredLikeMovies(t *testing.T) {
	tv, store := &fakeTV{}, &fakeTVStore{}
	env := newSvcWith(t, &fakeProvider{}, service.WithTV(tv, store))
	ctx := context.Background()

	m, err := env.svc.GetTVDetails(ctx, 1399)
	if err != nil || m.MediaType != domain.MediaTV || m.NumberOfSeasons != 8 || tv.calls.Load() != 1 {
		t.Fatalf("%+v %v, %d calls", m, err, tv.calls.Load())
	}
	if _, err := env.svc.GetTVDetails(ctx, 1399); err != nil || tv.calls.Load() != 1 {
		t.Fatalf("L1 hit: %v, %d calls", err, tv.calls.Load())
	}
	env.redis.FlushAll()
	if m, err := env.svc.GetTVDetails(ctx, 1399); err != nil || m.Title != "GoT" || tv.calls.Load() != 1 {
		t.Fatalf("L2 hit: %+v %v, %d calls", m, err, tv.calls.Load())
	}
	// A TV id never reads the movie cache, and vice versa.
	if !env.redis.Exists("catalog:tv:1399") || env.redis.Exists("catalog:movie:1399") {
		t.Fatalf("keys %v", env.redis.Keys())
	}
	if _, err := env.svc.GetTVDetails(ctx, 404); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing show: %v", err)
	}
	if _, err := env.svc.GetTVDetails(ctx, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("id 0: %v", err)
	}
}

func TestPopularTVIsCached(t *testing.T) {
	tv := &fakeTV{}
	env := newSvcWith(t, &fakeProvider{}, service.WithTV(tv, nil))
	for range 2 {
		if p, err := env.svc.GetPopularTV(context.Background(), 1); err != nil || len(p.Results) != 1 {
			t.Fatalf("%+v %v", p, err)
		}
	}
	if tv.calls.Load() != 1 {
		t.Fatalf("%d provider calls", tv.calls.Load())
	}
}

func TestDiscoverIsCachedPerGenreAndPage(t *testing.T) {
	d := &fakeTV{}
	env := newSvcWith(t, &fakeProvider{}, service.WithDiscovery(d))
	ctx := context.Background()
	for _, c := range [][2]int{{878, 3}, {878, 3}, {878, 4}, {0, 3}} {
		p, err := env.svc.DiscoverMovies(ctx, c[0], c[1])
		if err != nil || p.Results[0].ID != c[0]*1000+c[1] {
			t.Fatalf("%v: %+v %v", c, p, err)
		}
	}
	if len(d.discovers) != 3 {
		t.Fatalf("provider saw %v", d.discovers)
	}
	for _, c := range [][2]int{{-1, 1}, {28, 0}, {28, 501}} {
		if _, err := env.svc.DiscoverMovies(ctx, c[0], c[1]); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%v: %v", c, err)
		}
	}
}

func TestDiscoverTVIsCachedApartFromMovies(t *testing.T) {
	tv := &fakeTV{}
	env := newSvcWith(t, &fakeProvider{}, service.WithTV(tv, nil), service.WithDiscovery(tv))
	ctx := context.Background()
	for range 2 {
		if p, err := env.svc.DiscoverTV(ctx, 10765, 2); err != nil || p.Results[0].MediaType != domain.MediaTV {
			t.Fatalf("%+v %v", p, err)
		}
	}
	// Same genre id and page for movies: its own cache entry, not the series.
	if p, err := env.svc.DiscoverMovies(ctx, 10765, 2); err != nil || p.Results[0].MediaType == domain.MediaTV {
		t.Fatalf("movies %+v %v", p, err)
	}
	if len(tv.discovers) != 2 || !env.redis.Exists("catalog:discover:tv:10765:2") || !env.redis.Exists("catalog:discover:movie:10765:2") {
		t.Fatalf("provider saw %v, keys %v", tv.discovers, env.redis.Keys())
	}
	if _, err := env.svc.DiscoverTV(ctx, -1, 1); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("bad genre: %v", err)
	}
}

func TestSearchTV(t *testing.T) {
	tv := &fakeTV{}
	env := newSvcWith(t, &fakeProvider{}, service.WithTV(tv, nil))
	ctx := context.Background()
	for _, q := range []string{"  The   Office ", "the office"} {
		if p, err := env.svc.SearchTV(ctx, q, 1); err != nil || p.Results[0].MediaType != domain.MediaTV {
			t.Fatalf("%q: %+v %v", q, p, err)
		}
	}
	if tv.calls.Load() != 1 {
		t.Fatalf("normalized queries should share a cache entry: %d calls", tv.calls.Load())
	}
	if _, err := env.svc.SearchTV(ctx, "  ", 1); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty query: %v", err)
	}
}

func TestTVAndDiscoveryAreOptional(t *testing.T) {
	env := newSvcWith(t, &fakeProvider{})
	ctx := context.Background()
	_, e1 := env.svc.GetTVDetails(ctx, 1)
	_, e2 := env.svc.GetPopularTV(ctx, 1)
	_, e3 := env.svc.DiscoverMovies(ctx, 0, 1)
	for _, err := range []error{e1, e2, e3} {
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("unconfigured: %v", err)
		}
	}
}
