package main

import (
	"context"
	"testing"
	"time"

	"github.com/BearBump/TrackBox/config"
	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/emulatorv1"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/fake"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/track24http"
	"github.com/BearBump/TrackBox/internal/services/poller"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct{}

func (r *fakeRepo) ClaimDueTrackings(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]*models.Tracking, error) {
	return []*models.Tracking{}, nil
}

type noopProducer struct{}

func (p noopProducer) Publish(ctx context.Context, topic string, key, value []byte) error { return nil }

func TestDefaultWorkerFactories_SelectCarrierClient(t *testing.T) {
	f := defaultWorkerFactories()

	cfgV1 := &config.Config{
		TrackBox: config.TrackBoxConfig{
			CarrierEmulatorBaseURL: "http://localhost:9000",
			CarrierEmulatorMode:    "v1",
			CarrierEmulatorAPIKey:  "k",
		},
	}
	c1 := f.newCarrierClient(cfgV1)
	_, ok := c1.(*emulatorv1.Client)
	require.True(t, ok)

	cfgT24 := &config.Config{
		TrackBox: config.TrackBoxConfig{
			CarrierEmulatorBaseURL: "http://localhost:9000",
			CarrierEmulatorMode:    "track24",
			CarrierEmulatorAPIKey:  "k",
			CarrierEmulatorDomain:  "d",
		},
	}
	c2 := f.newCarrierClient(cfgT24)
	_, ok = c2.(*track24http.Client)
	require.True(t, ok)

	cfgFallback := &config.Config{
		TrackBox: config.TrackBoxConfig{
			CarrierEmulatorBaseURL: "http://localhost:9000",
			CarrierEmulatorMode:    "unknown",
		},
	}
	c3 := f.newCarrierClient(cfgFallback)
	_, ok = c3.(*fake.FakeClient)
	require.True(t, ok)
}

func TestDefaultWorkerFactories_ProducerAndRateLimiter_NonNil(t *testing.T) {
	f := defaultWorkerFactories()
	cfg := &config.Config{
		Kafka: config.KafkaConfig{Host: "localhost", Port: 9092},
		Redis: config.RedisConfig{Host: "localhost", Port: 6379},
	}
	require.NotNil(t, f.newProducer(cfg))
	require.NotNil(t, f.newRateLimiter(cfg))
}

func TestRunTrackWorker_ContextCanceled(t *testing.T) {
	calledClose := false

	f := workerFactories{
		newStorage: func(cfg *config.Config) (repo poller.Repository, closeFn func(), err error) {
			return &fakeRepo{}, func() { calledClose = true }, nil
		},
		newProducer: func(cfg *config.Config) poller.Producer {
			return noopProducer{}
		},
		newRateLimiter: func(cfg *config.Config) poller.RateLimiter {
			return nil
		},
		newCarrierClient: func(cfg *config.Config) carrier.Client {
			return fake.New() // не будет вызываться, т.к. контекст отменён
		},
	}

	cfg := &config.Config{
		Kafka:    config.KafkaConfig{TrackingUpdatedTopicName: "t"},
		TrackBox: config.TrackBoxConfig{WorkerPollIntervalSeconds: 1},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RunTrackWorker(ctx, cfg, f)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, calledClose)
}


