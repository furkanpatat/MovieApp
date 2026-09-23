package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/rediscache"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
	"github.com/furkanpatat/movieapp/services/catalog/internal/transport"
)

type stubProvider struct{}

func (stubProvider) GetPopularMovies(context.Context, int) (domain.MoviePage, error) {
	return domain.MoviePage{}, nil
}
func (stubProvider) GetMovieDetails(context.Context, int) (domain.Movie, error) {
	return domain.Movie{}, domain.ErrNotFound
}
func (stubProvider) SearchMovies(_ context.Context, q string, page int) (domain.MoviePage, error) {
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1, Title: q}}}, nil
}
func (stubProvider) GetPerson(_ context.Context, id int) (domain.Person, error) {
	if id == 404 {
		return domain.Person{}, domain.ErrNotFound
	}
	return domain.Person{ID: id, Name: "Ada", Credits: []domain.Credit{{Movie: domain.Movie{ID: 3, Title: "M"}, Character: "Lead"}}}, nil
}

func catalogServer(t *testing.T) *httptest.Server {
	mr := miniredis.RunT(t)
	cache := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), time.Hour)
	svc := service.NewCatalog(stubProvider{}, cache, nil, time.Hour, quiet)
	srv := httptest.NewServer(transport.NewHandler(svc, nil, nil, quiet))
	t.Cleanup(srv.Close)
	return srv
}

func TestPersonRoute(t *testing.T) {
	srv := catalogServer(t)
	code, body := call(t, srv, "GET", "/api/v1/people/31", "", "")
	credits, _ := body["credits"].([]any)
	if code != http.StatusOK || body["name"] != "Ada" || len(credits) != 1 {
		t.Fatalf("-> %d %v", code, body)
	}
	// A credit is a movie with the role inlined.
	if c := credits[0].(map[string]any); c["id"] != float64(3) || c["title"] != "M" || c["character"] != "Lead" {
		t.Fatalf("credit = %v", c)
	}
	if code, body := call(t, srv, "GET", "/api/v1/people/404", "", ""); code != http.StatusNotFound || body["error"] != "person not found" {
		t.Fatalf("unknown -> %d %v", code, body)
	}
	if code, _ := call(t, srv, "GET", "/api/v1/people/abc", "", ""); code != http.StatusBadRequest {
		t.Fatalf("bad id -> %d", code)
	}
}

func TestSearchRoute(t *testing.T) {
	srv := catalogServer(t)
	code, body := call(t, srv, "GET", "/api/v1/search/movies?q=dune&page=2", "", "")
	if code != http.StatusOK || body["page"] != float64(2) {
		t.Fatalf("-> %d %v", code, body)
	}
	for _, path := range []string{"/api/v1/search/movies", "/api/v1/search/movies?q=%20", "/api/v1/search/movies?q=x&page=zero"} {
		if code, _ := call(t, srv, "GET", path, "", ""); code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", path, code)
		}
	}
}
