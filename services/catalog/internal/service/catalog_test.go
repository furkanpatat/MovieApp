package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/rediscache"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/tmdb"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakeProvider struct {
	calls atomic.Int32
	delay time.Duration
	err   atomic.Value // error
}

func (f *fakeProvider) fail(err error) { f.err.Store(&err) }

func (f *fakeProvider) result() error {
	f.calls.Add(1)
	time.Sleep(f.delay)
	if p, _ := f.err.Load().(*error); p != nil && *p != nil {
		return *p
	}
	return nil
}
func (f *fakeProvider) GetPopularMovies(_ context.Context, page int) (domain.MoviePage, error) {
	if err := f.result(); err != nil {
		return domain.MoviePage{}, err
	}
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1, Title: "Alpha"}}}, nil
}
func (f *fakeProvider) GetMovieDetails(_ context.Context, id int) (domain.Movie, error) {
	if err := f.result(); err != nil {
		return domain.Movie{}, err
	}
	return domain.Movie{ID: id, Title: "Alpha"}, nil
}

func (f *fakeProvider) SearchMovies(_ context.Context, q string, page int) (domain.MoviePage, error) {
	if err := f.result(); err != nil {
		return domain.MoviePage{}, err
	}
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1, Title: q}}}, nil
}
func (f *fakeProvider) GetPerson(_ context.Context, id int) (domain.Person, error) {
	if err := f.result(); err != nil {
		return domain.Person{}, err
	}
	return domain.Person{ID: id, Name: "Ada", Credits: []domain.Credit{{Movie: domain.Movie{ID: 1, Title: "Alpha"}, Character: "Hero"}}}, nil
}

type fakeStore struct {
	movies    map[int]domain.Movie
	fetchedAt map[int]time.Time
}

func (f *fakeStore) UpsertMovie(ctx context.Context, m domain.Movie) error {
	if f.movies == nil {
		f.movies = make(map[int]domain.Movie)
		f.fetchedAt = make(map[int]time.Time)
	}
	f.movies[m.ID] = m
	f.fetchedAt[m.ID] = time.Now()
	return nil
}

func (f *fakeStore) GetMovie(ctx context.Context, id int) (domain.Movie, time.Time, error) {
	if m, ok := f.movies[id]; ok {
		return m, f.fetchedAt[id], nil
	}
	return domain.Movie{}, time.Time{}, domain.ErrNotFound
}

type svcEnv struct {
	svc   *service.Catalog
	redis *miniredis.Miniredis
}

func newSvcWith(t *testing.T, p domain.MovieProvider, opts ...service.Option) svcEnv {
	mr := miniredis.RunT(t)
	cache := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 24*time.Hour)
	return svcEnv{service.NewCatalog(p, cache, nil, time.Hour, quiet, opts...), mr}
}

func newSvc(t *testing.T, p domain.MovieProvider, s domain.MovieStore) (*service.Catalog, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)
	cache := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 24*time.Hour)
	return service.NewCatalog(p, cache, s, time.Hour, quiet), mr
}

func TestL2CacheFallback(t *testing.T) {
	p := &fakeProvider{}
	store := &fakeStore{}
	svc, mr := newSvc(t, p, store)
	ctx := context.Background()

	// 1. First fetch (Miss L1, Miss L2, Hit TMDB)
	_, _ = svc.GetMovieDetails(ctx, 1)
	if p.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", p.calls.Load())
	}

	// 2. Clear L1 (Redis)
	mr.FlushAll()

	// 3. Second fetch (Miss L1, Hit L2)
	m, err := svc.GetMovieDetails(ctx, 1)
	if err != nil || m.Title != "Alpha" {
		t.Fatalf("L2 hit failed: %+v %v", m, err)
	}
	// Provider should not be called again
	if p.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (should be L2 hit)", p.calls.Load())
	}
}

func TestMissThenHit(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		m, err := svc.GetMovieDetails(ctx, 1)
		if err != nil || m.Title != "Alpha" {
			t.Fatalf("call %d: %+v %v", i, m, err)
		}
	}
	if p.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (miss then hits)", p.calls.Load())
	}
	if ttl := mr.TTL("catalog:movie:1"); ttl != time.Hour {
		t.Fatalf("cache ttl = %v, want 1h", ttl)
	}

	mr.FastForward(time.Hour + time.Second) // TTL expiry -> miss again
	if _, err := svc.GetMovieDetails(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 2 {
		t.Fatalf("provider calls after expiry = %d, want 2", p.calls.Load())
	}
}

func TestPopularCachedPerPage(t *testing.T) {
	p := &fakeProvider{}
	svc, _ := newSvc(t, p, nil)
	ctx := context.Background()
	_, _ = svc.GetPopularMovies(ctx, 1)
	_, _ = svc.GetPopularMovies(ctx, 1)
	_, _ = svc.GetPopularMovies(ctx, 2)
	if p.calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", p.calls.Load())
	}
}

func TestServesStaleWhenProviderFails(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	ctx := context.Background()
	if _, err := svc.GetMovieDetails(ctx, 1); err != nil {
		t.Fatal(err)
	}

	mr.FastForward(2 * time.Hour) // fresh gone, stale (24h) remains
	p.fail(domain.ErrUnavailable)

	m, err := svc.GetMovieDetails(ctx, 1)
	if err != nil || m.Title != "Alpha" {
		t.Fatalf("expected stale fallback, got %+v %v", m, err)
	}
	// Never-cached key has no fallback.
	if _, err := svc.GetMovieDetails(ctx, 2); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestNotFoundIsNotMaskedByStale(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	_, _ = svc.GetMovieDetails(context.Background(), 1)
	mr.FastForward(2 * time.Hour)
	p.fail(domain.ErrNotFound)
	if _, err := svc.GetMovieDetails(context.Background(), 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestInvalidInput(t *testing.T) {
	svc, _ := newSvc(t, &fakeProvider{}, nil)
	for _, err := range []error{
		second(svc.GetPopularMovies(context.Background(), 0)),
		second(svc.GetPopularMovies(context.Background(), 501)),
		second(svc.GetMovieDetails(context.Background(), 0)),
	} {
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("want ErrInvalidInput, got %v", err)
		}
	}
}

func second[T any](_ T, err error) error { return err }

func TestRedisDownFallsThroughToProvider(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	mr.Close() // Redis is gone
	m, err := svc.GetMovieDetails(context.Background(), 1)
	if err != nil || m.Title != "Alpha" {
		t.Fatalf("cache outage must not fail requests: %+v %v", m, err)
	}
}

func TestConcurrentMissesShareOneProviderCall(t *testing.T) {
	p := &fakeProvider{delay: 100 * time.Millisecond}
	svc, _ := newSvc(t, p, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.GetMovieDetails(context.Background(), 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if p.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1 (singleflight)", p.calls.Load())
	}
}

// End to end: real TMDB client against a fake TMDB, real cache on miniredis.
// TMDB goes down; the breaker opens; cached data keeps being served and the
// dead upstream stops receiving traffic.
func TestTMDBOutageServedFromCacheBehindOpenBreaker(t *testing.T) {
	var down atomic.Bool
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"title":"Alpha"}`))
	}))
	defer srv.Close()

	client := tmdb.New(tmdb.Config{BaseURL: srv.URL, APIKey: "k", BreakerFailures: 3, BreakerOpenFor: time.Minute})
	svc, mr := newSvc(t, client, nil)
	ctx := context.Background()

	if _, err := svc.GetMovieDetails(ctx, 1); err != nil { // warm the cache
		t.Fatal(err)
	}
	mr.FastForward(2 * time.Hour) // fresh entry expires
	down.Store(true)

	// Uncached ids fail and trip the breaker.
	for id := 100; id < 103; id++ {
		if _, err := svc.GetMovieDetails(ctx, id); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("id %d: %v", id, err)
		}
	}
	before := hits.Load()

	// Cached movie still served (stale), and TMDB is no longer hit.
	for i := 0; i < 5; i++ {
		m, err := svc.GetMovieDetails(ctx, 1)
		if err != nil || m.Title != "Alpha" {
			t.Fatalf("stale serve %d: %+v %v", i, m, err)
		}
	}
	if hits.Load() != before {
		t.Fatalf("open breaker leaked %d requests to TMDB", hits.Load()-before)
	}
}
