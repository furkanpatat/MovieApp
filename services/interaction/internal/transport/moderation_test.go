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

// stubRM records the read models dropped; nothing else is used here.
type stubRM struct {
	domain.ReadModel
	dropped []domain.Title
}

func (r *stubRM) Delete(_ context.Context, t domain.Title) error {
	r.dropped = append(r.dropped, t)
	return nil
}

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
func (m *memModeration) ReportedComments(context.Context, int) ([]domain.ReportedComment, error) {
	out := []domain.ReportedComment{}
	for id := range m.comments {
		if m.reports[id+"|alice"] {
			out = append(out, domain.ReportedComment{Comment: domain.Comment{ID: id}, MediaType: "movie", MovieID: 7, Reports: 1})
		}
	}
	return out, nil
}
func (m *memModeration) DeleteComment(_ context.Context, id string) (domain.Title, error) {
	if !m.comments[id] {
		return domain.Title{}, domain.ErrNotFound
	}
	delete(m.comments, id)
	return domain.Title{Media: "movie", ID: 7}, nil
}
func (m *memModeration) DismissReports(_ context.Context, id string) error {
	delete(m.reports, id+"|alice")
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
	h := transport.NewHandler(nil, nil, nil, nil, quiet, transport.WithModeration(service.NewModeration(store, &stubRM{})))

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

func TestAdminEndpoints(t *testing.T) {
	known := uuid.NewString()
	store := &memModeration{comments: map[string]bool{known: true}, reports: map[string]bool{}, blocks: map[string]bool{}}
	rm := &stubRM{}
	h := transport.NewHandler(nil, nil, nil, nil, quiet, transport.WithModeration(service.NewModeration(store, rm)))
	doAs(h, "alice", "POST", "/api/v1/comments/"+known+"/report", "")

	if rec := doAs(h, "root", "GET", "/api/v1/admin/reports", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), known) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := doAs(h, "", "GET", "/api/v1/admin/reports", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("list signed out: %d", rec.Code)
	}
	if rec := doAs(h, "root", "POST", "/api/v1/admin/comments/"+known+"/dismiss", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss: %d", rec.Code)
	}
	if rec := doAs(h, "root", "GET", "/api/v1/admin/reports", ""); strings.Contains(rec.Body.String(), known) {
		t.Fatalf("a dismissed report is still listed: %s", rec.Body)
	}
	if rec := doAs(h, "root", "POST", "/api/v1/admin/comments/"+known+"/delete", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if len(rm.dropped) != 1 || rm.dropped[0].ID != 7 {
		t.Fatalf("the title's read model was not dropped: %+v", rm.dropped)
	}
	if rec := doAs(h, "root", "POST", "/api/v1/admin/comments/"+known+"/delete", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", rec.Code)
	}
	if rec := doAs(h, "root", "POST", "/api/v1/admin/comments/nope/delete", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete a non-id: %d", rec.Code)
	}
}
