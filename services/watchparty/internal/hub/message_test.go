package hub

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestParseChatAttachesServerIdentity(t *testing.T) {
	// The client tries to impersonate: user_id / room in the payload must be ignored.
	out, pb, err := parse([]byte(`{"type":"chat_message","text":"  hi there ","user_id":"admin","room":"other"}`), "alice", "movie-night", 500, now)
	if err != nil || pb != nil {
		t.Fatalf("%v %v", err, pb)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["user_id"] != "alice" || m["room"] != "movie-night" || m["text"] != "hi there" || m["type"] != "chat_message" {
		t.Fatalf("%v", m)
	}
}

func TestParsePlayback(t *testing.T) {
	for _, action := range []string{"play", "pause", "seek"} {
		out, pb, err := parse([]byte(`{"type":"playback_sync","action":"`+action+`","timestamp":12.5}`), "alice", "r", 500, now)
		if err != nil || pb == nil || pb.Action != action || pb.Timestamp != 12.5 || pb.UserID != "alice" {
			t.Fatalf("%s: %v %+v", action, err, pb)
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		if m["timestamp"] != 12.5 || m["action"] != action || m["user_id"] != "alice" {
			t.Fatalf("%v", m)
		}
	}
	// timestamp 0 is a valid position (start of the video), distinct from "missing"
	if _, _, err := parse([]byte(`{"type":"playback_sync","action":"seek","timestamp":0}`), "a", "r", 500, now); err != nil {
		t.Fatalf("timestamp 0 rejected: %v", err)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"not json":          `nope`,
		"empty object":      `{}`,
		"unknown type":      `{"type":"kick","user":"x"}`,
		"chat no text":      `{"type":"chat_message"}`,
		"chat blank":        `{"type":"chat_message","text":"  \n "}`,
		"chat too long":     `{"type":"chat_message","text":"` + strings.Repeat("a", 501) + `"}`,
		"sync no action":    `{"type":"playback_sync","timestamp":1}`,
		"sync bad action":   `{"type":"playback_sync","action":"rewind","timestamp":1}`,
		"sync no timestamp": `{"type":"playback_sync","action":"play"}`,
		"sync negative":     `{"type":"playback_sync","action":"seek","timestamp":-1}`,
		"sync huge":         `{"type":"playback_sync","action":"seek","timestamp":1e12}`,
		"sync string ts":    `{"type":"playback_sync","action":"seek","timestamp":"5"}`,
	}
	for name, in := range cases {
		_, _, err := parse([]byte(in), "alice", "r", 500, now)
		var bad badMessage
		if !errors.As(err, &bad) {
			t.Errorf("%s: want badMessage, got %v", name, err)
		}
	}
	// multibyte text is counted in characters, not bytes
	if _, _, err := parse([]byte(`{"type":"chat_message","text":"`+strings.Repeat("ş", 500)+`"}`), "a", "r", 500, now); err != nil {
		t.Errorf("500 multibyte characters rejected: %v", err)
	}
}
