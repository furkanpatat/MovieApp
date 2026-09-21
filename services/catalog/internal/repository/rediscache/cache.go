// Package rediscache implements domain.Cache on Redis.
package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

const staleSuffix = ":stale"

type Cache struct {
	rdb      redis.Cmdable
	staleTTL time.Duration
}

var _ domain.Cache = (*Cache)(nil)

// New returns a Cache. staleTTL is how long the fallback copy outlives writes.
func New(rdb redis.Cmdable, staleTTL time.Duration) *Cache {
	return &Cache{rdb: rdb, staleTTL: staleTTL}
}

func (c *Cache) Get(ctx context.Context, key string, dst any) (bool, error) {
	return c.read(ctx, key, dst)
}

func (c *Cache) GetStale(ctx context.Context, key string, dst any) (bool, error) {
	return c.read(ctx, key+staleSuffix, dst)
}

func (c *Cache) read(ctx context.Context, key string, dst any) (bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return false, err // corrupt entry: treat as an error, caller falls through to TMDB
	}
	return true, nil
}

func (c *Cache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, key, b, ttl)
	pipe.Set(ctx, key+staleSuffix, b, max(c.staleTTL, ttl))
	_, err = pipe.Exec(ctx)
	return err
}
