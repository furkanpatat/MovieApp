package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
)

func TestNormalizeRegistrationAccepts(t *testing.T) {
	u, e, err := domain.NormalizeRegistration("  Alice_01 ", "  Alice@Example.COM ", "password1")
	if err != nil || u != "Alice_01" || e != "alice@example.com" {
		t.Fatalf("%q %q %v", u, e, err)
	}
}

func TestNormalizeRegistrationRejects(t *testing.T) {
	ok := func() (string, string, string) { return "alice", "alice@example.com", "password1" }
	cases := map[string]func() (string, string, string){
		"short username":   func() (string, string, string) { _, e, p := ok(); return "ab", e, p },
		"long username":    func() (string, string, string) { _, e, p := ok(); return strings.Repeat("a", 33), e, p },
		"username spaces":  func() (string, string, string) { _, e, p := ok(); return "al ice", e, p },
		"username at sign": func() (string, string, string) { _, e, p := ok(); return "al@ice", e, p },
		"username unicode": func() (string, string, string) { _, e, p := ok(); return "alicé", e, p },
		"empty email":      func() (string, string, string) { u, _, p := ok(); return u, "", p },
		"no domain":        func() (string, string, string) { u, _, p := ok(); return u, "alice@", p },
		"no tld":           func() (string, string, string) { u, _, p := ok(); return u, "alice@localhost", p },
		"display name":     func() (string, string, string) { u, _, p := ok(); return u, "Alice <alice@example.com>", p },
		"two addresses":    func() (string, string, string) { u, _, p := ok(); return u, "a@example.com, b@example.com", p },
		"short password":   func() (string, string, string) { u, e, _ := ok(); return u, e, "short" },
		"empty password":   func() (string, string, string) { u, e, _ := ok(); return u, e, "" },
		"73 byte password": func() (string, string, string) { u, e, _ := ok(); return u, e, strings.Repeat("a", 73) },
	}
	for name, f := range cases {
		u, e, p := f()
		if _, _, err := domain.NormalizeRegistration(u, e, p); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
	// exactly at the bcrypt limit is fine
	if _, _, err := domain.NormalizeRegistration("alice", "alice@example.com", strings.Repeat("a", 72)); err != nil {
		t.Errorf("72-byte password rejected: %v", err)
	}
}
