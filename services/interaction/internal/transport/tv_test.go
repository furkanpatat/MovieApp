package transport_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/readmodel"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
	"github.com/furkanpatat/movieapp/services/interaction/internal/transport"
)

func TestSeriesHaveTheirOwnEndpoints(t *testing.T) {
	var got domain.OutboxMessage
	mr := miniredis.RunT(t)
	rm := readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20)
	h := transport.NewHandler(service.NewCommand(captureOutbox{&got}), service.NewQuery(panicRepo{}, rm, 20, quiet), nil, nil, quiet)

	if rec := do(h, "POST", "/api/v1/tv/1399/rate", `{"score":9}`); rec.Code != http.StatusAccepted {
		t.Fatalf("rate a series -> %d %s", rec.Code, rec.Body)
	}
	if p := string(got.Payload); !strings.Contains(p, `"media_type":"tv"`) || !strings.Contains(p, `"movie_id":1399`) {
		t.Fatalf("series rating event %s", p)
	}
	if rec := do(h, "POST", "/api/v1/tv/1399/comment", `{"text":"Winter is coming"}`); rec.Code != http.StatusAccepted ||
		!strings.Contains(string(got.Payload), `"media_type":"tv"`) {
		t.Fatalf("comment on a series -> %d %s", rec.Code, got.Payload)
	}
	// A movie's event says so (and old consumers still read movie_id).
	if rec := do(h, "POST", "/api/v1/movies/1399/rate", `{"score":3}`); rec.Code != http.StatusAccepted ||
		!strings.Contains(string(got.Payload), `"media_type":"movie"`) {
		t.Fatalf("movie event %s", got.Payload)
	}

	// Series 1399 and movie 1399 are different titles in the read model.
	ctx := context.Background()
	_ = rm.Init(ctx, domain.RatingStats{Title: domain.Title{Media: domain.MediaTV, ID: 1399}, TotalScore: 18, VoteCount: 2, Version: 1}, nil)
	_ = rm.Init(ctx, domain.RatingStats{Title: domain.Movie(1399), TotalScore: 3, VoteCount: 1, Version: 1}, nil)
	tv := do(h, "GET", "/api/v1/tv/1399/interactions", "")
	movie := do(h, "GET", "/api/v1/movies/1399/interactions", "")
	if !strings.Contains(tv.Body.String(), `"average_rating":9`) || !strings.Contains(tv.Body.String(), `"media_type":"tv"`) {
		t.Fatalf("series interactions %d %s", tv.Code, tv.Body)
	}
	if !strings.Contains(movie.Body.String(), `"average_rating":3`) || !strings.Contains(movie.Body.String(), `"media_type":"movie"`) {
		t.Fatalf("movie interactions %d %s", movie.Code, movie.Body)
	}
	if rec := do(h, "GET", "/api/v1/tv/0/interactions", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad series id -> %d", rec.Code)
	}
}
