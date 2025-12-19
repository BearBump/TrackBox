package main

import (
	"context"
	"fmt"
	"time"

	"github.com/BearBump/TrackBox/config"
	"github.com/BearBump/TrackBox/internal/broker/kafka"
	"github.com/BearBump/TrackBox/internal/cache/rediscache"
	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/emulatorv1"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/fake"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/track24http"
	"github.com/BearBump/TrackBox/internal/services/poller"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
)

type workerFactories struct {
	newStorage func(cfg *config.Config) (repo poller.Repository, closeFn func(), err error)
	newProducer func(cfg *config.Config) poller.Producer
	newRateLimiter func(cfg *config.Config) poller.RateLimiter
	newCarrierClient func(cfg *config.Config) carrier.Client
}

func defaultWorkerFactories() workerFactories {
	return workerFactories{
		newStorage: func(cfg *config.Config) (poller.Repository, func(), error) {
			sslMode := cfg.Database.SSLMode
			if sslMode == "" {
				sslMode = "disable"
			}
			connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
				cfg.Database.Username, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, sslMode)
			st, err := pgtracking.New(connString)
			if err != nil {
				return nil, nil, err
			}
			return st, st.Close, nil
		},
		newProducer: func(cfg *config.Config) poller.Producer {
			brokers := []string{fmt.Sprintf("%s:%d", cfg.Kafka.Host, cfg.Kafka.Port)}
			return kafka.NewProducer(brokers)
		},
		newRateLimiter: func(cfg *config.Config) poller.RateLimiter {
			redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
			return rediscache.NewRateLimiter(redisAddr)
		},
		newCarrierClient: func(cfg *config.Config) carrier.Client {
			// По умолчанию для демо используем python carrier-emulator, если задан base_url.
			// Иначе — fallback на локальный fake.
			if cfg.TrackBox.CarrierEmulatorBaseURL != "" && cfg.TrackBox.CarrierEmulatorMode != "" {
				switch cfg.TrackBox.CarrierEmulatorMode {
				case "v1":
					return emulatorv1.New(cfg.TrackBox.CarrierEmulatorBaseURL, cfg.TrackBox.CarrierEmulatorAPIKey)
				case "track24":
					return track24http.New(cfg.TrackBox.CarrierEmulatorBaseURL, cfg.TrackBox.CarrierEmulatorAPIKey, cfg.TrackBox.CarrierEmulatorDomain)
				default:
					return fake.New()
				}
			}
			return fake.New()
		},
	}
}

func RunTrackWorker(ctx context.Context, cfg *config.Config, f workerFactories) error {
	topic := cfg.Kafka.TrackingUpdatedTopicName
	if topic == "" {
		topic = "tracking.updated"
	}

	pollInterval := time.Duration(cfg.TrackBox.WorkerPollIntervalSeconds) * time.Second
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	batchSize := cfg.TrackBox.WorkerBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	concurrency := cfg.TrackBox.WorkerConcurrency
	if concurrency <= 0 {
		concurrency = 10
	}
	lease := time.Duration(cfg.TrackBox.WorkerLeaseSeconds) * time.Second
	if lease <= 0 {
		lease = 120 * time.Second
	}
	rlPerMin := int64(cfg.TrackBox.WorkerRateLimitPerMinute)
	if rlPerMin <= 0 {
		rlPerMin = 120
	}

	repo, closeFn, err := f.newStorage(cfg)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	producer := f.newProducer(cfg)
	rl := f.newRateLimiter(cfg)
	carrierClient := f.newCarrierClient(cfg)

	p := poller.New(repo, carrierClient, producer, rl, topic).
		WithSettings(pollInterval, batchSize, concurrency, lease, rlPerMin).
		WithCarrierRateLimits(cfg.TrackBox.WorkerRateLimitCDEKPerMinute, cfg.TrackBox.WorkerRateLimitPostRuPerMinute)

	return p.Run(ctx)
}


