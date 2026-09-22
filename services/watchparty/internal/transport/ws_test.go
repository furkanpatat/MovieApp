package transport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/furkanpatat/movieapp/services/watchparty/internal/hub"
	"github.com/furkanpatat/movieapp/services/watchparty/internal/transport"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type env struct {
	hub *hub.Hub
	srv *httptest.Server
}

func newEnv(t *testing.T, opts hub.Options, origins ...string) *env {
	t.Helper()
	opts.Logger = quiet
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	h := hub.New(opts)
	srv := httptest.NewServer(transport.NewHandler(h, origins, quiet))
	t.Cleanup(func() {
		h.Close()
		srv.Close()
	})
	return &env{h, srv}
}

// fast returns options with the per-client rate limit out of the way.
func fast(o hub.Options) hub.Options {
	o.MsgRate, o.MsgBurst = 100000, 100000
	return o
}

func (e *env) url(room string) string {
	return "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/v1/watch-party/rooms/" + room + "/ws"
}

type client struct {
	t    *testing.T
	conn *websocket.Conn
	user string
}

// join connects as user to room and consumes the room_state greeting.
func (e *env) join(t *testing.T, room, user string) *client {
	t.Helper()
	h := http.Header{}
	h.Set("X-User-Id", user)
	conn, resp, err := websocket.DefaultDialer.Dial(e.url(room), h)
	if err != nil {
		t.Fatalf("dial %s/%s: %v (resp %v)", room, user, err, resp)
	}
	c := &client{t: t, conn: conn, user: user}
	t.Cleanup(func() { conn.Close() })
	st := c.expect("room_state")
	if st["room"] != room {
		t.Fatalf("room_state for %v, want %s", st["room"], room)
	}
	return c
}

func (c *client) send(v any) {
	c.t.Helper()
	b, _ := json.Marshal(v)
	if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		c.t.Fatalf("%s send: %v", c.user, err)
	}
}

func (c *client) chat(text string) { c.send(map[string]any{"type": "chat_message", "text": text}) }
func (c *client) sync(action string, ts float64) {
	c.send(map[string]any{"type": "playback_sync", "action": action, "timestamp": ts})
}

// expect reads until a message of the wanted type arrives (skipping presence
// events), failing on timeout.
func (c *client) expect(typ string) map[string]any {
	c.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_ = c.conn.SetReadDeadline(deadline)
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.t.Fatalf("%s waiting for %q: %v", c.user, typ, err)
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["type"] == typ {
			return m
		}
	}
}

// silent asserts no message of the given types arrives within d.
func (c *client) silent(d time.Duration, types ...string) {
	c.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(deadline)
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return // timeout (or closed): nothing arrived
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		for _, ty := range types {
			if m["type"] == ty {
				c.t.Fatalf("%s unexpectedly received %v", c.user, m)
			}
		}
	}
}

func (e *env) stats(t *testing.T) hub.Stats {
	t.Helper()
	s, err := e.hub.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *env) waitStats(t *testing.T, what string, cond func(hub.Stats) bool) hub.Stats {
	t.Helper()
	var s hub.Stats
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if s = e.stats(t); cond(s) {
			return s
		}
	}
	t.Fatalf("timed out waiting for %s; stats=%+v", what, s)
	return s
}

// ------------------------------------------------------------------ broadcast

func TestChatBroadcastsToOthersInTheRoomWithSenderIdentity(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a, b, c := e.join(t, "movie-night", "alice"), e.join(t, "movie-night", "bob"), e.join(t, "movie-night", "carol")

	a.chat("hello everyone")
	for _, rcv := range []*client{b, c} {
		m := rcv.expect("chat_message")
		if m["user_id"] != "alice" || m["text"] != "hello everyone" || m["room"] != "movie-night" || m["sent_at"] == nil {
			t.Fatalf("%s got %v", rcv.user, m)
		}
	}
	a.silent(200*time.Millisecond, "chat_message") // the sender does not get an echo
}

func TestPlaybackSyncBroadcast(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a, b := e.join(t, "r", "alice"), e.join(t, "r", "bob")

	a.sync("seek", 754.25)
	m := b.expect("playback_sync")
	if m["user_id"] != "alice" || m["action"] != "seek" || m["timestamp"] != 754.25 {
		t.Fatalf("%v", m)
	}
	b.sync("pause", 760)
	if m := a.expect("playback_sync"); m["user_id"] != "bob" || m["action"] != "pause" {
		t.Fatalf("%v", m)
	}
}

func TestSpoofedIdentityInPayloadIsIgnored(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a, b := e.join(t, "r", "alice"), e.join(t, "r", "bob")
	a.send(map[string]any{"type": "chat_message", "text": "hi", "user_id": "admin", "room": "elsewhere"})
	m := b.expect("chat_message")
	if m["user_id"] != "alice" || m["room"] != "r" {
		t.Fatalf("payload identity was trusted: %v", m)
	}
}

func TestPresenceEventsAndLateJoinerState(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a := e.join(t, "r", "alice")
	a.sync("seek", 100)
	a.sync("play", 100)

	// bob joins late: he learns who is here and where playback is.
	h := http.Header{}
	h.Set("X-User-Id", "bob")
	conn, _, err := websocket.DefaultDialer.Dial(e.url("r"), h)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	b := &client{t: t, conn: conn, user: "bob"}
	st := b.expect("room_state")
	parts, _ := st["participants"].([]any)
	if len(parts) != 2 || parts[0] != "alice" || parts[1] != "bob" {
		t.Fatalf("participants %v", st["participants"])
	}
	pb, _ := st["playback"].(map[string]any)
	if pb == nil || pb["action"] != "play" || pb["timestamp"] != 100.0 || pb["user_id"] != "alice" {
		t.Fatalf("late joiner should get the last playback state, got %v", st["playback"])
	}

	if m := a.expect("user_joined"); m["user_id"] != "bob" {
		t.Fatalf("%v", m)
	}
	conn.Close()
	if m := a.expect("user_left"); m["user_id"] != "bob" {
		t.Fatalf("%v", m)
	}
}

// ------------------------------------------------------------------ isolation

// drain collects every chat/sync message a client receives for d.
func (c *client) drain(d time.Duration) []map[string]any {
	c.t.Helper()
	var out []map[string]any
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(deadline)
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["type"] == "chat_message" || m["type"] == "playback_sync" {
			out = append(out, m)
		}
	}
	return out
}

func TestRoomsAreCompletelyIsolated(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a1, a2 := e.join(t, "room-a", "alice"), e.join(t, "room-a", "amy")
	b1, b2 := e.join(t, "room-b", "bob"), e.join(t, "room-b", "ben")
	// The same user identity present in BOTH rooms with two connections: each
	// connection belongs to exactly one room.
	zedA, zedB := e.join(t, "room-a", "zed"), e.join(t, "room-b", "zed")

	a1.chat("secret of room A")
	a1.sync("seek", 42)
	b1.chat("secret of room B")
	b1.sync("pause", 7)

	type want struct {
		c     *client
		room  string
		count int // chat + sync from the other member(s) of its own room
	}
	for _, w := range []want{{a2, "room-a", 2}, {zedA, "room-a", 2}, {b2, "room-b", 2}, {zedB, "room-b", 2}} {
		got := w.c.drain(400 * time.Millisecond)
		if len(got) != w.count {
			t.Fatalf("%s (%s) received %d messages, want %d: %v", w.c.user, w.room, len(got), w.count, got)
		}
		for _, m := range got {
			if m["room"] != w.room {
				t.Fatalf("%s (%s) received a message from %v: %v", w.c.user, w.room, m["room"], m)
			}
			if txt, _ := m["text"].(string); txt != "" && txt != "secret of "+w.room[len("room-"):] && txt != "secret of room "+strings.ToUpper(w.room[len("room-"):]) {
				t.Fatalf("%s (%s) received foreign text %q", w.c.user, w.room, txt)
			}
		}
	}
	// Senders never get their own traffic back.
	if got := a1.drain(150 * time.Millisecond); len(got) != 0 {
		t.Fatalf("alice got %v", got)
	}
	if got := b1.drain(150 * time.Millisecond); len(got) != 0 {
		t.Fatalf("bob got %v", got)
	}
}

func TestPlaybackStateDoesNotLeakBetweenRooms(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a := e.join(t, "room-a", "alice")
	a.sync("seek", 999)
	a.chat("sync me") // ordering barrier: room-a has processed the seek
	other := e.join(t, "room-a", "amy")
	_ = other

	h := http.Header{}
	h.Set("X-User-Id", "bob")
	conn, _, err := websocket.DefaultDialer.Dial(e.url("room-b"), h)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	st := (&client{t: t, conn: conn, user: "bob"}).expect("room_state")
	if st["playback"] != nil {
		t.Fatalf("room-b inherited room-a's playback: %v", st["playback"])
	}
}

// ------------------------------------------------------------------ lifecycle

func TestRoomsAreCreatedOnFirstJoinAndDestroyedOnLastLeave(t *testing.T) {
	e := newEnv(t, hub.Options{})
	if s := e.stats(t); s.Rooms != 0 || s.Clients != 0 {
		t.Fatalf("starts empty: %+v", s)
	}

	a := e.join(t, "r1", "alice")
	if s := e.stats(t); s.Rooms != 1 || s.ByRoom["r1"] != 1 {
		t.Fatalf("first join creates the room: %+v", s)
	}
	b := e.join(t, "r1", "bob")
	c := e.join(t, "r2", "carol")
	if s := e.stats(t); s.Rooms != 2 || s.Clients != 3 || s.ByRoom["r1"] != 2 {
		t.Fatalf("%+v", s)
	}

	a.conn.Close()
	if s := e.waitStats(t, "alice to leave", func(s hub.Stats) bool { return s.ByRoom["r1"] == 1 }); s.Rooms != 2 {
		t.Fatalf("room survives while someone remains: %+v", s)
	}
	b.conn.Close()
	e.waitStats(t, "r1 destroyed", func(s hub.Stats) bool { _, ok := s.ByRoom["r1"]; return !ok })
	c.conn.Close()
	e.waitStats(t, "all rooms destroyed", func(s hub.Stats) bool { return s.Rooms == 0 && s.Clients == 0 })

	// A recreated room starts fresh: no stale playback.
	x := e.join(t, "r1", "xavier")
	x.sync("seek", 5)
	x.chat("barrier")
	x.conn.Close()
	e.waitStats(t, "r1 destroyed again", func(s hub.Stats) bool { return s.Rooms == 0 })
	h := http.Header{}
	h.Set("X-User-Id", "yan")
	conn, _, _ := websocket.DefaultDialer.Dial(e.url("r1"), h)
	defer conn.Close()
	if st := (&client{t: t, conn: conn, user: "yan"}).expect("room_state"); st["playback"] != nil {
		t.Fatalf("a re-created room kept old state: %v", st["playback"])
	}
}

func TestSameUserMayHaveSeveralConnections(t *testing.T) {
	e := newEnv(t, hub.Options{})
	tab1, tab2 := e.join(t, "r", "alice"), e.join(t, "r", "alice")
	obs := e.join(t, "r", "bob")
	tab1.chat("from tab 1")
	tab2.expect("chat_message")
	obs.expect("chat_message")
	if s := e.stats(t); s.Clients != 3 {
		t.Fatalf("%+v", s)
	}
}

// ------------------------------------------------------------------ robustness

func TestClientDisconnectsNeverStallOrCrashTheRoom(t *testing.T) {
	// This test is about surviving abrupt victim disconnects, not backpressure
	// eviction (that's TestSlowConsumerIsEvictedWithoutBlockingOthers), so the
	// buffer must comfortably outrun 200 unpaced sends even when the survivor's
	// reader goroutine gets briefly descheduled on a slow/shared CI runner.
	e := newEnv(t, fast(hub.Options{SendBuffer: 1024}))
	sender := e.join(t, "r", "sender")
	survivor := e.join(t, "r", "survivor")
	var victims []*client
	for i := 0; i < 5; i++ {
		victims = append(victims, e.join(t, "r", fmt.Sprintf("victim%d", i)))
	}

	// Drain the survivor concurrently and count chat messages.
	var got atomic.Int32
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			_ = survivor.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, data, err := survivor.conn.ReadMessage()
			if err != nil {
				return
			}
			if strings.Contains(string(data), `"chat_message"`) {
				if got.Add(1) == 200 {
					return
				}
			}
		}
	}()

	// Kill victims abruptly (no close frame) while messages are flowing.
	for i := 0; i < 200; i++ {
		sender.chat(fmt.Sprintf("m%d", i))
		if i%40 == 0 && len(victims) > 0 {
			_ = victims[0].conn.UnderlyingConn().Close()
			victims = victims[1:]
		}
	}

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatalf("room stalled: survivor received %d/200", got.Load())
	}
	if got.Load() != 200 {
		t.Fatalf("survivor received %d/200 messages", got.Load())
	}
	e.waitStats(t, "dead clients removed", func(s hub.Stats) bool { return s.Clients <= 2+len(victims) })
}

func TestSlowConsumerIsEvictedWithoutBlockingOthers(t *testing.T) {
	e := newEnv(t, fast(hub.Options{SendBuffer: 8, WriteWait: 300 * time.Millisecond, MaxMessageBytes: 1 << 20, MaxTextRunes: 200000}))
	sender := e.join(t, "r", "sender")
	fastC := e.join(t, "r", "fast")
	slow := e.join(t, "r", "slow") // never reads from now on
	_ = slow

	const n = 300
	payload := strings.Repeat("x", 60000)
	var got atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for got.Load() < n {
			_ = fastC.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, data, err := fastC.conn.ReadMessage()
			if err != nil {
				return
			}
			if strings.Contains(string(data), `"chat_message"`) {
				got.Add(1)
			}
		}
	}()
	for i := 0; i < n; i++ {
		sender.chat(payload)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("fast client only got %d/%d: one slow client blocked the room", got.Load(), n)
	}
	if got.Load() != n {
		t.Fatalf("fast client got %d/%d", got.Load(), n)
	}
	e.waitStats(t, "slow consumer evicted", func(s hub.Stats) bool { return s.Clients == 2 })
}

func TestManyConcurrentClientsJoinChatAndLeaveWithoutDeadlockOrPanic(t *testing.T) {
	e := newEnv(t, fast(hub.Options{}))
	base := runtime.NumGoroutine()

	const clients, rooms = 60, 6
	var wg sync.WaitGroup
	var received atomic.Int64
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := http.Header{}
			h.Set("X-User-Id", fmt.Sprintf("user%d", i))
			conn, _, err := websocket.DefaultDialer.Dial(e.url(fmt.Sprintf("room%d", i%rooms)), h)
			if err != nil {
				t.Errorf("dial: %v", err)
				return
			}
			// reader
			stop := make(chan struct{})
			readerDone := make(chan struct{})
			go func() {
				defer close(readerDone)
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
					received.Add(1)
				}
			}()
			r := rand.New(rand.NewPCG(uint64(i), 1))
			end := time.Now().Add(1500 * time.Millisecond)
			for time.Now().Before(end) {
				switch r.IntN(3) {
				case 0:
					_ = conn.WriteJSON(map[string]any{"type": "chat_message", "text": "hi"})
				case 1:
					_ = conn.WriteJSON(map[string]any{"type": "playback_sync", "action": "seek", "timestamp": r.Float64() * 100})
				default:
					_ = conn.WriteMessage(websocket.TextMessage, []byte(`garbage`))
				}
				time.Sleep(time.Duration(r.IntN(5)) * time.Millisecond)
			}
			close(stop)
			if i%2 == 0 { // half leave abruptly, half politely
				_ = conn.UnderlyingConn().Close()
			} else {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
				conn.Close()
			}
			<-readerDone
		}(i)
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("clients hung: deadlock")
	}

	e.waitStats(t, "everything cleaned up", func(s hub.Stats) bool { return s.Rooms == 0 && s.Clients == 0 })
	if received.Load() == 0 {
		t.Fatal("no broadcasts were delivered")
	}
	waitFor(t, "goroutines to wind down", func() bool { return runtime.NumGoroutine() <= base+6 })
}

func TestInvalidMessagesGetAnErrorAndTheConnectionSurvives(t *testing.T) {
	e := newEnv(t, hub.Options{})
	a, b := e.join(t, "r", "alice"), e.join(t, "r", "bob")

	for name, raw := range map[string]string{
		"garbage":       `{{{`,
		"unknown type":  `{"type":"kick"}`,
		"bad action":    `{"type":"playback_sync","action":"warp","timestamp":1}`,
		"missing ts":    `{"type":"playback_sync","action":"play"}`,
		"empty chat":    `{"type":"chat_message","text":""}`,
		"too long chat": `{"type":"chat_message","text":"` + strings.Repeat("a", 501) + `"}`,
	} {
		_ = a.conn.WriteMessage(websocket.TextMessage, []byte(raw))
		if m := a.expect("error"); m["message"] == nil || m["message"] == "" {
			t.Fatalf("%s: %v", name, m)
		}
	}
	// binary frames are refused politely too
	_ = a.conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})
	a.expect("error")

	// none of that reached bob, and alice can carry on
	a.chat("still here")
	if m := b.expect("chat_message"); m["text"] != "still here" {
		t.Fatalf("bob saw %v (invalid input must never be broadcast)", m)
	}
}

func TestOversizedFrameClosesTheConnection(t *testing.T) {
	e := newEnv(t, hub.Options{MaxMessageBytes: 256})
	a := e.join(t, "r", "alice")
	_ = a.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"chat_message","text":"`+strings.Repeat("a", 1000)+`"}`))
	_ = a.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, _, err := a.conn.ReadMessage()
		if err == nil {
			continue
		}
		if ce, ok := err.(*websocket.CloseError); !ok || ce.Code != websocket.CloseMessageTooBig {
			t.Fatalf("want close 1009, got %v", err)
		}
		break
	}
	e.waitStats(t, "oversize client removed", func(s hub.Stats) bool { return s.Clients == 0 })
}

func TestFloodingClientIsDisconnected(t *testing.T) {
	e := newEnv(t, hub.Options{MsgRate: 5, MsgBurst: 5})
	a, b := e.join(t, "r", "alice"), e.join(t, "r", "bob")
	for i := 0; i < 60; i++ {
		_ = a.conn.WriteJSON(map[string]any{"type": "chat_message", "text": "spam"})
	}
	_ = a.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var code int
	for {
		_, _, err := a.conn.ReadMessage()
		if err == nil {
			continue
		}
		if ce, ok := err.(*websocket.CloseError); ok {
			code = ce.Code
		}
		break
	}
	if code != websocket.ClosePolicyViolation {
		t.Fatalf("close code %d, want 1008", code)
	}
	if m := b.expect("user_left"); m["user_id"] != "alice" {
		t.Fatalf("%v", m)
	}
}

func TestRoomFullIsRejectedWithTryAgainLater(t *testing.T) {
	e := newEnv(t, hub.Options{MaxRoomClients: 2})
	e.join(t, "r", "a")
	e.join(t, "r", "b")
	h := http.Header{}
	h.Set("X-User-Id", "c")
	conn, _, err := websocket.DefaultDialer.Dial(e.url("r"), h)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = conn.ReadMessage()
	if ce, ok := err.(*websocket.CloseError); !ok || ce.Code != websocket.CloseTryAgainLater {
		t.Fatalf("want close 1013, got %v", err)
	}
	if s := e.stats(t); s.Clients != 2 {
		t.Fatalf("%+v", s)
	}
	// another room is unaffected
	e.join(t, "other", "c")
}

func TestServerPingsKeepIdleClientsAliveAndSilentOnesGetDropped(t *testing.T) {
	e := newEnv(t, hub.Options{PongWait: 400 * time.Millisecond, PingInterval: 100 * time.Millisecond})
	// alive: gorilla's default ping handler answers automatically as long as we keep reading
	alive := e.join(t, "r", "alive")
	go func() {
		for {
			if _, _, err := alive.conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	// dead: TCP connection open, but never reads (so never pongs)
	e.join(t, "r", "dead")

	e.waitStats(t, "silent client dropped by the read deadline", func(s hub.Stats) bool { return s.Clients == 1 })
	time.Sleep(600 * time.Millisecond)
	if s := e.stats(t); s.Clients != 1 {
		t.Fatalf("the responsive client must stay connected: %+v", s)
	}
}

func TestShutdownClosesEveryClientAndWaitReturns(t *testing.T) {
	e := newEnv(t, hub.Options{})
	var cs []*client
	for i := 0; i < 10; i++ {
		cs = append(cs, e.join(t, fmt.Sprintf("r%d", i%3), fmt.Sprintf("u%d", i)))
	}
	e.hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.hub.Wait(ctx); err != nil {
		t.Fatalf("sessions did not finish: %v", err)
	}
	for _, c := range cs {
		_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			if _, _, err := c.conn.ReadMessage(); err != nil {
				break
			}
		}
	}
	// new connections are refused politely and nothing panics
	h := http.Header{}
	h.Set("X-User-Id", "late")
	if conn, _, err := websocket.DefaultDialer.Dial(e.url("r0"), h); err == nil {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, rerr := conn.ReadMessage()
		if ce, ok := rerr.(*websocket.CloseError); !ok || ce.Code != websocket.CloseGoingAway {
			t.Fatalf("want 1001, got %v", rerr)
		}
		conn.Close()
	}
	if _, err := e.hub.Stats(context.Background()); err == nil {
		t.Fatal("stats on a closed hub should error, not hang")
	}
}

// ------------------------------------------------------------------ handshake

func TestHandshakeRules(t *testing.T) {
	e := newEnv(t, hub.Options{}, "https://app.example.com")
	dial := func(room string, hdr map[string]string) (*http.Response, error) {
		h := http.Header{}
		for k, v := range hdr {
			h.Set(k, v)
		}
		conn, resp, err := websocket.DefaultDialer.Dial(e.url(room), h)
		if conn != nil {
			conn.Close()
		}
		return resp, err
	}
	expectStatus := func(name string, want int, room string, hdr map[string]string) {
		t.Helper()
		resp, err := dial(room, hdr)
		if want == http.StatusSwitchingProtocols {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			return
		}
		if err == nil || resp == nil || resp.StatusCode != want {
			t.Errorf("%s: want %d, got resp=%v err=%v", name, want, resp, err)
		}
	}

	expectStatus("no identity", 401, "r", nil)
	expectStatus("blank identity", 401, "r", map[string]string{"X-User-Id": "  "})
	expectStatus("unsafe identity", 401, "r", map[string]string{"X-User-Id": "bad id!"})
	expectStatus("bad room", 400, "bad$room", map[string]string{"X-User-Id": "alice"})
	expectStatus("long room", 400, strings.Repeat("a", 65), map[string]string{"X-User-Id": "alice"})
	expectStatus("ok, no origin (non-browser)", 101, "r", map[string]string{"X-User-Id": "alice"})
	expectStatus("ok, allowed origin", 101, "r", map[string]string{"X-User-Id": "alice", "Origin": "https://app.example.com"})
	expectStatus("foreign origin", 403, "r", map[string]string{"X-User-Id": "alice", "Origin": "https://evil.example.net"})

	// a plain HTTP GET (no upgrade) is rejected, not hung
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/watch-party/rooms/r/ws", nil)
	req.Header.Set("X-User-Id", "alice")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-upgrade GET -> %d, want 400", resp.StatusCode)
	}
	// rejected handshakes must not leave rooms behind
	if s := e.stats(t); s.Rooms != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestPeerThatVanishesMidHandshakeIsHarmless(t *testing.T) {
	e := newEnv(t, hub.Options{})
	raw, err := net.Dial("tcp", strings.TrimPrefix(e.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = raw.Write([]byte("GET /api/v1/watch-party/rooms/r/ws HTTP/1.1\r\nHost: x\r\nX-User-Id: alice\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"))
	raw.Close() // hang up right after the request, never reading the 101
	e.waitStats(t, "no stuck client", func(s hub.Stats) bool { return s.Clients == 0 && s.Rooms == 0 })
	// the service still works
	a, b := e.join(t, "r", "alice"), e.join(t, "r", "bob")
	a.chat("ok")
	b.expect("chat_message")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}
