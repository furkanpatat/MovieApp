package rediscache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/rediscache"
)

func TestFreshAndStaleTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	c := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 24*time.Hour)
	ctx := context.Background()

	in := domain.Movie{ID: 7, Title: "Seven"}
	if err := c.Set(ctx, "k", in, time.Hour); err != nil {
		t.Fatal(err)
	}
	if ttl := mr.TTL("k"); ttl != time.Hour {
		t.Fatalf("fresh ttl = %v", ttl)
	}

	var out domain.Movie
	if hit, err := c.Get(ctx, "k", &out); err != nil || !hit || out.ID != in.ID || out.Title != in.Title {
		t.Fatalf("get: %v %v %+v", hit, err, out)
	}

	mr.FastForward(2 * time.Hour) // fresh copy expires, stale survives
	if hit, _ := c.Get(ctx, "k", &out); hit {
		t.Fatal("fresh entry should have expired")
	}
	out = domain.Movie{}
	if hit, err := c.GetStale(ctx, "k", &out); err != nil || !hit || out.ID != 7 {
		t.Fatalf("stale: %v %v %+v", hit, err, out)
	}
}

func TestMissAndCorrupt(t *testing.T) {
	mr := miniredis.RunT(t)
	c := rediscache.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), time.Hour)
	var m domain.Movie
	if hit, err := c.Get(context.Background(), "nope", &m); hit || err != nil {
		t.Fatalf("miss: %v %v", hit, err)
	}
	_ = mr.Set("bad", "{not json")
	if _, err := c.Get(context.Background(), "bad", &m); err == nil {
		t.Fatal("corrupt entry should error")
	}
}
