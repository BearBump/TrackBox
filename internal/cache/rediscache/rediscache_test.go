package rediscache

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestRedisCache_GetSet(t *testing.T) {
	mr := miniredis.RunT(t)
	c := New(mr.Addr())

	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "k", []byte("v"), time.Minute))

	b, ok, err := c.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []byte("v"), b)
}

func TestRateLimiter_Allow(t *testing.T) {
	mr := miniredis.RunT(t)
	rl := NewRateLimiter(mr.Addr())

	ctx := context.Background()
	ok, n, err := rl.Allow(ctx, "rl:test", 2, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(1), n)

	ok, n, _ = rl.Allow(ctx, "rl:test", 2, time.Minute)
	require.True(t, ok)
	require.Equal(t, int64(2), n)

	ok, n, _ = rl.Allow(ctx, "rl:test", 2, time.Minute)
	require.False(t, ok)
	require.Equal(t, int64(3), n)
}


