// Package ratelimit is a Redis-backed sliding-window rate limiter.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Sliding-window log: every allowed request is a ZSET member scored by its
// timestamp; entries older than the window are trimmed, and the request is
// allowed while fewer than `limit` remain. Unlike a fixed window there is no
// burst of 2x limit at a window boundary. Redis TIME is the clock so all
// gateway replicas agree, and the whole thing is one atomic script.
var script = redis.NewScript(`
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
local count = redis.call('ZCARD', KEYS[1])
if count < limit then
  redis.call('ZADD', KEYS[1], now, ARGV[3])
  redis.call('PEXPIRE', KEYS[1], window)
  return {1, limit - count - 1, 0}
end
local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
local retry = tonumber(oldest[2]) + window - now
if retry < 1 then retry = 1 end
return {0, 0, retry}`)

type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration // set when !Allowed
}

type Limiter struct {
	rdb    redis.Scripter
	limit  int
	window time.Duration
	prefix string
}

// New allows `limit` requests per `window` per key.
func New(rdb redis.Scripter, limit int, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, limit: limit, window: window, prefix: "ratelimit:"}
}

func (l *Limiter) Allow(ctx context.Context, key string) (Result, error) {
	raw, err := script.Run(ctx, l.rdb, []string{l.prefix + key}, l.window.Milliseconds(), l.limit, uuid.NewString()).Int64Slice()
	if err != nil {
		return Result{}, err
	}
	if len(raw) != 3 {
		return Result{}, fmt.Errorf("ratelimit: unexpected script reply %v", raw)
	}
	return Result{
		Allowed:    raw[0] == 1,
		Limit:      l.limit,
		Remaining:  int(raw[1]),
		RetryAfter: time.Duration(raw[2]) * time.Millisecond,
	}, nil
}

// MiddlewareConfig configures Middleware.
type MiddlewareConfig struct {
	KeyPrefix         string // separates buckets of different limiters (default "ip:")
	FailOpen          bool   // serve requests if Redis is unavailable
	TrustForwardedFor bool
	Log               *slog.Logger
}

// Middleware limits requests per client IP, answering 429 with Retry-After.
func Middleware(l *Limiter, cfg MiddlewareConfig) func(http.Handler) http.Handler {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "ip:"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, err := l.Allow(r.Context(), prefix+ClientIP(r, cfg.TrustForwardedFor))
			if err != nil {
				log.Error("rate limiter unavailable", "error", err, "fail_open", cfg.FailOpen)
				if cfg.FailOpen {
					next.ServeHTTP(w, r)
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, "service temporarily unavailable")
				return
			}
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			if !res.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(res.RetryAfter.Seconds()))))
				writeJSON(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP returns the caller's address. X-Forwarded-For is honoured only when
// trusted, and then the RIGHTMOST entry is used: that is the one appended by
// our own proxy, whereas everything to its left is client-controlled.
func ClientIP(r *http.Request, trustForwardedFor bool) string {
	if trustForwardedFor {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}` + "\n"))
}
