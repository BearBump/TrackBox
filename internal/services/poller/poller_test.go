package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	"trackbox/internal/integrations/carrier"
	"trackbox/internal/models"
	"github.com/stretchr/testify/require"
)

type fakeProducer struct {
	topic string
	key   []byte
	value []byte
	calls int
	err   error
}

func (p *fakeProducer) Publish(ctx context.Context, topic string, key, value []byte) error {
	p.calls++
	p.topic, p.key, p.value = topic, key, value
	return p.err
}

type fakeRL struct {
	allowed bool
	count   int64
	err     error
}

func (r fakeRL) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, int64, error) {
	return r.allowed, r.count, r.err
}

type fakeCarrier struct {
	res carrier.TrackingResult
	err error
}

func (c fakeCarrier) GetTracking(ctx context.Context, carrierCode, trackNumber string) (carrier.TrackingResult, error) {
	return c.res, c.err
}

func TestPoller_processOne_okPublishes(t *testing.T) {
	now := time.Now().UTC()
	fp := &fakeProducer{}
	p := New(nil, fakeCarrier{
		res: carrier.TrackingResult{
			Status:    "IN_TRANSIT",
			StatusRaw: "RAW",
			StatusAt:  &now,
			Events: []*models.TrackingEvent{
				{Status: "IN_TRANSIT", StatusRaw: "RAW", EventTime: now},
			},
		},
	}, fp, fakeRL{allowed: true}, "tracking.updated")

	tr := &models.Tracking{ID: 42, CarrierCode: "C", TrackNumber: "N", CheckFailCount: 0}
	require.NoError(t, p.processOne(context.Background(), tr))
	require.Equal(t, 1, fp.calls)
	require.Equal(t, "tracking.updated", fp.topic)
	require.NotEmpty(t, fp.value)
}

func TestPoller_processOne_errorBackoff(t *testing.T) {
	fp := &fakeProducer{}
	p := New(nil, fakeCarrier{err: errors.New("boom")}, fp, nil, "tracking.updated")
	tr := &models.Tracking{ID: 1, CarrierCode: "C", TrackNumber: "N", CheckFailCount: 2}
	require.NoError(t, p.processOne(context.Background(), tr))
	require.Equal(t, 1, fp.calls)
}

func TestPoller_WithSettings(t *testing.T) {
	fp := &fakeProducer{}
	p := New(nil, fakeCarrier{}, fp, nil, "t").
		WithSettings(5*time.Second, 7, 9, 11*time.Second, 13)
	require.Equal(t, 5*time.Second, p.pollInterval)
	require.Equal(t, 7, p.batchSize)
	require.Equal(t, 9, p.concurrency)
	require.Equal(t, 11*time.Second, p.lease)
	require.Equal(t, int64(13), p.rateLimitPerMinute)
}


