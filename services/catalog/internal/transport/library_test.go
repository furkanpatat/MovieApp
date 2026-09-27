package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
	"github.com/furkanpatat/movieapp/services/catalog/internal/transport"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

const (
	alice = "7f2c1a4e-0000-4000-8000-000000000001"
	bob   = "7f2c1a4e-0000-4000-8000-000000000002"
	gone  = "7f2c1a4e-0000-4000-8000-00000000dead" // token outlived the account
)

// memLibrary is an in-memory LibraryStore keyed by user.
type memLibrary struct {
	mu      sync.Mutex
	list    map[string]map[domain.TitleRef]time.Time
	ratings map[string]map[domain.TitleRef]int
}

func newMem() *memLibrary {
	return &memLibrary{list: map[string]map[domain.TitleRef]time.Time{}, ratings: map[string]map[domain.TitleRef]int{}}
}

func (m *memLibrary) AddToWatchlist(_ context.Context, u string, id domain.TitleRef) (domain.WatchlistItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u == gone {
		return domain.WatchlistItem{}, domain.ErrUnknownUser
	}
	if m.list[u] == nil {
		m.list[u] = map[domain.TitleRef]time.Time{}
	}
	if _, ok := m.list[u][id]; !ok {
		m.list[u][id] = time.Now()
	}
	return domain.WatchlistItem{Movie: domain.Movie{ID: id.ID, MediaType: id.MediaType, Title: "M"}, AddedAt: m.list[u][id]}, nil
}
func (m *memLibrary) RemoveFromWatchlist(_ context.Context, u string, id domain.TitleRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.list[u], id)
	return nil
}
func (m *memLibrary) GetUserWatchlist(_ context.Context, u string) ([]domain.WatchlistItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := []domain.WatchlistItem{}
	for id, at := range m.list[u] {
		items = append(items, domain.WatchlistItem{Movie: domain.Movie{ID: id.ID, MediaType: id.MediaType}, AddedAt: at})
	}
	return items, nil
}
func (m *memLibrary) UpsertUserRating(_ context.Context, u string, id domain.TitleRef, r int) (domain.UserRating, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ratings[u] == nil {
		m.ratings[u] = map[domain.TitleRef]int{}
	}
	m.ratings[u][id] = r
	return domain.UserRating{Movie: domain.Movie{ID: id.ID, MediaType: id.MediaType}, Rating: r}, nil
}
func (m *memLibrary) GetUserRatings(_ context.Context, u string) ([]domain.UserRating, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := []domain.UserRating{}
	for id, r := range m.ratings[u] {
		items = append(items, domain.UserRating{Movie: domain.Movie{ID: id.ID, MediaType: id.MediaType}, Rating: r})
	}
	return items, nil
}

type storeAll struct{}

func (storeAll) EnsureTitleStored(context.Context, domain.TitleRef) error { return nil }

func newServer(t *testing.T) (*httptest.Server, *memLibrary) {
	mem := newMem()
	srv := httptest.NewServer(transport.NewHandler(nil, service.NewLibrary(mem, storeAll{}), nil, quiet))
	t.Cleanup(srv.Close)
	return srv, mem
}

func call(t *testing.T, srv *httptest.Server, method, path, user, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if user != "" {
		req.Header.Set(transport.UserIDHeader, user)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestLibraryRoutesRequireAUserIdentity(t *testing.T) {
	srv, mem := newServer(t)
	routes := [][3]string{
		{"GET", "/api/v1/watchlist", ""},
		{"POST", "/api/v1/watchlist", `{"movie_id":1}`},
		{"DELETE", "/api/v1/watchlist/1", ""},
		{"GET", "/api/v1/ratings", ""},
		{"PUT", "/api/v1/ratings", `{"movie_id":1,"rating":5}`},
	}
	for _, user := range []string{"", "alice", "' OR 1=1 --"} {
		for _, rt := range routes {
			if code, _ := call(t, srv, rt[0], rt[1], user, rt[2]); code != http.StatusUnauthorized {
				t.Errorf("%s %s as %q -> %d, want 401", rt[0], rt[1], user, code)
			}
		}
	}
	if len(mem.list)+len(mem.ratings) != 0 {
		t.Fatal("an unauthenticated request wrote to the store")
	}
}

func TestWatchlistLifecycle(t *testing.T) {
	srv, _ := newServer(t)

	for range 2 { // re-adding is idempotent
		code, item := call(t, srv, "POST", "/api/v1/watchlist", alice, `{"movie_id":42}`)
		if code != http.StatusOK || item["movie"].(map[string]any)["id"] != float64(42) || item["added_at"] == nil {
			t.Fatalf("add -> %d %v", code, item)
		}
	}
	_, got := call(t, srv, "GET", "/api/v1/watchlist", alice, "")
	if items := got["items"].([]any); len(items) != 1 {
		t.Fatalf("alice's list = %v", items)
	}
	// Lists are per user.
	if _, got := call(t, srv, "GET", "/api/v1/watchlist", bob, ""); len(got["items"].([]any)) != 0 {
		t.Fatalf("bob sees %v", got["items"])
	}

	for range 2 { // removing is idempotent
		if code, _ := call(t, srv, "DELETE", "/api/v1/watchlist/42", alice, ""); code != http.StatusNoContent {
			t.Fatalf("delete -> %d", code)
		}
	}
	if _, got := call(t, srv, "GET", "/api/v1/watchlist", alice, ""); len(got["items"].([]any)) != 0 {
		t.Fatalf("list after delete = %v", got["items"])
	}
}

func TestRatingUpsert(t *testing.T) {
	srv, mem := newServer(t)
	for _, r := range []int{6, 9} {
		code, item := call(t, srv, "PUT", "/api/v1/ratings", alice, fmt.Sprintf(`{"movie_id":7,"rating":%d}`, r))
		if code != http.StatusOK || item["rating"] != float64(r) {
			t.Fatalf("rate %d -> %d %v", r, code, item)
		}
	}
	if mem.ratings[alice][domain.MovieRef(7)] != 9 {
		t.Fatalf("stored rating = %d, want 9 (the latest)", mem.ratings[alice][domain.MovieRef(7)])
	}
	_, got := call(t, srv, "GET", "/api/v1/ratings", alice, "")
	if items := got["items"].([]any); len(items) != 1 {
		t.Fatalf("ratings = %v", items)
	}
}

func TestLibraryRejectsBadInput(t *testing.T) {
	srv, mem := newServer(t)
	cases := map[string][4]string{
		"rating too high":     {"PUT", "/api/v1/ratings", `{"movie_id":7,"rating":11}`},
		"rating missing":      {"PUT", "/api/v1/ratings", `{"movie_id":7}`},
		"movie id zero":       {"POST", "/api/v1/watchlist", `{"movie_id":0}`},
		"body picks the user": {"POST", "/api/v1/watchlist", `{"movie_id":7,"user_id":"` + bob + `"}`},
		"not json":            {"PUT", "/api/v1/ratings", `rating=5`},
		"trailing garbage":    {"POST", "/api/v1/watchlist", `{"movie_id":7}{}`},
		"non-numeric path":    {"DELETE", "/api/v1/watchlist/abc", ""},
	}
	for name, c := range cases {
		if code, _ := call(t, srv, c[0], c[1], alice, c[2]); code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", name, code)
		}
	}
	if len(mem.list)+len(mem.ratings) != 0 {
		t.Fatal("bad input reached the store")
	}
}

func TestDeletedAccountIsUnauthorized(t *testing.T) {
	srv, _ := newServer(t)
	if code, _ := call(t, srv, "POST", "/api/v1/watchlist", gone, `{"movie_id":1}`); code != http.StatusUnauthorized {
		t.Fatalf("-> %d, want 401", code)
	}
}

func TestSeriesInTheLibraryRoutes(t *testing.T) {
	srv, mem := newServer(t)
	code, item := call(t, srv, "POST", "/api/v1/watchlist", alice, `{"media_type":"tv","movie_id":1399}`)
	if movie, _ := item["movie"].(map[string]any); code != http.StatusOK || movie["media_type"] != "tv" {
		t.Fatalf("add series -> %d %v", code, item)
	}
	if code, _ := call(t, srv, "POST", "/api/v1/watchlist", alice, `{"movie_id":1399}`); code != http.StatusOK {
		t.Fatalf("add movie 1399 -> %d", code)
	}
	if len(mem.list[alice]) != 2 {
		t.Fatalf("series and movie 1399 should be two entries: %v", mem.list[alice])
	}
	if code, _ := call(t, srv, "DELETE", "/api/v1/watchlist/1399?media_type=tv", alice, ""); code != http.StatusNoContent {
		t.Fatalf("remove series -> %d", code)
	}
	if _, ok := mem.list[alice][domain.MovieRef(1399)]; !ok || len(mem.list[alice]) != 1 {
		t.Fatalf("removing the series touched the movie: %v", mem.list[alice])
	}
	if code, _ := call(t, srv, "PUT", "/api/v1/ratings", alice, `{"media_type":"tv","movie_id":1399,"rating":9}`); code != http.StatusOK ||
		mem.ratings[alice][domain.TitleRef{MediaType: domain.MediaTV, ID: 1399}] != 9 {
		t.Fatalf("rate series -> %d %v", code, mem.ratings[alice])
	}
	for _, c := range [][3]string{
		{"POST", "/api/v1/watchlist", `{"media_type":"podcast","movie_id":1}`},
		{"DELETE", "/api/v1/watchlist/1?media_type=podcast", ""},
		{"PUT", "/api/v1/ratings", `{"media_type":"book","movie_id":1,"rating":5}`},
	} {
		if code, _ := call(t, srv, c[0], c[1], alice, c[2]); code != http.StatusBadRequest {
			t.Errorf("%s %s -> %d, want 400", c[0], c[1], code)
		}
	}
}
