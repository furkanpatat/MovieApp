package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// signup registers a fresh user and logs in, returning their id and JWT.
func signup(t *testing.T, gw, label string) (id, token string) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	username, email, pw := "ws_"+label+suffix[len(suffix)-8:], "ws"+label+suffix+"@e2e.test", "correct-horse-battery"
	r := call(t, "POST", gw+"/api/v1/auth/register", "", fmt.Sprintf(`{"username":%q,"email":%q,"password":%q}`, username, email, pw))
	if r.code != 201 {
		t.Fatalf("register %s -> %d %s", label, r.code, r.body)
	}
	id = field(t, r.body, "id")
	r = call(t, "POST", gw+"/api/v1/auth/login", "", fmt.Sprintf(`{"login":%q,"password":%q}`, username, pw))
	if r.code != 200 {
		t.Fatalf("login %s -> %d %s", label, r.code, r.body)
	}
	return id, field(t, r.body, "access_token")
}

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
	name string
}

// browserDial connects the way a browser must: the JWT rides in ?token=, with
// no custom headers, and an Origin header.
func browserDial(t *testing.T, gw, room, token, name string) *wsClient {
	t.Helper()
	url := "ws" + strings.TrimPrefix(gw, "http") + "/api/v1/watch-party/rooms/" + room + "/ws?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"http://localhost:3000"}})
	if err != nil {
		t.Fatalf("%s dial: %v (resp %v)", name, err, resp)
	}
	t.Cleanup(func() { conn.Close() })
	c := &wsClient{t, conn, name}
	c.expect("room_state")
	return c
}

func (c *wsClient) expect(typ string) map[string]any {
	c.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_ = c.conn.SetReadDeadline(deadline)
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.t.Fatalf("%s waiting for %s: %v", c.name, typ, err)
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["type"] == typ {
			return m
		}
	}
}

func (c *wsClient) noChat(d time.Duration) {
	c.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(deadline)
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if strings.Contains(string(data), `"chat_message"`) || strings.Contains(string(data), `"playback_sync"`) {
			c.t.Fatalf("%s received a message it must not see: %s", c.name, data)
		}
	}
}

func TestWatchPartyThroughTheGateway(t *testing.T) {
	gw := gatewayURL(t)
	aliceID, aliceTok := signup(t, gw, "alice")
	bobID, bobTok := signup(t, gw, "bob")
	room := fmt.Sprintf("e2e-%d", time.Now().UnixNano())

	// --- the gateway refuses handshakes without a valid token
	wsBase := "ws" + strings.TrimPrefix(gw, "http") + "/api/v1/watch-party/rooms/" + room + "/ws"
	for name, url := range map[string]string{"no token": wsBase, "garbage token": wsBase + "?token=abc.def.ghi", "tampered": wsBase + "?token=" + aliceTok[:len(aliceTok)-3] + "AAA"} {
		c, resp, err := websocket.DefaultDialer.Dial(url, nil)
		if c != nil {
			c.Close()
		}
		if err == nil || resp == nil || resp.StatusCode != 401 {
			t.Errorf("%s: want 401, got resp=%v err=%v", name, resp, err)
		}
	}

	// --- two real users in one room, a third connection (alice again) in another
	alice := browserDial(t, gw, room, aliceTok, "alice")
	bob := browserDial(t, gw, room, bobTok, "bob")
	if m := alice.expect("user_joined"); m["user_id"] != bobID {
		t.Fatalf("alice should learn that bob (%s) joined: %v", bobID, m)
	}
	elsewhere := browserDial(t, gw, room+"-other", aliceTok, "alice-elsewhere")

	// --- chat is attributed to the AUTHENTICATED user, whatever the payload claims
	_ = alice.conn.WriteJSON(map[string]any{"type": "chat_message", "text": "hello bob", "user_id": "admin"})
	m := bob.expect("chat_message")
	if m["user_id"] != aliceID || m["text"] != "hello bob" || m["room"] != room {
		t.Fatalf("bob got %v, want a message from %s", m, aliceID)
	}

	// --- playback sync goes to the other member, and back
	_ = bob.conn.WriteJSON(map[string]any{"type": "playback_sync", "action": "seek", "timestamp": 1234.5})
	m = alice.expect("playback_sync")
	if m["user_id"] != bobID || m["action"] != "seek" || m["timestamp"] != 1234.5 {
		t.Fatalf("alice got %v", m)
	}

	// --- the other room heard none of it
	elsewhere.noChat(300 * time.Millisecond)

	// --- rooms are torn down when everyone leaves
	if stats := os.Getenv("WATCHPARTY_STATS_URL"); stats != "" {
		alice.conn.Close()
		bob.conn.Close()
		elsewhere.conn.Close()
		deadline := time.Now().Add(5 * time.Second)
		for {
			r := call(t, "GET", stats, "", "")
			if !strings.Contains(r.body, room) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("room %s still present after everyone left: %s", room, r.body)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
