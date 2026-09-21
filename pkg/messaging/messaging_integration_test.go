package messaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/furkanpatat/movieapp/pkg/messaging"
)

// Integration tests need a live broker: RABBITMQ_URL=amqp://user:pass@localhost:5673/
// Optionally RABBITMQ_MGMT_URL=http://user:pass@localhost:15673 enables the reconnect test.
func setup(t *testing.T) (context.Context, *messaging.Connection) {
	t.Helper()
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	conn, err := messaging.Dial(ctx, messaging.Config{URL: url, InitialBackoff: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return ctx, conn
}

func uniq(p string) string { return fmt.Sprintf("%s.%d", p, time.Now().UnixNano()) }

func TestPublishConsumeAndDeadLetter(t *testing.T) {
	ctx, conn := setup(t)
	ex, q := uniq("test.ex"), uniq("test.q")
	pub := messaging.NewPublisher(conn, 3)
	if err := pub.DeclareExchange(ctx, ex, "topic"); err != nil {
		t.Fatal(err)
	}

	var okCount, failAttempts atomic.Int32
	cctx, stop := context.WithCancel(ctx)
	defer stop()
	go messaging.Consume(cctx, conn, messaging.ConsumerConfig{
		Exchange: ex, Queue: q, RoutingKeys: []string{"movie.#"},
		MaxRetries: 2, RetryDelay: 50 * time.Millisecond,
		Handler: func(_ context.Context, d amqp.Delivery) error {
			if string(d.Body) == "bad" {
				failAttempts.Add(1)
				return errors.New("boom")
			}
			okCount.Add(1)
			return nil
		},
	}, nil)

	// give the consumer time to declare topology
	time.Sleep(500 * time.Millisecond)
	if err := pub.Publish(ctx, ex, "movie.created", "text/plain", []byte("good"), nil); err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(ctx, ex, "movie.created", "text/plain", []byte("bad"), nil); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return okCount.Load() == 1 && failAttempts.Load() == 3 }, "handler calls (1 ok, 3 attempts)")

	ch, _ := conn.Channel(ctx)
	defer ch.Close()
	waitFor(t, func() bool {
		d, ok, _ := ch.Get(q+".dlq", true)
		return ok && string(d.Body) == "bad"
	}, "bad message in DLQ")
}

func TestReconnectAfterConnectionKilled(t *testing.T) {
	mgmt := os.Getenv("RABBITMQ_MGMT_URL")
	if mgmt == "" {
		t.Skip("RABBITMQ_MGMT_URL not set")
	}
	ctx, conn := setup(t)
	ex, q := uniq("test.ex"), uniq("test.q")
	pub := messaging.NewPublisher(conn, 5)
	_ = pub.DeclareExchange(ctx, ex, "direct")

	var got atomic.Int32
	cctx, stop := context.WithCancel(ctx)
	defer stop()
	go messaging.Consume(cctx, conn, messaging.ConsumerConfig{
		Exchange: ex, ExchangeKind: "direct", Queue: q, RoutingKeys: []string{"k"},
		Handler: func(context.Context, amqp.Delivery) error { got.Add(1); return nil },
	}, nil)
	time.Sleep(500 * time.Millisecond)

	// Force-close our client connections through the management API. The API
	// reports connections with a delay, so poll until ours shows up.
	killed := 0
	waitFor(t, func() bool {
		resp, err := http.Get(mgmt + "/api/connections")
		if err != nil {
			return false
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var conns []struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(b, &conns)
		for _, c := range conns {
			req, _ := http.NewRequest(http.MethodDelete, mgmt+"/api/connections/"+urlPath(c.Name), nil)
			if r, err := http.DefaultClient.Do(req); err == nil {
				r.Body.Close()
				killed++
			}
		}
		return killed > 0
	}, "connections visible in management API")
	waitFor(t, func() bool {
		return pub.Publish(ctx, ex, "k", "text/plain", []byte("after"), nil) == nil && got.Load() >= 1
	}, "publish + consume after reconnect")
}

func urlPath(s string) string {
	r := ""
	for _, c := range []byte(s) {
		if c == ' ' {
			r += "%20"
		} else {
			r += string(c)
		}
	}
	return r
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}
