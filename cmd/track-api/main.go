package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/BearBump/TrackBox/config"
	trackingsapi "github.com/BearBump/TrackBox/internal/api/trackings_api"
	"github.com/BearBump/TrackBox/internal/broker/kafka"
	"github.com/BearBump/TrackBox/internal/broker/messages"
	"github.com/BearBump/TrackBox/internal/cache/rediscache"
	"github.com/BearBump/TrackBox/internal/pb/trackings_api"
	"github.com/BearBump/TrackBox/internal/services/trackings"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
	"github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	httpSwagger "github.com/swaggo/http-swagger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	api := trackingsapi.New(svc)

	brokers := []string{fmt.Sprintf("%s:%d", cfg.Kafka.Host, cfg.Kafka.Port)}
	consumer := kafka.NewConsumer(brokers, topic, consumerGroup)
	defer func() { _ = consumer.Close() }()

	go func() {
		slog.Info("kafka consumer started", "topic", topic, "group", consumerGroup)
		err := consumer.Consume(context.Background(), func(_key, value []byte) error {
			var m messages.TrackingUpdated
			if err := json.Unmarshal(value, &m); err != nil {
				return err
			}
			return svc.ApplyKafkaUpdate(context.Background(), m)
		})
		if err != nil {
			slog.Error("kafka consumer stopped", "error", err.Error())
			// MVP: падаем — в docker это даст рестарт.
			os.Exit(1)
		}
	}()

	go func() {
		if err := runGRPCServer(grpcAddr, api); err != nil {
			panic(fmt.Errorf("failed to run gRPC server: %v", err))
		}
	}()

	if err := runGatewayServer(httpAddr, grpcAddr); err != nil {
		panic(fmt.Errorf("failed to run gateway server: %v", err))
	}
}

func runGRPCServer(addr string, api *trackingsapi.TrackingsAPI) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s := grpc.NewServer()
	trackings_api.RegisterTrackingsServiceServer(s, api)

	slog.Info("gRPC server listening", "addr", addr)
	return s.Serve(lis)
}

func runGatewayServer(httpAddr, grpcAddr string) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	swaggerPath := os.Getenv("swaggerPath")
	if _, err := os.Stat(swaggerPath); os.IsNotExist(err) {
		panic(fmt.Errorf("swagger file not found: %s", swaggerPath))
	}

	r := chi.NewRouter()
	r.Get("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, swaggerPath)
	})

	r.Get("/docs/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger.json"),
	))

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	if err := trackings_api.RegisterTrackingsServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts); err != nil {
		return err
	}

	r.Mount("/", mux)

	slog.Info("HTTP gateway listening", "addr", httpAddr)
	return http.ListenAndServe(httpAddr, r)
}


