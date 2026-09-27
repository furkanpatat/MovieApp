package server_test

import (
	"net/http"
	"testing"
)

var libraryRoutes = [][3]string{ // method, path, body
	{"GET", "/api/v1/watchlist", ""},
	{"POST", "/api/v1/watchlist", `{"movie_id":7}`},
	{"DELETE", "/api/v1/watchlist/7", ""},
	{"GET", "/api/v1/ratings", ""},
	{"PUT", "/api/v1/ratings", `{"movie_id":7,"rating":8}`},
	{"POST", "/api/v1/chat", `{"messages":[{"role":"user","content":"hi"}]}`},
}

func TestLibraryRoutesRequireAuth(t *testing.T) {
	e := newEnv(t, opts{})
	for _, rt := range libraryRoutes {
		for name, hdr := range map[string]map[string]string{
			"no credentials":    {},
			"spoofed user only": {"X-User-Id": "alice"},
			"bad token":         bearer("not-a-jwt"),
		} {
			if resp := e.do(t, rt[0], rt[1], rt[2], hdr); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s [%s] -> %d, want 401", rt[0], rt[1], name, resp.StatusCode)
			}
		}
	}
	if hits, _ := e.catalog.snapshot(); hits != 0 {
		t.Fatalf("unauthenticated library requests reached the catalog %d times", hits)
	}
}

func TestLibraryRoutesGoToCatalogAsTheCookieUser(t *testing.T) {
	e := newEnv(t, opts{})
	tok := e.token(t, "alice")
	for _, rt := range libraryRoutes {
		resp := e.do(t, rt[0], rt[1], rt[2], map[string]string{
			"Cookie":    cookie(tok),
			"Origin":    appOrigin,
			"X-User-Id": "mallory", // the client tries to pick its own identity
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s -> %d", rt[0], rt[1], resp.StatusCode)
		}
		_, got := e.catalog.snapshot()
		if got.Method != rt[0] || got.Path != rt[1] || got.Body != rt[2] {
			t.Fatalf("catalog saw %+v", got)
		}
		if id := got.Header.Get("X-User-Id"); id != "alice" {
			t.Fatalf("%s %s: catalog saw X-User-Id=%q, want the session's user", rt[0], rt[1], id)
		}
		if got.Header.Get("Cookie") != "" {
			t.Fatal("the session cookie must not be forwarded")
		}
	}
	if hits, _ := e.interaction.snapshot(); hits != 0 {
		t.Fatal("library traffic must not reach the interaction service")
	}
}

func TestLibraryCookieWritesFromForeignOriginsAreRejected(t *testing.T) {
	e := newEnv(t, opts{})
	tok := e.token(t, "alice")
	for _, rt := range libraryRoutes {
		if rt[0] == "GET" {
			continue
		}
		resp := e.do(t, rt[0], rt[1], rt[2], map[string]string{"Cookie": cookie(tok), "Origin": "https://evil.example.net"})
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s -> %d, want 403", rt[0], rt[1], resp.StatusCode)
		}
	}
	if hits, _ := e.catalog.snapshot(); hits != 0 {
		t.Fatal("a cross-origin write reached the catalog")
	}
}

func TestPeopleAndSearchArePublicCatalogRoutes(t *testing.T) {
	e := newEnv(t, opts{})
	paths := []string{"/api/v1/people/31", "/api/v1/search/movies?q=dune", "/api/v1/discover/movies?genre=28", "/api/v1/tv/1399", "/api/v1/tv/popular", "/api/v1/discover/tv?genre=10765", "/api/v1/search/tv?q=office"}
	for _, path := range paths {
		resp := e.do(t, "GET", path, "", map[string]string{"X-User-Id": "admin"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s -> %d", path, resp.StatusCode)
		}
		if _, got := e.catalog.snapshot(); got.Header.Get("X-User-Id") != "" {
			t.Fatalf("%s: public route forwarded an identity", path)
		}
	}
	if hits, _ := e.catalog.snapshot(); hits != len(paths) {
		t.Fatalf("catalog hits = %d", hits)
	}
}
