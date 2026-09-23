package tmdb_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/tmdb"
)

const popularJSON = `{"page":1,"total_pages":10,"total_results":200,"results":[
 {"id":1,"title":"Alpha","overview":"a","poster_path":"/a.jpg","backdrop_path":"/a-bg.jpg","release_date":"2024-01-01","vote_average":7.5,"vote_count":100}]}`
const detailsJSON = `{"id":1,"title":"Alpha","runtime":120,"tagline":"t","genres":[{"id":18,"name":"Drama"}]}`

// fakeTMDB is a scriptable stand-in for the TMDB API.
type fakeTMDB struct {
	*httptest.Server
	hits    atomic.Int32
	handler atomic.Value // http.HandlerFunc
}

func newFake(t *testing.T, h http.HandlerFunc) *fakeTMDB {
	f := &fakeTMDB{}
	f.handler.Store(h)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.handler.Load().(http.HandlerFunc)(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func ok(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func newClient(f *fakeTMDB, mod func(*tmdb.Config)) *tmdb.Client {
	cfg := tmdb.Config{BaseURL: f.URL, APIKey: "k", Timeout: 200 * time.Millisecond, BreakerFailures: 3, BreakerOpenFor: 100 * time.Millisecond}
	if mod != nil {
		mod(&cfg)
	}
	return tmdb.New(cfg)
}

func TestSuccess200(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/movie/popular" {
			ok(popularJSON)(w, r)
			return
		}
		ok(detailsJSON)(w, r)
	})
	c := newClient(f, nil)

	page, err := c.GetPopularMovies(context.Background(), 1)
	if err != nil || len(page.Results) != 1 || page.Results[0].Title != "Alpha" || page.TotalPages != 10 ||
		page.Results[0].BackdropPath != "/a-bg.jpg" {
		t.Fatalf("popular: %+v %v", page, err)
	}
	m, err := c.GetMovieDetails(context.Background(), 1)
	if err != nil || m.Runtime != 120 || len(m.Genres) != 1 || m.Genres[0].Name != "Drama" {
		t.Fatalf("details: %+v %v", m, err)
	}
}

func TestBearerTokenAuth(t *testing.T) {
	var gotAuth, gotKey string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotKey = r.Header.Get("Authorization"), r.URL.Query().Get("api_key")
		ok(detailsJSON)(w, r)
	})
	c := newClient(f, func(c *tmdb.Config) { c.APIKey = "eyJtoken" })
	if _, err := c.GetMovieDetails(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer eyJtoken" || gotKey != "" {
		t.Fatalf("auth=%q api_key=%q", gotAuth, gotKey)
	}
}

func TestNotFoundDoesNotTripBreaker(t *testing.T) {
	f := newFake(t, status(http.StatusNotFound))
	c := newClient(f, nil)
	for i := 0; i < 10; i++ {
		if _, err := c.GetMovieDetails(context.Background(), 999); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("call %d: want ErrNotFound, got %v", i, err)
		}
	}
	if c.BreakerState() != gobreaker.StateClosed || f.hits.Load() != 10 {
		t.Fatalf("state=%v hits=%d", c.BreakerState(), f.hits.Load())
	}
}

func TestBreakerOpensOn500AndShortCircuits(t *testing.T) {
	f := newFake(t, status(http.StatusInternalServerError))
	c := newClient(f, nil) // opens after 3 consecutive failures

	for i := 0; i < 3; i++ {
		if _, err := c.GetMovieDetails(context.Background(), 1); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if c.BreakerState() != gobreaker.StateOpen {
		t.Fatalf("state = %v, want open", c.BreakerState())
	}

	before := f.hits.Load()
	for i := 0; i < 5; i++ {
		if _, err := c.GetMovieDetails(context.Background(), 1); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("open call: %v", err)
		}
	}
	if f.hits.Load() != before {
		t.Fatalf("open breaker still hit TMDB: %d -> %d", before, f.hits.Load())
	}
}

func TestBreakerRecoversThroughHalfOpen(t *testing.T) {
	f := newFake(t, status(http.StatusServiceUnavailable))
	c := newClient(f, nil)
	for i := 0; i < 3; i++ {
		_, _ = c.GetMovieDetails(context.Background(), 1)
	}
	if c.BreakerState() != gobreaker.StateOpen {
		t.Fatal("breaker should be open")
	}

	f.handler.Store(ok(detailsJSON))   // TMDB heals
	time.Sleep(150 * time.Millisecond) // > BreakerOpenFor

	if c.BreakerState() != gobreaker.StateHalfOpen {
		t.Fatalf("state = %v, want half-open", c.BreakerState())
	}
	if _, err := c.GetMovieDetails(context.Background(), 1); err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if c.BreakerState() != gobreaker.StateClosed {
		t.Fatalf("state = %v, want closed", c.BreakerState())
	}
}

func TestTimeoutCountsAsFailure(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	c := newClient(f, func(c *tmdb.Config) { c.Timeout = 50 * time.Millisecond })

	for i := 0; i < 3; i++ {
		if _, err := c.GetMovieDetails(context.Background(), 1); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("call %d: want ErrUnavailable, got %v", i, err)
		}
	}
	if c.BreakerState() != gobreaker.StateOpen {
		t.Fatalf("timeouts should open the breaker, state=%v", c.BreakerState())
	}
}

func TestMalformedJSONCountsAsFailure(t *testing.T) {
	f := newFake(t, ok(`{"id": <<<not json`))
	c := newClient(f, nil)
	for i := 0; i < 3; i++ {
		if _, err := c.GetMovieDetails(context.Background(), 1); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if c.BreakerState() != gobreaker.StateOpen {
		t.Fatalf("state = %v", c.BreakerState())
	}
}

func TestCallerCancellationDoesNotTripBreaker(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c := newClient(f, func(c *tmdb.Config) { c.Timeout = 5 * time.Second })
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		_, err := c.GetMovieDetails(ctx, 1)
		cancel()
		if err == nil {
			t.Fatal("expected error")
		}
	}
	if c.BreakerState() != gobreaker.StateClosed {
		t.Fatalf("state = %v, want closed", c.BreakerState())
	}
}

func TestErrorsDoNotLeakAPIKey(t *testing.T) {
	f := newFake(t, ok(detailsJSON))
	url := f.URL
	f.Close()
	c := tmdb.New(tmdb.Config{BaseURL: url, APIKey: "SECRET-KEY", Timeout: 100 * time.Millisecond})
	_, err := c.GetMovieDetails(context.Background(), 1)
	if err == nil || contains(err.Error(), "SECRET-KEY") {
		t.Fatalf("error leaks key or is nil: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
