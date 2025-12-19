package poller

import (
	"context"
	"testing"
	"time"

	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	calls int
}

func (r *fakeRepo) ClaimDueTrackings(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]*models.Tracking, error) {
	r.calls++
	return []*models.Tracking{}, nil
}

type noopProducer struct{}

func (p noopProducer) Publish(ctx context.Context, topic string, key, value []byte) error { return nil }

type noopCarrier struct{}

func (c noopCarrier) GetTracking(ctx context.Context, carrierCode, trackNumber string) (carrier.TrackingResult, error) {
	return carrier.TrackingResult{}, nil
}

func TestPoller_Run_StopsOnContextCancel(t *testing.T) {
	repo := &fakeRepo{}
	p := New(repo, noopCarrier{}, noopProducer{}, nil, "t").WithSettings(5*time.Millisecond, 1, 1, 1*time.Second, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := p.Run(ctx)
	require.Error(t, err)
	require.GreaterOrEqual(t, repo.calls, 1)
}


