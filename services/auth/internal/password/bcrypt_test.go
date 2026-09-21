package password_test

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/furkanpatat/movieapp/services/auth/internal/password"
)

func TestHashIsSaltedBcryptAndVerifies(t *testing.T) {
	h, err := password.NewBcrypt(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery staple"

	hash1, err := h.Hash(pw)
	if err != nil {
		t.Fatal(err)
	}
	hash2, _ := h.Hash(pw)

	if strings.Contains(hash1, pw) {
		t.Fatal("hash contains the plaintext password")
	}
	if !strings.HasPrefix(hash1, "$2") || len(hash1) != 60 {
		t.Fatalf("not a bcrypt hash: %q", hash1)
	}
	if hash1 == hash2 {
		t.Fatal("hashing twice must give different results (random salt)")
	}
	for _, hash := range []string{hash1, hash2} {
		if err := h.Compare(hash, pw); err != nil {
			t.Fatalf("correct password rejected: %v", err)
		}
	}
	for _, wrong := range []string{"", "correct horse battery stapl", "Correct horse battery staple", pw + " "} {
		if err := h.Compare(hash1, wrong); err == nil {
			t.Fatalf("wrong password %q accepted", wrong)
		}
	}
	if err := h.Compare("not-a-hash", pw); err == nil {
		t.Fatal("garbage hash accepted")
	}
}

func TestCostIsEmbeddedAndValidated(t *testing.T) {
	h, _ := password.NewBcrypt(6)
	hash, _ := h.Hash("password123")
	if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != 6 {
		t.Fatalf("cost = %d err=%v, want 6", cost, err)
	}
	for _, bad := range []int{0, 3, 32} {
		if _, err := password.NewBcrypt(bad); err == nil {
			t.Errorf("cost %d accepted", bad)
		}
	}
}
