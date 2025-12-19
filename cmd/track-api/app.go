package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	trackingsapi "github.com/BearBump/TrackBox/internal/api/trackings_api"
	"github.com/BearBump/TrackBox/internal/broker/messages"
	"github.com/BearBump/TrackBox/internal/pb/trackings_api"
	"github.com/BearBump/TrackBox/internal/services/trackings"
	"github.com/go-chi/chi/v5"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	httpSwagger "github.com/swaggo/http-swagger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type trackAPIOpts struct {
	grpcAddr     string
	httpAddr     string
	grpcDialAddr string
	swaggerPath  string

	topic         string
	consumerGroup string

	onListen func(grpcAddr, httpAddr string)
}

type kafkaConsumer interface {
	Consume(ctx context.Context, handler func(key, value []byte) error) error
}

func runTrackAPI(ctx context.Context, opts trackAPIOpts, svc *trackings.Service, consumer kafkaConsumer) error {
	if opts.swaggerPath == "" {
		return fmt.Errorf("swaggerPath env var is required")
	}
	if _, err := os.Stat(opts.swaggerPath); os.IsNotExist(err) {
		return fmt.Errorf("swagger file not found: %s", opts.swaggerPath)
	}

	api := trackingsapi.New(svc)

	grpcLis, err := net.Listen("tcp", opts.grpcAddr)
	if err != nil {
		return err
	}
	httpLis, err := net.Listen("tcp", opts.httpAddr)
	if err != nil {
		_ = grpcLis.Close()
		return err
	}

	if opts.onListen != nil {
		opts.onListen(grpcLis.Addr().String(), httpLis.Addr().String())
	}

	dialAddr := opts.grpcDialAddr
	if dialAddr == "" || strings.HasSuffix(dialAddr, ":0") {
		dialAddr = grpcLis.Addr().String()
	}

	grpcErr := make(chan error, 1)
	go func() {
		grpcErr <- runGRPCServer(ctx, grpcLis, api)
	}()

	httpErr := make(chan error, 1)
	go func() {
		httpErr <- runGatewayServer(ctx, httpLis, dialAddr, opts.swaggerPath)
	}()

	go func() {
		slog.Info("kafka consumer started", "topic", opts.topic, "group", opts.consumerGroup)
		_ = consumer.Consume(ctx, func(_key, value []byte) error {
			var m messages.TrackingUpdated
			if err := json.Unmarshal(value, &m); err != nil {
				return err
			}
			return svc.ApplyKafkaUpdate(ctx, m)
		})
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-grpcErr:
		return err
	case err := <-httpErr:
		return err
	}
}

func runGRPCServer(ctx context.Context, lis net.Listener, api *trackingsapi.TrackingsAPI) error {
	s := grpc.NewServer()
	trackings_api.RegisterTrackingsServiceServer(s, api)

	go func() {
		<-ctx.Done()
		stopped := make(chan struct{})
		go func() {
			s.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			s.Stop()
		}
		_ = lis.Close()
	}()

	slog.Info("gRPC server listening", "addr", lis.Addr().String())
	return s.Serve(lis)
}

func runGatewayServer(ctx context.Context, lis net.Listener, grpcAddr string, swaggerPath string) error {
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

	srv := &http.Server{Handler: r}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("HTTP gateway listening", "addr", lis.Addr().String())
	return srv.Serve(lis)
}


