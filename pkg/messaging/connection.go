// Package messaging wraps RabbitMQ (amqp091-go) with automatic reconnection,
// confirmed publishing and consumers backed by retry + dead-letter queues.
package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ErrClosed is returned once the Connection has been closed.
var ErrClosed = errors.New("messaging: connection closed")

// Config configures a Connection.
type Config struct {
	URL            string
	Logger         *slog.Logger
	InitialBackoff time.Duration // default 500ms
	MaxBackoff     time.Duration // default 30s
}

func (c *Config) defaults() {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.InitialBackoff <= 0 {
		c.InitialBackoff = 500 * time.Millisecond
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 30 * time.Second
	}
}

// Connection is a self-healing AMQP connection. After a drop it redials with
// exponential backoff and jitter; publishers and consumers built on it
// re-open their channels transparently.
type Connection struct {
	cfg Config
	log *slog.Logger

	mu    sync.RWMutex
	conn  *amqp.Connection
	ready chan struct{} // closed while connected, replaced while disconnected

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// Dial connects to the broker, retrying with backoff until it succeeds or ctx
// is cancelled, then keeps the connection alive in the background.
func Dial(ctx context.Context, cfg Config) (*Connection, error) {
	cfg.defaults()
	c := &Connection{
		cfg:   cfg,
		log:   cfg.Logger.With("component", "rabbitmq"),
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}

	conn, err := c.connectWithBackoff(ctx)
	if err != nil {
		return nil, err
	}
	c.setConn(conn)

	c.wg.Add(1)
	go c.supervise(conn)
	return c, nil
}

func (c *Connection) setConn(conn *amqp.Connection) {
	c.mu.Lock()
	c.conn = conn
	close(c.ready)
	c.mu.Unlock()
}

func (c *Connection) markDisconnected() {
	c.mu.Lock()
	c.conn = nil
	c.ready = make(chan struct{})
	c.mu.Unlock()
}

func (c *Connection) supervise(conn *amqp.Connection) {
	defer c.wg.Done()
	for {
		notify := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-c.done:
			return
		case amqpErr, ok := <-notify:
			if !ok || amqpErr == nil { // graceful close
				return
			}
			c.log.Warn("connection lost, reconnecting", "error", amqpErr)
			c.markDisconnected()

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				select {
				case <-c.done:
					cancel()
				case <-ctx.Done():
				}
			}()
			next, err := c.connectWithBackoff(ctx)
			cancel()
			if err != nil {
				return // closed while reconnecting
			}
			c.setConn(next)
			c.log.Info("reconnected")
			conn = next
		}
	}
}

func (c *Connection) connectWithBackoff(ctx context.Context) (*amqp.Connection, error) {
	backoff := c.cfg.InitialBackoff
	for attempt := 1; ; attempt++ {
		conn, err := amqp.Dial(c.cfg.URL)
		if err == nil {
			return conn, nil
		}
		c.log.Warn("dial failed", "attempt", attempt, "retry_in", backoff, "error", err)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("messaging: dial: %w", ctx.Err())
		case <-time.After(jitter(backoff)):
		}
		backoff = min(backoff*2, c.cfg.MaxBackoff)
	}
}

func jitter(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

// Channel opens a fresh channel, waiting for a live connection if necessary.
func (c *Connection) Channel(ctx context.Context) (*amqp.Channel, error) {
	for {
		c.mu.RLock()
		conn, ready := c.conn, c.ready
		c.mu.RUnlock()

		if conn != nil {
			ch, err := conn.Channel()
			if err == nil {
				return ch, nil
			}
			if !errors.Is(err, amqp.ErrClosed) {
				return nil, err
			}
			// connection died between the check and the call; wait for the swap
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-c.done:
				return nil, ErrClosed
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.done:
			return nil, ErrClosed
		}
	}
}

// Healthy reports whether the connection is currently up (for readiness probes).
func (c *Connection) Healthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && !c.conn.IsClosed()
}

// Close shuts the connection down and stops reconnecting.
func (c *Connection) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn != nil {
			err = conn.Close()
		}
		c.wg.Wait()
	})
	return err
}
