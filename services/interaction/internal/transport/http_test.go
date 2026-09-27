package transport_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/readmodel"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
	"github.com/furkanpatat/movieapp/services/interaction/internal/transport"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// outbox is a fake domain.Outbox.
type outbox struct {
	n   int
	err error
}

func (o *outbox) Enqueue(context.Context, domain.OutboxMessage) error {
	o.n++
	return o.err
}

// panicRepo proves the write path never touches the projection tables.
type panicRepo struct{}

func (panicRepo) SaveRating(context.Context, domain.RatingSubmitted) (domain.RatingStats, error) {
	panic("write path touched Postgres")
}
func (panicRepo) SaveComment(context.Context, domain.CommentAdded) error {
	panic("write path touched Postgres")
}
func (panicRepo) GetStats(context.Context, domain.Title) (domain.RatingStats, error) {
	return domain.RatingStats{}, errors.New("no db")
}
func (panicRepo) RecentComments(context.Context, domain.Title, int) ([]domain.Comment, error) {
	return nil, errors.New("no db")
}

func server(t *testing.T, p *outbox) (http.Handler, *readmodel.Model) {
	mr := miniredis.RunT(t)
	rm := readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20)
	return transport.NewHandler(service.NewCommand(p), service.NewQuery(panicRepo{}, rm, 20, quiet), nil, quiet), rm
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	return doAs(h, "alice", method, path, body)
}

// doAs sends the request as the given user ("" = no identity header).
func doAs(h http.Handler, user, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User-Id", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWriteEndpointsReturn202(t *testing.T) {
	p := &outbox{}
	h, _ := server(t, p)
	for path, body := range map[string]string{
		"/api/v1/movies/7/rate":    `{"score":9}`,
		"/api/v1/movies/7/comment": `{"text":"loved it"}`,
	} {
		rec := do(h, "POST", path, body)
		if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"event_id"`) {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
	if p.n != 2 {
		t.Fatalf("stored %d events, want 2", p.n)
	}
}

func TestBadRequests(t *testing.T) {
	p := &outbox{}
	h, _ := server(t, p)
	cases := []struct{ path, body string }{
		{"/api/v1/movies/7/rate", `{"score":0}`},
		{"/api/v1/movies/7/rate", `{"score":11}`},
		{"/api/v1/movies/7/rate", `{"score":7.5}`},
		{"/api/v1/movies/7/rate", `{"score":5,"admin":true}`},    // unknown field
		{"/api/v1/movies/7/rate", `{"user_id":"bob","score":5}`}, // identity in the body is refused
		{"/api/v1/movies/7/rate", `not json`},
		{"/api/v1/movies/7/rate", `{"score":5} trailing`},
		{"/api/v1/movies/abc/rate", `{"score":5}`},
		{"/api/v1/movies/0/comment", `{"text":"x"}`},
		{"/api/v1/movies/7/comment", `{"text":""}`},
		{"/api/v1/movies/7/comment", `{"user_id":"bob","text":"impersonation"}`},
		{"/api/v1/movies/7/comment", `{"text":"` + strings.Repeat("x", 1001) + `"}`},
	}
	for _, c := range cases {
		if rec := do(h, "POST", c.path, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s -> %d, want 400", c.path, c.body, rec.Code)
		}
	}
	if p.n != 0 {
		t.Fatalf("invalid requests stored %d events", p.n)
	}
}

func TestMissingIdentityIs401(t *testing.T) {
	p := &outbox{}
	h, _ := server(t, p)
	for _, path := range []string{"/api/v1/movies/7/rate", "/api/v1/movies/7/comment"} {
		if rec := doAs(h, "", "POST", path, `{"score":5,"text":"x"}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without identity -> %d, want 401", path, rec.Code)
		}
	}
	if p.n != 0 {
		t.Fatal("unauthenticated request stored an event")
	}
}

func TestEventCarriesHeaderIdentity(t *testing.T) {
	var got domain.OutboxMessage
	mr := miniredis.RunT(t)
	rm := readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20)
	h := transport.NewHandler(service.NewCommand(captureOutbox{&got}), service.NewQuery(panicRepo{}, rm, 20, quiet), nil, quiet)
	if rec := doAs(h, "carol", "POST", "/api/v1/movies/7/rate", `{"score":4}`); rec.Code != http.StatusAccepted {
		t.Fatal(rec.Code)
	}
	if !strings.Contains(string(got.Payload), `"user_id":"carol"`) {
		t.Fatalf("event should carry the header identity: %s", got.Payload)
	}
}

type captureOutbox struct{ into *domain.OutboxMessage }

func (c captureOutbox) Enqueue(_ context.Context, m domain.OutboxMessage) error {
	*c.into = m
	return nil
}

func TestStoreDownGives503(t *testing.T) {
	h, _ := server(t, &outbox{err: errors.New("broker down")})
	if rec := do(h, "POST", "/api/v1/movies/7/rate", `{"score":5}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestGetServedFromReadModelOnly(t *testing.T) {
	h, rm := server(t, &outbox{})
	_ = rm.Init(context.Background(), domain.RatingStats{Title: domain.Movie(7), TotalScore: 17, VoteCount: 2, Version: 1}, nil)
	_, _ = rm.AddComment(context.Background(), domain.Movie(7), domain.Comment{ID: "c", UserID: "a", Text: "hey"})

	rec := do(h, "GET", "/api/v1/movies/7/interactions", "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for _, want := range []string{`"average_rating":8.5`, `"total_votes":2`, `"text":"hey"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %s in %s", want, rec.Body)
		}
	}
	if rec := do(h, "GET", "/api/v1/movies/7/interactions?limit=x", ""); rec.Code != 400 {
		t.Fatalf("limit=x -> %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/movies/7/interactions", ""); rec.Code != 200 {
		t.Fatal("repeat read failed")
	}
}

func TestGetOnColdCacheWithDeadDatabaseIs503(t *testing.T) {
	h, _ := server(t, &outbox{})
	if rec := do(h, "GET", "/api/v1/movies/99/interactions", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}
