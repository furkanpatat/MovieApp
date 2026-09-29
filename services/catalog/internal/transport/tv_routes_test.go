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

// tvStub serves TV and discovery.
type tvStub struct{}

func (tvStub) GetPopularTV(_ context.Context, page int) (domain.MoviePage, error) {
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1399, MediaType: domain.MediaTV, Title: "GoT"}}}, nil
}

func (tvStub) GetTVDetails(_ context.Context, id int) (domain.Movie, error) {
	if id == 404 {
		return domain.Movie{}, domain.ErrNotFound
	}
	return domain.Movie{ID: id, MediaType: domain.MediaTV, Title: "GoT", TVDetails: domain.TVDetails{NumberOfSeasons: 8}}, nil
}

func (tvStub) DiscoverTV(_ context.Context, f domain.DiscoverFilter) (domain.MoviePage, error) {
	return domain.MoviePage{Page: f.Page, Results: []domain.Movie{{ID: f.GenreID, MediaType: domain.MediaTV}}}, nil
}

func (tvStub) SearchTV(_ context.Context, q string, page int) (domain.MoviePage, error) {
	return domain.MoviePage{Page: page, Results: []domain.Movie{{ID: 1, MediaType: domain.MediaTV, Title: q}}}, nil
}

func (tvStub) DiscoverMovies(_ context.Context, f domain.DiscoverFilter) (domain.MoviePage, error) {
	return domain.MoviePage{Page: f.Page, Results: []domain.Movie{{ID: f.GenreID, MediaType: domain.MediaMovie}}}, nil
}

func tvServer(t *testing.T) *httptest.Server {
	mr := miniredis.RunT(t)
	cache := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), time.Hour)
	svc := service.NewCatalog(stubProvider{}, cache, nil, time.Hour, quiet,
		service.WithTV(tvStub{}, nil), service.WithDiscovery(tvStub{}))
	srv := httptest.NewServer(transport.NewHandler(svc, nil, nil, quiet))
	t.Cleanup(srv.Close)
	return srv
}

func TestTVAndDiscoverRoutes(t *testing.T) {
	srv := tvServer(t)

	code, res := call(t, srv, "GET", "/api/v1/tv/1399", "", "")
	if code != http.StatusOK || res["media_type"] != "tv" || res["number_of_seasons"] != float64(8) {
		t.Fatalf("tv details -> %d %v", code, res)
	}
	if code, _ := call(t, srv, "GET", "/api/v1/tv/404", "", ""); code != http.StatusNotFound {
		t.Fatalf("missing show -> %d", code)
	}
	code, res = call(t, srv, "GET", "/api/v1/tv/popular?page=2", "", "")
	if code != http.StatusOK || res["page"] != float64(2) {
		t.Fatalf("popular tv -> %d %v", code, res)
	}
	code, res = call(t, srv, "GET", "/api/v1/discover/movies?genre=878&page=3", "", "")
	results, _ := res["results"].([]any)
	if code != http.StatusOK || res["page"] != float64(3) || len(results) != 1 || results[0].(map[string]any)["id"] != float64(878) {
		t.Fatalf("discover -> %d %v", code, res)
	}
	code, res = call(t, srv, "GET", "/api/v1/discover/tv?genre=10765&page=2", "", "")
	results, _ = res["results"].([]any)
	if code != http.StatusOK || len(results) != 1 || results[0].(map[string]any)["media_type"] != "tv" {
		t.Fatalf("discover tv -> %d %v", code, res)
	}
	code, res = call(t, srv, "GET", "/api/v1/search/tv?q=office", "", "")
	results, _ = res["results"].([]any)
	if code != http.StatusOK || len(results) != 1 || results[0].(map[string]any)["title"] != "office" {
		t.Fatalf("search tv -> %d %v", code, res)
	}
	for _, path := range []string{"/api/v1/search/tv?q=", "/api/v1/discover/tv?page=0", "/api/v1/tv/x", "/api/v1/tv/popular?page=0", "/api/v1/discover/movies?genre=abc", "/api/v1/discover/movies?page=501"} {
		if code, _ := call(t, srv, "GET", path, "", ""); code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", path, code)
		}
	}
}
