package jwtauth

import (
	"testing"
	"time"
)

func TestIssueVerifyRoundTripAndExpiry(t *testing.T) {
	now := time.Now()
	clock := now
	m := newManager("0123456789012345678901234567890123456789", "iss", time.Minute, func() time.Time { return clock })

	tok, exp, err := m.Issue("alice")
	if err != nil || !exp.Equal(now.Add(time.Minute)) {
		t.Fatal(err, exp)
	}
	if id, err := m.Verify(tok); err != nil || id != "alice" {
		t.Fatalf("%q %v", id, err)
	}
	clock = now.Add(2 * time.Minute)
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
	clock = now.Add(-time.Hour) // issued "in the future"
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("token from the future accepted")
	}
}

func TestIssueRejectsUnsafeIDs(t *testing.T) {
	m := NewManager("0123456789012345678901234567890123456789", "iss", time.Minute)
	for _, id := range []string{"", "a b", "x\ny", "é", string(make([]byte, 65))} {
		if _, _, err := m.Issue(id); err == nil {
			t.Errorf("%q accepted", id)
		}
	}
	for _, id := range []string{"alice", "a.b_c-d", "user@example.com", "google:12345"} {
		if _, _, err := m.Issue(id); err != nil {
			t.Errorf("%q rejected: %v", id, err)
		}
	}
}
