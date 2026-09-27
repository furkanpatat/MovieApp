package tmdb_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

const tvDetailsJSON = `{"id":1399,"name":"Game of Thrones","overview":"o","first_air_date":"2011-04-17",
 "last_air_date":"2019-05-19","status":"Ended","number_of_seasons":8,"number_of_episodes":73,
 "episode_run_time":[60],"genres":[{"id":18,"name":"Drama"}],"networks":[{"name":"HBO"}],
 "created_by":[{"name":"David Benioff"},{"name":"D. B. Weiss"}],"external_ids":{"imdb_id":"tt0944947"},
 "videos":{"results":[{"key":"clip","type":"Teaser","site":"YouTube"},{"key":"yt1","type":"Trailer","site":"YouTube"}]},
 "credits":{"cast":[{"id":22970,"name":"Peter Dinklage"}]}}`

func TestTVDetailsMapToATitle(t *testing.T) {
	var gotPath, gotAppend string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAppend = r.URL.Path, r.URL.Query().Get("append_to_response")
		ok(tvDetailsJSON)(w, r)
	})
	m, err := newClient(f, nil).GetTVDetails(context.Background(), 1399)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/tv/1399" || gotAppend != "videos,credits,external_ids" {
		t.Fatalf("request %s ?append_to_response=%s", gotPath, gotAppend)
	}
	if m.MediaType != domain.MediaTV || m.Title != "Game of Thrones" || m.ReleaseDate != "2011-04-17" || m.Runtime != 60 ||
		m.IMDbID != "tt0944947" || m.TrailerKey != "yt1" || m.CastJSON == "" {
		t.Fatalf("show %+v", m)
	}
	if m.NumberOfSeasons != 8 || m.NumberOfEpisodes != 73 || m.Status != "Ended" || m.LastAirDate != "2019-05-19" ||
		len(m.Networks) != 1 || m.Networks[0] != "HBO" || len(m.Creators) != 2 {
		t.Fatalf("tv details %+v", m.TVDetails)
	}
}

func TestPopularTV(t *testing.T) {
	var q url.Values
	var path string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		path, q = r.URL.Path, r.URL.Query()
		ok(`{"page":2,"total_pages":5,"results":[{"id":1,"name":"Show","first_air_date":"2020-01-01"}]}`)(w, r)
	})
	p, err := newClient(f, nil).GetPopularTV(context.Background(), 2)
	if path != "/discover/tv" || q.Get("page") != "2" || q.Get("without_genres") != "10767,10763,10764" {
		t.Fatalf("request %s %v", path, q)
	}
	if err != nil || p.Page != 2 || len(p.Results) != 1 || p.Results[0].Title != "Show" || p.Results[0].MediaType != domain.MediaTV {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestDiscoverMovies(t *testing.T) {
	var q url.Values
	var path string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		path, q = r.URL.Path, r.URL.Query()
		ok(popularJSON)(w, r)
	})
	c := newClient(f, nil)
	p, err := c.DiscoverMovies(context.Background(), domain.DiscoverFilter{GenreID: 878, Page: 7})
	if err != nil || len(p.Results) != 1 || p.Results[0].MediaType != domain.MediaMovie {
		t.Fatalf("%+v %v", p, err)
	}
	if path != "/discover/movie" || q.Get("with_genres") != "878" || q.Get("page") != "7" || q.Get("include_adult") != "false" ||
		q.Get("sort_by") != "popularity.desc" || q.Get("vote_count.gte") == "" {
		t.Fatalf("request %s %v", path, q)
	}
	if _, err := c.DiscoverMovies(context.Background(), domain.DiscoverFilter{Page: 1}); err != nil || q.Has("with_genres") {
		t.Fatalf("any genre: %v %v", q, err)
	}
}

func TestDiscoverAndSearchTV(t *testing.T) {
	var q url.Values
	var path string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		path, q = r.URL.Path, r.URL.Query()
		ok(`{"page":1,"total_pages":3,"results":[{"id":1399,"name":"Game of Thrones"}]}`)(w, r)
	})
	c := newClient(f, nil)
	p, err := c.DiscoverTV(context.Background(), domain.DiscoverFilter{GenreID: 10765, Page: 3})
	if err != nil || len(p.Results) != 1 || p.Results[0].MediaType != domain.MediaTV || p.Results[0].Title != "Game of Thrones" {
		t.Fatalf("%+v %v", p, err)
	}
	if path != "/discover/tv" || q.Get("with_genres") != "10765" || q.Get("page") != "3" || q.Get("without_genres") == "" {
		t.Fatalf("discover request %s %v", path, q)
	}
	if _, err := c.SearchTV(context.Background(), "thrones", 2); err != nil || path != "/search/tv" || q.Get("query") != "thrones" || q.Get("page") != "2" {
		t.Fatalf("search request %s %v %v", path, q, err)
	}
}
