package poller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeRand struct{ v int }

func (f fakeRand) Intn(n int) int { return f.v % n }

func TestBackoffDelay(t *testing.T) {
	require.Equal(t, 5*time.Minute, BackoffDelay(1))
	require.Equal(t, 15*time.Minute, BackoffDelay(2))
	require.Equal(t, 30*time.Minute, BackoffDelay(3))
	require.Equal(t, 60*time.Minute, BackoffDelay(4))
	require.Equal(t, 60*time.Minute, BackoffDelay(100))
}

func TestNextCheckDelay_Delivered(t *testing.T) {
	d := NextCheckDelay("DELIVERED", 0, fakeRand{v: 0})
	require.Equal(t, 365*24*time.Hour, d)
}

func TestNextCheckDelay_InTransitRange(t *testing.T) {
	// min=30, max=120, fakeRand(0) => 30
	d := NextCheckDelay("IN_TRANSIT", 0, fakeRand{v: 0})
	require.Equal(t, 30*time.Minute, d)
}

func TestNextCheckDelay_Unknown(t *testing.T) {
	d := NextCheckDelay("UNKNOWN", 0, fakeRand{v: 0})
	require.Equal(t, 90*time.Minute, d)
}


