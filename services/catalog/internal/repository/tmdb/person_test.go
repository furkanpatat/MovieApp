package tmdb_test

import (
	"context"
	"net/http"
	"testing"
)

const personJSON = `{"id":31,"name":"Tom Hanks","biography":"b","profile_path":"/p.jpg","birthday":"1956-07-09",
 "known_for_department":"Acting","combined_credits":{
  "cast":[
   {"id":13,"media_type":"movie","title":"Forrest Gump","poster_path":"/fg.jpg","popularity":50,"character":"Forrest"},
   {"id":862,"media_type":"movie","title":"Toy Story","poster_path":"/ts.jpg","popularity":80,"character":"Woody (voice)"},
   {"id":99,"media_type":"tv","name":"Some Show","poster_path":"/tv.jpg","popularity":999},
   {"id":7,"media_type":"movie","title":"No Poster","popularity":90}
  ],
  "crew":[
   {"id":13,"media_type":"movie","title":"Forrest Gump","poster_path":"/fg.jpg","popularity":50,"job":"Producer"},
   {"id":500,"media_type":"movie","title":"That Thing You Do!","poster_path":"/t.jpg","popularity":10,"job":"Director"}
  ]}}`

func TestGetPerson(t *testing.T) {
	var path, appended string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		path, appended = r.URL.Path, r.URL.Query().Get("append_to_response")
		ok(personJSON)(w, r)
	})
	p, err := newClient(f, nil).GetPerson(context.Background(), 31)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/person/31" || appended != "combined_credits" {
		t.Fatalf("requested %s append=%q", path, appended)
	}
	if p.Name != "Tom Hanks" || p.Birthday != "1956-07-09" || p.ProfilePath != "/p.jpg" {
		t.Fatalf("person = %+v", p)
	}
	// Movies only, with posters, deduplicated, most popular first; the cast
	// role wins over a crew job for the same movie.
	want := []struct {
		id             int
		character, job string
	}{{862, "Woody (voice)", ""}, {13, "Forrest", ""}, {500, "", "Director"}}
	if len(p.Credits) != len(want) {
		t.Fatalf("credits = %+v", p.Credits)
	}
	for i, w := range want {
		c := p.Credits[i]
		if c.ID != w.id || c.Character != w.character || c.Job != w.job {
			t.Errorf("credit %d = %+v, want %+v", i, c, w)
		}
	}
}

func TestSearchMovies(t *testing.T) {
	var q, adult string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		q, adult = r.URL.Query().Get("query"), r.URL.Query().Get("include_adult")
		ok(popularJSON)(w, r)
	})
	p, err := newClient(f, nil).SearchMovies(context.Background(), "alpha & omega", 2)
	if err != nil || len(p.Results) != 1 || p.Results[0].Title != "Alpha" {
		t.Fatalf("got %+v, %v", p, err)
	}
	if q != "alpha & omega" || adult != "false" {
		t.Fatalf("query=%q include_adult=%q", q, adult)
	}
}
