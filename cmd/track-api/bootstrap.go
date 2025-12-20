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
	"github.com/BearBump/TrackBox/internal/services/trackings"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
)

type trackAPIApp struct {
	ctx      context.Context
	cancel   context.CancelFunc
	opts     trackAPIOpts
	svc      *trackings.Service
	consumer *kafka.Consumer
	closeDB  func()
}

func mustBootstrapTrackAPI() *trackAPIApp {
	cfgPath := os.Getenv("configPath")
	if cfgPath == "" {
		panic("configPath env var is required")
	}
	swaggerPath := os.Getenv("swaggerPath")
	if swaggerPath == "" {
		panic("swaggerPath env var is required")
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		panic(fmt.Sprintf("ошибка парсинга конфига, %v", err))
	}

	grpcAddr := cfg.TrackBox.GRPCAddr
	if grpcAddr == "" {
		grpcAddr = ":50051"
	}
	httpAddr := cfg.TrackBox.HTTPAddr
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	consumerGroup := cfg.TrackBox.KafkaConsumerGroup
	if consumerGroup == "" {
		consumerGroup = "track-api"
	}
	topic := cfg.Kafka.TrackingUpdatedTopicName
	if topic == "" {
		topic = "tracking.updated"
	}

	cacheTTL := time.Duration(cfg.TrackBox.CurrentStatusTTLSeconds) * time.Second
	if cacheTTL <= 0 {
		cacheTTL = 10 * time.Minute
	}

	sslMode := cfg.Database.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.Username, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, sslMode)
	st := mustOpenPostgresWithRetry(connString, 60*time.Second)

	redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rc := rediscache.New(redisAddr)

	svc := trackings.New(st, rc, cacheTTL)

	brokers := []string{fmt.Sprintf("%s:%d", cfg.Kafka.Host, cfg.Kafka.Port)}
	consumer := kafka.NewConsumer(brokers, topic, consumerGroup)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	return &trackAPIApp{
		ctx:    ctx,
		cancel: cancel,
		opts: trackAPIOpts{
			grpcAddr:      grpcAddr,
			httpAddr:      httpAddr,
			grpcDialAddr:  grpcAddr,
			swaggerPath:   swaggerPath,
			topic:         topic,
			consumerGroup: consumerGroup,
		},
		svc:      svc,
		consumer: consumer,
		closeDB:  st.Close,
	}
}

func mustOpenPostgresWithRetry(connString string, wait time.Duration) *pgtracking.Storage {
	deadline := time.Now().Add(wait)
	var lastErr error
	for time.Now().Before(deadline) {
		st, err := pgtracking.New(connString)
		if err == nil {
			return st
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	panic(fmt.Sprintf("postgres is not ready after %s: %v", wait, lastErr))
}

func (a *trackAPIApp) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.closeDB != nil {
		a.closeDB()
	}
}

func (a *trackAPIApp) Run() error {
	return runTrackAPI(a.ctx, a.opts, a.svc, a.consumer)
}


