package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
			st, err := openPostgresWithRetry(connString, 60*time.Second)
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

func openPostgresWithRetry(connString string, wait time.Duration) (*pgtracking.Storage, error) {
	deadline := time.Now().Add(wait)
	var lastErr error
	for time.Now().Before(deadline) {
		st, err := pgtracking.New(connString)
		if err == nil {
			return st, nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	return nil, fmt.Errorf("postgres is not ready after %s: %w", wait, lastErr)
}

func RunTrackWorker(ctx context.Context, cfg *config.Config, f workerFactories) error {
	topic := cfg.Kafka.TrackingUpdatedTopicName
	if topic == "" {
		topic = "tracking.updated"
	}

	// Optional ops HTTP server (Swagger + health).
	workerHTTPAddr := cfg.TrackBox.WorkerHTTPAddr
	if workerHTTPAddr == "" {
		workerHTTPAddr = ":8082"
	}
	workerSwaggerPath := os.Getenv("swaggerPath")
	if workerSwaggerPath == "" {
		workerSwaggerPath = "/app/swagger.json"
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

	plannerCfg := poller.PlannerConfig{}
	if cfg.TrackBox.WorkerNextCheckInTransitMinSeconds > 0 {
		plannerCfg.InTransitMinDelay = time.Duration(cfg.TrackBox.WorkerNextCheckInTransitMinSeconds) * time.Second
	}
	if cfg.TrackBox.WorkerNextCheckInTransitMaxSeconds > 0 {
		plannerCfg.InTransitMaxDelay = time.Duration(cfg.TrackBox.WorkerNextCheckInTransitMaxSeconds) * time.Second
	}
	if cfg.TrackBox.WorkerNextCheckUnknownSeconds > 0 {
		plannerCfg.UnknownDelay = time.Duration(cfg.TrackBox.WorkerNextCheckUnknownSeconds) * time.Second
	}
	if cfg.TrackBox.WorkerBackoff1Seconds > 0 {
		plannerCfg.Backoff1 = time.Duration(cfg.TrackBox.WorkerBackoff1Seconds) * time.Second
	}
	if cfg.TrackBox.WorkerBackoff2Seconds > 0 {
		plannerCfg.Backoff2 = time.Duration(cfg.TrackBox.WorkerBackoff2Seconds) * time.Second
	}
	if cfg.TrackBox.WorkerBackoff3Seconds > 0 {
		plannerCfg.Backoff3 = time.Duration(cfg.TrackBox.WorkerBackoff3Seconds) * time.Second
	}
	if cfg.TrackBox.WorkerBackoff4Seconds > 0 {
		plannerCfg.Backoff4 = time.Duration(cfg.TrackBox.WorkerBackoff4Seconds) * time.Second
	}

	p := poller.New(repo, carrierClient, producer, rl, topic).
		WithSettings(pollInterval, batchSize, concurrency, lease, rlPerMin).
		WithPlanner(plannerCfg).
		WithCarrierRateLimits(cfg.TrackBox.WorkerRateLimitCDEKPerMinute, cfg.TrackBox.WorkerRateLimitPostRuPerMinute)

	go func() {
		if err := runWorkerHTTPServer(ctx, workerHTTPOpts{
			httpAddr:    workerHTTPAddr,
			swaggerPath: workerSwaggerPath,
			poller:      p,
			cfg:         cfg,
		}); err != nil && err != context.Canceled {
			slog.Error("worker http server stopped", "error", err.Error())
		}
	}()

	return p.Run(ctx)
}


