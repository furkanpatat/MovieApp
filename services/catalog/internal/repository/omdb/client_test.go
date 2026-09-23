package omdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

func server(t *testing.T, status int, body string) (*Client, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("apikey") != "k" || r.URL.Query().Get("i") == "" {
			t.Errorf("bad query %q", r.URL.RawQuery)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL + "/", APIKey: "k"}), &hits
}

func TestParsesRating(t *testing.T) {
	c, _ := server(t, 200, `{"Response":"True","imdbRating":"8.1","imdbVotes":"1,234,567"}`)
	r, err := c.GetIMDbRating(context.Background(), "tt0111161")
	if err != nil || r.Rating != 8.1 || r.Votes != 1234567 {
		t.Fatalf("got %+v, %v", r, err)
	}
}

// The sample response from the OMDb docs (Guardians of the Galaxy Vol. 2).
const fullJSON = `{"Title":"Guardians of the Galaxy Vol. 2","Rated":"PG-13","Director":"James Gunn",
 "Writer":"James Gunn, Dan Abnett, Andy Lanning","Language":"English","Country":"United States",
 "Awards":"Nominated for 1 Oscar. 15 wins & 62 nominations total",
 "Ratings":[{"Source":"Internet Movie Database","Value":"7.6/10"},{"Source":"Rotten Tomatoes","Value":"85%"},{"Source":"Metacritic","Value":"67/100"}],
 "Metascore":"67","imdbRating":"7.6","imdbVotes":"828,114","imdbID":"tt3896198","BoxOffice":"$389,813,101","Response":"True"}`

func TestParsesDetails(t *testing.T) {
	c, _ := server(t, 200, fullJSON)
	r, err := c.GetIMDbRating(context.Background(), "tt3896198")
	if err != nil {
		t.Fatal(err)
	}
	want := domain.OMDbDetails{
		Rated: "PG-13", RottenTomatoes: "85%", Metascore: 67,
		Awards: "Nominated for 1 Oscar. 15 wins & 62 nominations total", Director: "James Gunn",
		Writer: "James Gunn, Dan Abnett, Andy Lanning", BoxOffice: "$389,813,101",
		Country: "United States", Language: "English",
	}
	if r.Rating != 7.6 || r.Votes != 828114 || r.OMDbDetails != want {
		t.Fatalf("got %+v", r)
	}
}

func TestNoRatingYet(t *testing.T) {
	c, _ := server(t, 200, `{"Response":"True","imdbRating":"N/A","imdbVotes":"N/A","Metascore":"N/A","Rated":"N/A","Awards":"N/A","BoxOffice":"N/A"}`)
	r, err := c.GetIMDbRating(context.Background(), "tt9999999")
	if err != nil || r.Rating != 0 || r.Votes != 0 || r.OMDbDetails != (domain.OMDbDetails{}) {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestUnknownID(t *testing.T) {
	c, hits := server(t, 200, `{"Response":"False","Error":"Incorrect IMDb ID."}`)
	if _, err := c.GetIMDbRating(context.Background(), "tt0000000"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := c.GetIMDbRating(context.Background(), "nm123"); !errors.Is(err, domain.ErrNotFound) || hits.Load() != 1 {
		t.Fatalf("non-title id: %v, hits %d (want no request)", err, hits.Load())
	}
}

func TestLimitReachedPausesRequestsAndHidesKey(t *testing.T) {
	c, hits := server(t, 401, `{"Response":"False","Error":"Request limit reached!"}`)
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	_, err := c.GetIMDbRating(context.Background(), "tt1")
	if !errors.Is(err, domain.ErrUnavailable) || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("got %v", err)
	}
	if _, err := c.GetIMDbRating(context.Background(), "tt2"); !errors.Is(err, domain.ErrUnavailable) || hits.Load() != 1 {
		t.Fatalf("during cooldown: %v, hits %d (want 1)", err, hits.Load())
	}
	now = now.Add(11 * time.Minute)
	_, _ = c.GetIMDbRating(context.Background(), "tt3")
	if hits.Load() != 2 {
		t.Fatalf("after cooldown hits = %d, want 2", hits.Load())
	}
}

func TestNetworkErrorHidesKey(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:1/", APIKey: "secret-key"})
	_, err := c.GetIMDbRating(context.Background(), "tt1")
	if !errors.Is(err, domain.ErrUnavailable) || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("got %v", err)
	}
}
