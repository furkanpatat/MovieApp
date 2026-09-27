package service_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

// pickCatalog is a tiny catalog: "Heat" is a crime movie (genre 80), every
// genre's feed is 7 movies with posters (ids genre*100+i), and trending is
// ids 1..7.
type pickCatalog struct{}

func feed(base int) []domain.Movie {
	var out []domain.Movie
	for i := 1; i <= 7; i++ {
		out = append(out, domain.Movie{ID: base + i, Title: "Movie", PosterPath: "/p.jpg", ReleaseDate: "2020-01-01"})
	}
	return out
}

func (pickCatalog) GetPopularMovies(context.Context, int) (domain.MoviePage, error) {
	return domain.MoviePage{Results: feed(0)}, nil
}
func (pickCatalog) GetMovieDetails(_ context.Context, id int) (domain.Movie, error) {
	if id == 949 {
		return domain.Movie{ID: 949, Title: "Heat", PosterPath: "/h.jpg", Genres: []domain.Genre{{ID: 80, Name: "Crime"}}}, nil
	}
	return domain.Movie{ID: id, Title: "Movie", PosterPath: "/p.jpg"}, nil
}
func (pickCatalog) SearchMovies(_ context.Context, q string, _ int) (domain.MoviePage, error) {
	if strings.EqualFold(q, "heat") {
		return domain.MoviePage{Results: []domain.Movie{{ID: 949, Title: "Heat"}}}, nil
	}
	return domain.MoviePage{Results: []domain.Movie{{ID: 5, Title: "Something Else"}}}, nil
}
func (pickCatalog) GetPerson(context.Context, int) (domain.Person, error) {
	return domain.Person{}, nil
}
func (pickCatalog) DiscoverMovies(_ context.Context, f domain.DiscoverFilter) (domain.MoviePage, error) {
	return domain.MoviePage{Results: feed(f.GenreID * 100)}, nil
}

// failingModel always fails with err, counting calls.
type failingModel struct {
	err   error
	calls atomic.Int32
}

func (m *failingModel) Reply(context.Context, domain.ChatRequest) (domain.ChatReply, error) {
	m.calls.Add(1)
	return domain.ChatReply{}, m.err
}

func fallbackAssistant(t *testing.T, err error) (*service.Assistant, *failingModel) {
	env := newSvcWith(t, pickCatalog{}, service.WithDiscovery(pickCatalog{}))
	m := &failingModel{err: err}
	return service.NewAssistant(m, env.svc), m
}

func ask(t *testing.T, a *service.Assistant, msg, locale string) service.ChatResponse {
	t.Helper()
	res, err := a.Chat(context.Background(), "u", []domain.ChatMessage{{Role: domain.RoleUser, Content: msg}}, "", locale)
	if err != nil {
		t.Fatalf("%q: %v", msg, err)
	}
	return res
}

func TestKeywordPicksWhenTheModelIsDown(t *testing.T) {
	a, _ := fallbackAssistant(t, domain.ErrUnavailable)
	cases := []struct {
		msg, locale string
		firstID     int
		says        string
	}{
		{"something funny tonight", "en", 3501, "comedy"},
		{"bu akşam komik bir film", "tr", 3501, "komedi"},
		{"Korku filmi gecesi", "tr", 2701, "korku"},
		{"a mind-bending sci-fi", "en", 87801, "science fiction"}, // sci-fi beats "mind-bending" (mystery)
		{"crime thrillers like Heat", "en", 8001, "If you liked **Heat**"},
		{"Heat gibi filmler", "tr", 8001, "**Heat** sevdiysen"},
		{"Heat", "en", 8001, "If you liked **Heat**"}, // a bare title that matches
		{"what should I watch tonight", "en", 1, "everyone is watching"},
	}
	for _, c := range cases {
		res := ask(t, a, c.msg, c.locale)
		if !res.Fallback || len(res.MovieIDs) != 5 || res.MovieIDs[0] != c.firstID || !strings.Contains(res.Message, c.says) {
			t.Errorf("%q -> fallback=%v ids=%v\n%s", c.msg, res.Fallback, res.MovieIDs, res.Message)
		}
		if len(res.Movies) != len(res.MovieIDs) {
			t.Errorf("%q: %d cards for %d ids", c.msg, len(res.Movies), len(res.MovieIDs))
		}
		for _, id := range res.MovieIDs {
			if id == 949 {
				t.Errorf("%q recommended the movie it was asked to compare to", c.msg)
			}
		}
	}
	if res := ask(t, a, "komik", "tr"); !strings.Contains(res.Message, "AI asistan şu anda yoğun") {
		t.Errorf("Turkish lead missing:\n%s", res.Message)
	}
}

func TestRateLimitCoolsDownTheModel(t *testing.T) {
	a, m := fallbackAssistant(t, &domain.RateLimitError{RetryAfter: 30 * time.Second})
	for range 3 {
		if res := ask(t, a, "funny", "en"); !res.Fallback {
			t.Fatal("expected keyword picks")
		}
	}
	if m.calls.Load() != 1 {
		t.Fatalf("the model was asked %d times during its cooldown", m.calls.Load())
	}
}

func TestOtherModelErrorsStillFail(t *testing.T) {
	boom := errors.New("boom")
	a, _ := fallbackAssistant(t, boom)
	if _, err := a.Chat(context.Background(), "u", []domain.ChatMessage{{Role: domain.RoleUser, Content: "hi"}}, "", "en"); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
