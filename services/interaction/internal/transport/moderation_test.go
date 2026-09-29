package transport_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
	"github.com/furkanpatat/movieapp/services/interaction/internal/transport"
)

// memModeration is an in-memory ModerationStore.
type memModeration struct {
	comments map[string]bool
	reports  map[string]bool // commentID|reporter
	blocks   map[string]bool // blocker|blocked
}

func (m *memModeration) ReportComment(_ context.Context, id, by string) error {
	if !m.comments[id] {
		return domain.ErrNotFound
	}
	m.reports[id+"|"+by] = true
	return nil
}
func (m *memModeration) BlockUser(_ context.Context, a, b string) error {
	m.blocks[a+"|"+b] = true
	return nil
}
func (m *memModeration) UnblockUser(_ context.Context, a, b string) error {
	delete(m.blocks, a+"|"+b)
	return nil
}
func (m *memModeration) BlockedUsers(_ context.Context, a string) ([]string, error) {
	out := []string{}
	for k := range m.blocks {
		if blocker, blocked, _ := strings.Cut(k, "|"); blocker == a {
			out = append(out, blocked)
		}
	}
	return out, nil
}

func TestModerationEndpoints(t *testing.T) {
	known := uuid.NewString()
	store := &memModeration{comments: map[string]bool{known: true}, reports: map[string]bool{}, blocks: map[string]bool{}}
	h := transport.NewHandler(nil, nil, nil, nil, quiet, transport.WithModeration(service.NewModeration(store)))

	cases := []struct {
		name, user, method, path string
		want                     int
	}{
		{"report", "alice", "POST", "/api/v1/comments/" + known + "/report", http.StatusNoContent},
		{"report twice", "alice", "POST", "/api/v1/comments/" + known + "/report", http.StatusNoContent},
		{"report unknown comment", "alice", "POST", "/api/v1/comments/" + uuid.NewString() + "/report", http.StatusNotFound},
		{"report a non-id", "alice", "POST", "/api/v1/comments/nope/report", http.StatusBadRequest},
		{"report signed out", "", "POST", "/api/v1/comments/" + known + "/report", http.StatusUnauthorized},
		{"block", "alice", "PUT", "/api/v1/blocks/bob", http.StatusNoContent},
		{"block yourself", "alice", "PUT", "/api/v1/blocks/alice", http.StatusBadRequest},
		{"block signed out", "", "PUT", "/api/v1/blocks/bob", http.StatusUnauthorized},
	}
	for _, c := range cases {
		if rec := doAs(h, c.user, c.method, c.path, ""); rec.Code != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, rec.Code, rec.Body, c.want)
		}
	}
	if !store.reports[known+"|alice"] {
		t.Error("the report was not stored under the caller")
	}

	if rec := doAs(h, "alice", "GET", "/api/v1/blocks", ""); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"blocked":["bob"]}` {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := doAs(h, "bob", "GET", "/api/v1/blocks", ""); strings.TrimSpace(rec.Body.String()) != `{"blocked":[]}` {
		t.Fatalf("bob blocked nobody, got %s", rec.Body)
	}
	if rec := doAs(h, "alice", "DELETE", "/api/v1/blocks/bob", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("unblock: %d", rec.Code)
	}
	if rec := doAs(h, "alice", "GET", "/api/v1/blocks", ""); strings.TrimSpace(rec.Body.String()) != `{"blocked":[]}` {
		t.Fatalf("after unblock: %s", rec.Body)
	}
}
