package hub

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Options tunes the hub. Zero values get defaults.
type Options struct {
	MaxMessageBytes int64         // largest inbound frame (default 4096)
	MaxTextRunes    int           // longest chat text (default 500)
	SendBuffer      int           // per-client outbound queue; a client that falls this far behind is dropped (default 64)
	WriteWait       time.Duration // deadline for one write (default 10s)
	PongWait        time.Duration // drop a client that stays silent this long (default 60s)
	PingInterval    time.Duration // must be < PongWait (default 30s)
	MsgRate         float64       // sustained inbound messages/second per client (default 20)
	MsgBurst        int           // burst allowance (default 40)
	MaxRoomClients  int           // 0 = unlimited
	Logger          *slog.Logger
}

func (o *Options) defaults() {
	if o.MaxMessageBytes <= 0 {
		o.MaxMessageBytes = 4096
	}
	if o.MaxTextRunes <= 0 {
		o.MaxTextRunes = 500
	}
	if o.SendBuffer < 8 {
		o.SendBuffer = 64
	}
	if o.WriteWait <= 0 {
		o.WriteWait = 10 * time.Second
	}
	if o.PongWait <= 0 {
		o.PongWait = 60 * time.Second
	}
	if o.PingInterval <= 0 || o.PingInterval >= o.PongWait {
		o.PingInterval = o.PongWait / 2
	}
	if o.MsgRate <= 0 {
		o.MsgRate = 20
	}
	if o.MsgBurst <= 0 {
		o.MsgBurst = 40
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
}

// Stats is a point-in-time snapshot.
type Stats struct {
	Rooms   int            `json:"rooms"`
	Clients int            `json:"clients"`
	ByRoom  map[string]int `json:"by_room"`
}

type joinReq struct {
	room string
	c    *Client
	resp chan error
}

// Hub owns the room registry. A single goroutine (run) serialises every
// membership change, so "create on first join / destroy on last leave" cannot
// race: a join that arrives after the last leave simply creates a fresh room.
// Each Room then runs its own goroutine for message fan-out, so a busy room
// never slows another.
type Hub struct {
	opts Options
	log  *slog.Logger

	joins  chan joinReq
	leaves chan *Client
	stats  chan chan Stats

	quit chan struct{} // closed by Close
	done chan struct{} // closed when run exits

	mu       sync.RWMutex
	closing  bool
	sessions sync.WaitGroup // live Serve calls

	rooms map[string]*Room // owned by run()
}

// New starts a hub. Call Close to stop it.
func New(opts Options) *Hub {
	opts.defaults()
	h := &Hub{
		opts:   opts,
		log:    opts.Logger.With("component", "hub"),
		joins:  make(chan joinReq),
		leaves: make(chan *Client),
		stats:  make(chan chan Stats),
		quit:   make(chan struct{}),
		done:   make(chan struct{}),
		rooms:  make(map[string]*Room),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	defer close(h.done)
	for {
		select {
		case req := <-h.joins:
			req.resp <- h.handleJoin(req)

		case c := <-h.leaves:
			h.handleLeave(c)

		case reply := <-h.stats:
			s := Stats{Rooms: len(h.rooms), ByRoom: make(map[string]int, len(h.rooms))}
			for name, r := range h.rooms {
				s.ByRoom[name] = r.refs
				s.Clients += r.refs
			}
			reply <- s

		case <-h.quit:
			for name, r := range h.rooms {
				close(r.quit) // each room closes its clients' send queues
				delete(h.rooms, name)
			}
			return
		}
	}
}

func (h *Hub) handleJoin(req joinReq) error {
	r, ok := h.rooms[req.room]
	if ok && h.opts.MaxRoomClients > 0 && r.refs >= h.opts.MaxRoomClients {
		return ErrRoomFull
	}
	if !ok { // created dynamically by the first user to join
		r = newRoom(req.room, h.opts)
		h.rooms[req.room] = r
		go r.run()
		h.log.Debug("room created", "room", req.room)
	}
	r.refs++
	req.c.room = r
	r.register <- req.c // room.run never blocks, so this is prompt
	return nil
}

func (h *Hub) handleLeave(c *Client) {
	r := c.room
	r.unregister <- c // safe: the room is alive while refs > 0
	r.refs--
	if r.refs == 0 { // destroyed when the last user leaves
		delete(h.rooms, r.name)
		close(r.quit)
		h.log.Debug("room destroyed", "room", r.name)
	}
}

func (h *Hub) join(room string, c *Client) error {
	req := joinReq{room: room, c: c, resp: make(chan error, 1)}
	select {
	case h.joins <- req:
	case <-h.done:
		return ErrHubClosed
	}
	select {
	case err := <-req.resp:
		return err
	case <-h.done:
		return ErrHubClosed
	}
}

func (h *Hub) leave(c *Client) {
	select {
	case h.leaves <- c:
	case <-h.done:
	}
}

// Stats returns a snapshot of rooms and connected clients.
func (h *Hub) Stats(ctx context.Context) (Stats, error) {
	reply := make(chan Stats, 1)
	select {
	case h.stats <- reply:
	case <-h.done:
		return Stats{}, ErrHubClosed
	case <-ctx.Done():
		return Stats{}, ctx.Err()
	}
	select {
	case s := <-reply:
		return s, nil
	case <-h.done:
		return Stats{}, ErrHubClosed
	case <-ctx.Done():
		return Stats{}, ctx.Err()
	}
}

// Serve runs a client session on conn in room, as userID, and blocks until the
// connection ends. It always closes conn.
func (h *Hub) Serve(conn *websocket.Conn, room, userID string) {
	h.mu.RLock()
	if h.closing {
		h.mu.RUnlock()
		closeWith(conn, websocket.CloseGoingAway, "server shutting down", h.opts.WriteWait)
		return
	}
	h.sessions.Add(1)
	h.mu.RUnlock()
	defer h.sessions.Done()

	c := newClient(h, conn, userID)
	if err := h.join(room, c); err != nil {
		code, reason := websocket.CloseTryAgainLater, "room is full"
		if err == ErrHubClosed {
			code, reason = websocket.CloseGoingAway, "server shutting down"
		}
		closeWith(conn, code, reason, h.opts.WriteWait)
		return
	}
	h.log.Debug("client joined", "room", room, "user_id", userID)
	c.serve()
	h.log.Debug("client left", "room", room, "user_id", userID)
}

// Close stops accepting sessions and tells every client to go away.
// Use Wait to block until their goroutines have finished.
func (h *Hub) Close() {
	h.mu.Lock()
	first := !h.closing
	h.closing = true
	h.mu.Unlock()
	if first {
		close(h.quit)
	}
	<-h.done
}

// Wait blocks until all sessions have ended or ctx expires.
func (h *Hub) Wait(ctx context.Context) error {
	ended := make(chan struct{})
	go func() { h.sessions.Wait(); close(ended) }()
	select {
	case <-ended:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func closeWith(conn *websocket.Conn, code int, reason string, wait time.Duration) {
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(wait))
	_ = conn.Close()
}

// --- Room ---

type fanout struct {
	from     *Client
	data     []byte
	playback *Playback
}

// Room distributes messages among its clients. Its goroutine is the sole owner
// of clients and playback; it never blocks (sends to clients are non-blocking),
// so nothing a slow or dead client does can stall it.
type Room struct {
	name string
	opts Options

	refs int // owned by the hub goroutine: joined sessions that have not left

	register   chan *Client
	unregister chan *Client
	broadcast  chan fanout
	quit       chan struct{}

	clients  map[*Client]struct{} // owned by run()
	playback *Playback            // owned by run()
}

func newRoom(name string, opts Options) *Room {
	return &Room{
		name: name, opts: opts,
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan fanout, 64),
		quit:       make(chan struct{}),
		clients:    make(map[*Client]struct{}),
	}
}

func (r *Room) run() {
	for {
		select {
		case c := <-r.register:
			r.clients[c] = struct{}{}
			c.send <- roomState(r.name, r.participants(), r.playback) // fresh queue: cannot block
			r.deliver(presence(TypeUserJoined, r.name, c.userID, time.Now().UTC()), c)

		case c := <-r.unregister:
			if _, ok := r.clients[c]; ok { // not already evicted
				delete(r.clients, c)
				close(c.send)
				r.deliver(presence(TypeUserLeft, r.name, c.userID, time.Now().UTC()), nil)
			}

		case f := <-r.broadcast:
			if _, ok := r.clients[f.from]; !ok {
				continue // sender was evicted in the meantime
			}
			if f.playback != nil {
				r.playback = f.playback
			}
			r.deliver(f.data, f.from)

		case <-r.quit:
			for c := range r.clients {
				close(c.send)
			}
			return
		}
	}
}

// deliver queues data for every client except `except`. A client whose queue is
// full is evicted (its send channel is closed, ending its session) and its
// departure is announced in turn.
func (r *Room) deliver(data []byte, except *Client) {
	evicted := r.push(data, except)
	for len(evicted) > 0 {
		c := evicted[0]
		evicted = evicted[1:]
		evicted = append(evicted, r.push(presence(TypeUserLeft, r.name, c.userID, time.Now().UTC()), nil)...)
	}
}

func (r *Room) push(data []byte, except *Client) (evicted []*Client) {
	for c := range r.clients {
		if c == except {
			continue
		}
		select {
		case c.send <- data:
		default: // slow consumer: never wait for it
			delete(r.clients, c)
			close(c.send)
			evicted = append(evicted, c)
		}
	}
	return evicted
}

func (r *Room) participants() []string {
	seen := make(map[string]struct{}, len(r.clients))
	out := make([]string, 0, len(r.clients))
	for c := range r.clients {
		if _, dup := seen[c.userID]; !dup {
			seen[c.userID] = struct{}{}
			out = append(out, c.userID)
		}
	}
	sort.Strings(out)
	return out
}
