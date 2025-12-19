package rediscache

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	c *redis.Client
}

func New(addr string) *RedisCache {
	return &RedisCache{
		c: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := r.c.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.Wrap(err, "redis get")
	}
	return val, true, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := r.c.Set(ctx, key, value, ttl).Err(); err != nil {
		return errors.Wrap(err, "redis set")
	}
	return nil
}


