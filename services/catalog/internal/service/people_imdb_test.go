package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/rediscache"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

type fakePeople struct {
	mu        sync.Mutex
	people    map[int]domain.Person
	fetchedAt map[int]time.Time
}

func newFakePeople() *fakePeople {
	return &fakePeople{people: map[int]domain.Person{}, fetchedAt: map[int]time.Time{}}
}
func (f *fakePeople) UpsertPerson(_ context.Context, p domain.Person) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.people[p.ID], f.fetchedAt[p.ID] = p, time.Now()
	return nil
}
func (f *fakePeople) GetPerson(_ context.Context, id int) (domain.Person, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.people[id]
	if !ok {
		return domain.Person{}, time.Time{}, domain.ErrNotFound
	}
	return p, f.fetchedAt[id], nil
}

func TestGetPersonLayers(t *testing.T) {
	ctx := context.Background()
	p, people := &fakeProvider{}, newFakePeople()
	mr := newSvcWith(t, p, service.WithPeople(people))

	got, err := mr.svc.GetPerson(ctx, 7)
	if err != nil || got.Name != "Ada" || len(got.Credits) != 1 || got.Credits[0].Character != "Hero" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, ok := people.people[7]; !ok {
		t.Fatal("TMDB result not stored in L2")
	}

	// L1 hit: no provider call.
	if _, err := mr.svc.GetPerson(ctx, 7); err != nil || p.calls.Load() != 1 {
		t.Fatalf("L1: %v, calls %d", err, p.calls.Load())
	}

	// L1 gone, fresh L2: still no provider call.
	mr.redis.FlushAll()
	if _, err := mr.svc.GetPerson(ctx, 7); err != nil || p.calls.Load() != 1 {
		t.Fatalf("L2: %v, calls %d", err, p.calls.Load())
	}

	// Stale L2 and TMDB down: the stale row is served.
	mr.redis.FlushAll()
	people.fetchedAt[7] = time.Now().Add(-30 * 24 * time.Hour)
	p.fail(domain.ErrUnavailable)
	if got, err := mr.svc.GetPerson(ctx, 7); err != nil || got.Name != "Ada" {
		t.Fatalf("stale L2 fallback: %+v, %v", got, err)
	}

	if _, err := mr.svc.GetPerson(ctx, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("id 0: %v", err)
	}
}

func TestSearchMovies(t *testing.T) {
	ctx := context.Background()
	p := &fakeProvider{}
	mr := newSvcWith(t, p)

	got, err := mr.svc.SearchMovies(ctx, "  the   matrix ", 1)
	if err != nil || got.Results[0].Title != "the matrix" {
		t.Fatalf("got %+v, %v (query should be trimmed and collapsed)", got, err)
	}
	// Case-insensitive cache hit.
	if _, err := mr.svc.SearchMovies(ctx, "The Matrix", 1); err != nil || p.calls.Load() != 1 {
		t.Fatalf("cache: %v, calls %d", err, p.calls.Load())
	}
	long := string(make([]rune, 101))
	for name, q := range map[string]string{"empty": "   ", "too long": long} {
		if _, err := mr.svc.SearchMovies(ctx, q, 1); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := mr.svc.SearchMovies(ctx, "x", 501); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("page 501: %v", err)
	}
}

// --- IMDb ---

type fakeIMDb struct {
	mu      sync.Mutex
	ratings map[string]domain.IMDbRating
	byMovie map[int]string // movie id -> imdb id
}

func newFakeIMDb() *fakeIMDb {
	return &fakeIMDb{ratings: map[string]domain.IMDbRating{}, byMovie: map[int]string{}}
}
func (f *fakeIMDb) GetIMDbRating(_ context.Context, id string) (domain.IMDbRating, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.ratings[id]
	if !ok {
		return domain.IMDbRating{}, domain.ErrNotFound
	}
	return r, nil
}
func (f *fakeIMDb) SaveIMDbRating(_ context.Context, id string, r domain.IMDbRating) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.FetchedAt = time.Now()
	f.ratings[id] = r
	return nil
}
func (f *fakeIMDb) IMDbRatingsForMovies(_ context.Context, ids []int) (map[int]domain.IMDbRating, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]domain.IMDbRating{}
	for _, id := range ids {
		if r, ok := f.ratings[f.byMovie[id]]; ok && r.Rating > 0 {
			out[id] = r
		}
	}
	return out, nil
}

type fakeOMDb struct {
	calls int
	r     domain.IMDbRating
	err   error
}

func (f *fakeOMDb) GetIMDbRating(context.Context, string) (domain.IMDbRating, error) {
	f.calls++
	return f.r, f.err
}

// imdbProvider returns details carrying an IMDb id.
type imdbProvider struct{ fakeProvider }

func (p *imdbProvider) GetMovieDetails(ctx context.Context, id int) (domain.Movie, error) {
	m, err := p.fakeProvider.GetMovieDetails(ctx, id)
	m.IMDbID = "tt0000001"
	return m, err
}

func TestIMDbRatingIsFetchedOnceAndReused(t *testing.T) {
	ctx := context.Background()
	store, omdb := newFakeIMDb(), &fakeOMDb{r: domain.IMDbRating{Rating: 8.1, Votes: 1000,
		OMDbDetails: domain.OMDbDetails{Rated: "R", RottenTomatoes: "91%", Metascore: 80}}}
	mr := newSvcWith(t, &imdbProvider{}, service.WithIMDb(omdb, store, time.Hour))

	m, err := mr.svc.GetMovieDetails(ctx, 5)
	if err != nil || m.IMDbRating != 8.1 || m.IMDbVotes != 1000 || m.Rated != "R" || m.RottenTomatoes != "91%" || m.Metascore != 80 {
		t.Fatalf("got %+v, %v", m, err)
	}
	mr.redis.FlushAll() // force another TMDB fetch
	if m, _ := mr.svc.GetMovieDetails(ctx, 5); m.IMDbRating != 8.1 || omdb.calls != 1 {
		t.Fatalf("rating %v, omdb calls %d (want the stored rating, one call)", m.IMDbRating, omdb.calls)
	}

	// Stale rating + OMDb down: the stored rating is still shown.
	mr.redis.FlushAll()
	store.ratings["tt0000001"] = domain.IMDbRating{Rating: 8.1, FetchedAt: time.Now().Add(-2 * time.Hour)}
	omdb.err = domain.ErrUnavailable
	if m, _ := mr.svc.GetMovieDetails(ctx, 5); m.IMDbRating != 8.1 || omdb.calls != 2 {
		t.Fatalf("rating %v, omdb calls %d", m.IMDbRating, omdb.calls)
	}
}

func TestWithoutIMDbKeyMoviesStillWork(t *testing.T) {
	store := newFakeIMDb()
	mr := newSvcWith(t, &imdbProvider{}, service.WithIMDb(nil, store, time.Hour))
	m, err := mr.svc.GetMovieDetails(context.Background(), 5)
	if err != nil || m.IMDbRating != 0 || m.IMDbID != "tt0000001" {
		t.Fatalf("got %+v, %v", m, err)
	}
}

func TestListsCarryStoredIMDbRatings(t *testing.T) {
	store := newFakeIMDb()
	store.byMovie[1] = "tt1"
	store.ratings["tt1"] = domain.IMDbRating{Rating: 7.4, Votes: 50}
	mr := newSvcWith(t, &fakeProvider{}, service.WithIMDb(nil, store, time.Hour))

	page, err := mr.svc.GetPopularMovies(context.Background(), 1)
	if err != nil || page.Results[0].IMDbRating != 7.4 {
		t.Fatalf("got %+v, %v", page, err)
	}
	// The cached page itself must not carry it (ratings are attached per read).
	if raw, _ := mr.redis.Get("catalog:popular:1"); raw == "" || strings.Contains(raw, "imdb_rating") {
		t.Fatalf("cached page: %s", raw)
	}
}

// imdbStore is a MovieStore whose movies carry an IMDb id.
type imdbMovieStore struct{ fakeStore }

func TestStoredMovieGetsIMDbDataWithoutTMDB(t *testing.T) {
	ctx := context.Background()
	movies := &imdbMovieStore{}
	_ = movies.UpsertMovie(ctx, domain.Movie{ID: 9, Title: "Stored", IMDbID: "tt9"})
	store, omdb := newFakeIMDb(), &fakeOMDb{r: domain.IMDbRating{Rating: 7.7, OMDbDetails: domain.OMDbDetails{Rated: "PG"}}}
	p := &fakeProvider{}
	mr := miniredis.RunT(t)
	cache := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), time.Hour)
	svc := service.NewCatalog(p, cache, movies, time.Hour, quiet, service.WithIMDb(omdb, store, time.Hour))

	m, err := svc.GetMovieDetails(ctx, 9)
	if err != nil || m.IMDbRating != 7.7 || m.Rated != "PG" {
		t.Fatalf("got %+v, %v", m, err)
	}
	if p.calls.Load() != 0 || omdb.calls != 1 {
		t.Fatalf("tmdb calls %d (want 0: fresh L2), omdb calls %d", p.calls.Load(), omdb.calls)
	}
}
