package hub

import (
	"errors"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// Client is one WebSocket connection. Two goroutines serve it: readPump (the
// caller of Serve) and writePump. gorilla/websocket allows one concurrent
// reader and one concurrent writer, and these are exactly that; the only other
// write, a close frame, goes through WriteControl, which is safe to call
// concurrently.
type Client struct {
	hub    *Hub
	room   *Room // set by the hub goroutine before the join is acknowledged
	userID string
	conn   *websocket.Conn

	send    chan []byte   // closed only by the room, which owns it
	done    chan struct{} // closed when readPump ends
	limiter *rate.Limiter
}

func newClient(h *Hub, conn *websocket.Conn, userID string) *Client {
	return &Client{
		hub: h, conn: conn, userID: userID,
		send:    make(chan []byte, h.opts.SendBuffer),
		done:    make(chan struct{}),
		limiter: rate.NewLimiter(rate.Limit(h.opts.MsgRate), h.opts.MsgBurst),
	}
}

func (c *Client) serve() {
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		c.writePump()
	}()

	c.readPump()

	c.hub.leave(c) // room drops us and closes c.send (unless it already evicted us)
	close(c.done)
	<-writerDone
	_ = c.conn.Close()
}

func (c *Client) readPump() {
	o := c.hub.opts
	c.conn.SetReadLimit(o.MaxMessageBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(o.PongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(o.PongWait))
	})

	for {
		mt, data, err := c.conn.ReadMessage()
		if err != nil {
			// Close frames, resets, deadline expiry and oversize frames all end here.
			var ce *websocket.CloseError
			if errors.As(err, &ce) || websocket.IsUnexpectedCloseError(err) {
				c.hub.log.Debug("read ended", "user_id", c.userID, "error", err)
			}
			return
		}
		// Any frame proves the peer is alive; extend the deadline.
		_ = c.conn.SetReadDeadline(time.Now().Add(o.PongWait))

		if !c.limiter.Allow() {
			_ = c.conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "rate limit exceeded"),
				time.Now().Add(o.WriteWait))
			return
		}

		var (
			out []byte
			pb  *Playback
		)
		if mt != websocket.TextMessage {
			err = errBinary
		} else {
			out, pb, err = parse(data, c.userID, c.room.name, o.MaxTextRunes, time.Now().UTC())
		}
		if err != nil {
			var bad badMessage
			if errors.As(err, &bad) {
				c.reply(errorMessage(bad.reason))
				continue
			}
			c.hub.log.Error("encode message", "error", err)
			continue
		}

		select {
		case c.room.broadcast <- fanout{from: c, data: out, playback: pb}:
		case <-c.room.quit: // hub shutting down
			return
		}
	}
}

// reply queues a message for this client only; if its queue is full it is
// simply dropped (the room will evict a client that stays that far behind).
func (c *Client) reply(b []byte) {
	select {
	case c.send <- b:
	default:
	}
}

func (c *Client) writePump() {
	o := c.hub.opts
	ping := time.NewTicker(o.PingInterval)
	defer ping.Stop()
	defer c.conn.Close() // unblocks readPump if we die first

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(o.WriteWait))
			if !ok { // the room closed our queue: hub shutdown, eviction or normal leave
				_ = c.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(o.WriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}
