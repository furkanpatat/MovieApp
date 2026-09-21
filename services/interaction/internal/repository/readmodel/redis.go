// Package readmodel is the Redis materialised view served by the query side.
//
// Layout per movie:
//
//	interaction:{id}:rating    HASH  v (aggregate version), sum, count   <- existence marks "model built"
//	interaction:{id}:comments  ZSET  member = comment JSON, score = unix ms
package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// applyRating: only touches an existing model, and never moves the aggregate backwards.
var applyRating = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
local cur = tonumber(redis.call('HGET', KEYS[1], 'v'))
if cur and cur > tonumber(ARGV[1]) then return 1 end
redis.call('HSET', KEYS[1], 'v', ARGV[1], 'sum', ARGV[2], 'count', ARGV[3])
return 1`)

// initRating: creates the model, but never overwrites a newer aggregate.
var initRating = redis.NewScript(`
local cur = redis.call('HGET', KEYS[1], 'v')
if cur and tonumber(cur) > tonumber(ARGV[1]) then return 0 end
redis.call('HSET', KEYS[1], 'v', ARGV[1], 'sum', ARGV[2], 'count', ARGV[3])
return 1`)

// addComment: only touches an existing model; ZADD of an identical member is a no-op,
// so redelivery is harmless. Trims to the newest ARGV[3] entries.
var addComment = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[2])
redis.call('ZREMRANGEBYRANK', KEYS[2], 0, -(tonumber(ARGV[3]) + 1))
return 1`)

type Model struct {
	rdb  redis.Cmdable
	keep int // comments retained per movie
}

var _ domain.ReadModel = (*Model)(nil)

func New(rdb redis.Cmdable, keepComments int) *Model {
	if keepComments < 1 {
		keepComments = 20
	}
	return &Model{rdb: rdb, keep: keepComments}
}

func ratingKey(id int) string   { return fmt.Sprintf("interaction:%d:rating", id) }
func commentsKey(id int) string { return fmt.Sprintf("interaction:%d:comments", id) }

func (m *Model) Get(ctx context.Context, movieID, limit int) (domain.Interactions, bool, error) {
	if limit < 1 || limit > m.keep {
		limit = m.keep
	}
	pipe := m.rdb.Pipeline()
	h := pipe.HGetAll(ctx, ratingKey(movieID))
	z := pipe.ZRevRange(ctx, commentsKey(movieID), 0, int64(limit-1))
	if _, err := pipe.Exec(ctx); err != nil {
		return domain.Interactions{}, false, err
	}

	fields := h.Val()
	if len(fields) == 0 {
		return domain.Interactions{}, false, nil
	}
	sum, err1 := strconv.ParseInt(fields["sum"], 10, 64)
	count, err2 := strconv.ParseInt(fields["count"], 10, 64)
	if err1 != nil || err2 != nil {
		return domain.Interactions{}, false, errors.New("corrupt rating hash")
	}
	out := domain.Interactions{
		MovieID:        movieID,
		AverageRating:  domain.RatingStats{TotalScore: sum, VoteCount: count}.Average(),
		TotalVotes:     count,
		RecentComments: make([]domain.Comment, 0, len(z.Val())),
	}
	for _, raw := range z.Val() {
		var c domain.Comment
		if json.Unmarshal([]byte(raw), &c) == nil {
			out.RecentComments = append(out.RecentComments, c)
		}
	}
	return out, true, nil
}

func (m *Model) ApplyRating(ctx context.Context, s domain.RatingStats) (bool, error) {
	n, err := applyRating.Run(ctx, m.rdb, []string{ratingKey(s.MovieID)}, s.Version, s.TotalScore, s.VoteCount).Int()
	return n == 1, err
}

func (m *Model) AddComment(ctx context.Context, movieID int, c domain.Comment) (bool, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return false, err
	}
	n, err := addComment.Run(ctx, m.rdb,
		[]string{ratingKey(movieID), commentsKey(movieID)}, c.CreatedAt.UnixMilli(), string(b), m.keep).Int()
	return n == 1, err
}

// Init writes comments first and the rating hash last: the hash is the
// "model exists" marker, so readers never see a marker without its comments.
func (m *Model) Init(ctx context.Context, s domain.RatingStats, recent []domain.Comment) error {
	if len(recent) > 0 {
		pipe := m.rdb.Pipeline()
		zs := make([]redis.Z, 0, len(recent))
		for _, c := range recent {
			b, err := json.Marshal(c)
			if err != nil {
				return err
			}
			zs = append(zs, redis.Z{Score: float64(c.CreatedAt.UnixMilli()), Member: string(b)})
		}
		pipe.ZAdd(ctx, commentsKey(s.MovieID), zs...)
		pipe.ZRemRangeByRank(ctx, commentsKey(s.MovieID), 0, int64(-(m.keep + 1)))
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return initRating.Run(ctx, m.rdb, []string{ratingKey(s.MovieID)}, s.Version, s.TotalScore, s.VoteCount).Err()
}
