package transport_test

import (
	"context"
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

// memWatched is an in-memory WatchedStore; alice's username is "alice".
type memWatched struct {
	mu   sync.Mutex
	byID map[string]map[domain.TitleRef]time.Time
}

func (m *memWatched) MarkWatched(_ context.Context, u string, ref domain.TitleRef) (domain.WatchedItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byID[u] == nil {
		m.byID[u] = map[domain.TitleRef]time.Time{}
	}
	if _, ok := m.byID[u][ref]; !ok {
		m.byID[u][ref] = time.Now()
	}
	return domain.WatchedItem{Movie: domain.Movie{ID: ref.ID, MediaType: ref.MediaType}, WatchedAt: m.byID[u][ref]}, nil
}
func (m *memWatched) UnmarkWatched(_ context.Context, u string, ref domain.TitleRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID[u], ref)
	return nil
}
func (m *memWatched) GetUserWatched(_ context.Context, u string) ([]domain.WatchedItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := []domain.WatchedItem{}
	for ref, at := range m.byID[u] {
		items = append(items, domain.WatchedItem{Movie: domain.Movie{ID: ref.ID, MediaType: ref.MediaType}, WatchedAt: at})
	}
	return items, nil
}
func (m *memWatched) GetWatchedByUsername(ctx context.Context, name string, _ int) (domain.PublicWatched, error) {
	if !strings.EqualFold(name, "alice") {
		return domain.PublicWatched{}, domain.ErrNotFound
	}
	items, _ := m.GetUserWatched(ctx, alice)
	return domain.PublicWatched{Username: "alice", Items: items}, nil
}

func TestWatchedRoutes(t *testing.T) {
	mem := &memWatched{byID: map[string]map[domain.TitleRef]time.Time{}}
	srv := httptest.NewServer(transport.NewHandler(nil, service.NewLibrary(newMem(), storeAll{}).WithWatched(mem), nil, quiet))
	t.Cleanup(srv.Close)

	if code, _ := call(t, srv, "POST", "/api/v1/watched", "", `{"movie_id":27205}`); code != http.StatusUnauthorized {
		t.Fatalf("anonymous mark -> %d", code)
	}
	for _, body := range []string{`{"movie_id":27205}`, `{"media_type":"tv","movie_id":1399}`} {
		if code, _ := call(t, srv, "POST", "/api/v1/watched", alice, body); code != http.StatusOK {
			t.Fatalf("mark %s -> %d", body, code)
		}
	}
	code, res := call(t, srv, "GET", "/api/v1/watched", alice, "")
	if items, _ := res["items"].([]any); code != http.StatusOK || len(items) != 2 {
		t.Fatalf("mine -> %d %v", code, res)
	}

	// Public, no identity: anyone can see alice's watched titles.
	code, res = call(t, srv, "GET", "/api/v1/users/Alice/watched", "", "")
	if items, _ := res["items"].([]any); code != http.StatusOK || res["username"] != "alice" || len(items) != 2 {
		t.Fatalf("public -> %d %v", code, res)
	}
	for _, name := range []string{"nobody", "x", "bad%20name"} {
		if code, _ := call(t, srv, "GET", "/api/v1/users/"+name+"/watched", "", ""); code != http.StatusNotFound {
			t.Errorf("%s -> %d, want 404", name, code)
		}
	}

	if code, _ := call(t, srv, "DELETE", "/api/v1/watched/1399?media_type=tv", alice, ""); code != http.StatusNoContent {
		t.Fatalf("unmark -> %d", code)
	}
	if _, res = call(t, srv, "GET", "/api/v1/watched", alice, ""); len(res["items"].([]any)) != 1 {
		t.Fatalf("after unmark %v", res)
	}
	if code, _ := call(t, srv, "POST", "/api/v1/watched", alice, `{"media_type":"book","movie_id":1}`); code != http.StatusBadRequest {
		t.Fatalf("bad media type -> %d", code)
	}
}
