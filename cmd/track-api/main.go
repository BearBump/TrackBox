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

func main() {
	cfg, err := config.LoadConfig(os.Getenv("configPath"))
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
	st, err := pgtracking.New(connString)
	if err != nil {
		panic(err)
	}
	defer st.Close()

	redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rc := rediscache.New(redisAddr)

	svc := trackings.New(st, rc, cacheTTL)

	brokers := []string{fmt.Sprintf("%s:%d", cfg.Kafka.Host, cfg.Kafka.Port)}
	consumer := kafka.NewConsumer(brokers, topic, consumerGroup)
	defer func() { _ = consumer.Close() }()

	swaggerPath := os.Getenv("swaggerPath")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := runTrackAPI(ctx, trackAPIOpts{
		grpcAddr:      grpcAddr,
		httpAddr:      httpAddr,
		grpcDialAddr:  grpcAddr,
		swaggerPath:   swaggerPath,
		topic:         topic,
		consumerGroup: consumerGroup,
	}, svc, consumer); err != nil && err != context.Canceled {
		panic(err)
	}
}


