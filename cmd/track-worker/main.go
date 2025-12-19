package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BearBump/TrackBox/config"
	"github.com/BearBump/TrackBox/internal/broker/kafka"
	"github.com/BearBump/TrackBox/internal/cache/rediscache"
	"github.com/BearBump/TrackBox/internal/integrations/carrier"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/fake"
	"github.com/BearBump/TrackBox/internal/integrations/carrier/track24http"
	"github.com/BearBump/TrackBox/internal/services/poller"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
)

func main() {
	cfg, err := config.LoadConfig(os.Getenv("configPath"))
	if err != nil {
		panic(fmt.Sprintf("ошибка парсинга конфига, %v", err))
	}

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

	sslMode := cfg.Database.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.Username, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, sslMode)
	st, err := pgtracking.New(connString)
	if err != nil {
		panic(err)
	}
	defer st.Close()

	brokers := []string{fmt.Sprintf("%s:%d", cfg.Kafka.Host, cfg.Kafka.Port)}
	producer := kafka.NewProducer(brokers)

	redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rl := rediscache.NewRateLimiter(redisAddr)

	// По умолчанию для демо используем эмулятор Track24 (Python), если задан base_url.
	// Иначе — fallback на локальный fake.
	var carrierClient carrier.Client
	if cfg.TrackBox.CarrierEmulatorBaseURL != "" && cfg.TrackBox.CarrierEmulatorMode != "" {
		switch cfg.TrackBox.CarrierEmulatorMode {
		case "track24":
			carrierClient = track24http.New(
				cfg.TrackBox.CarrierEmulatorBaseURL,
				cfg.TrackBox.CarrierEmulatorAPIKey,
				cfg.TrackBox.CarrierEmulatorDomain,
			)
		default:
			carrierClient = fake.New()
		}
	} else {
		carrierClient = fake.New()
	}

	p := poller.New(st, carrierClient, producer, rl, topic).
		WithSettings(pollInterval, batchSize, concurrency, lease, rlPerMin)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := p.Run(ctx); err != nil && err != context.Canceled {
		panic(err)
	}
}


