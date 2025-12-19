package rediscache

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	c *redis.Client
}

func NewRateLimiter(addr string) *RateLimiter {
	return &RateLimiter{
		c: redis.NewClient(&redis.Options{Addr: addr}),
	}
}

// Allow делает INCR по ключу и ставит TTL, если ключ создаётся впервые.
// Возвращает (allowed, currentCount).
func (rl *RateLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, int64, error) {
	pipe := rl.c.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, errors.Wrap(err, "redis ratelimit")
	}
	n := incr.Val()
	return n <= limit, n, nil
}


