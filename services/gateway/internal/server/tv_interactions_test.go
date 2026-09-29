package server_test

import (
	"net/http"
	"testing"
)

// Series' ratings and comments go to the Interaction service (not the
// catalog, which serves the rest of /api/v1/tv/), writes signed in only.
func TestSeriesInteractionsRouteToInteraction(t *testing.T) {
	e := newEnv(t, opts{})

	// (The fake upstreams answer 202 to everything; what matters is where
	// each request lands and whether the gateway let it through.)
	if resp := e.do(t, "GET", "/api/v1/tv/1399/interactions", "", nil); resp.StatusCode >= 300 {
		t.Fatalf("public read -> %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/v1/tv/1399/rate", `{"score":9}`, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous rate -> %d, want 401", resp.StatusCode)
	}
	auth := map[string]string{"Authorization": "Bearer " + e.token(t, "alice")}
	for _, path := range []string{"/api/v1/tv/1399/rate", "/api/v1/tv/1399/comment"} {
		if resp := e.do(t, "POST", path, `{}`, auth); resp.StatusCode >= 300 {
			t.Fatalf("%s -> %d", path, resp.StatusCode)
		}
	}
	if hits, got := e.interaction.snapshot(); hits != 3 || got.Header.Get("X-User-Id") != "alice" {
		t.Fatalf("interaction saw %d requests, last as %q", hits, got.Header.Get("X-User-Id"))
	}
	// The rest of /tv/ still goes to the catalog.
	before, _ := e.catalog.snapshot()
	if resp := e.do(t, "GET", "/api/v1/tv/1399", "", nil); resp.StatusCode >= 300 {
		t.Fatalf("tv details -> %d", resp.StatusCode)
	}
	if after, _ := e.catalog.snapshot(); after != before+1 {
		t.Fatal("tv details did not reach the catalog")
	}
}
