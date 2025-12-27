package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/BearBump/TrackBox/config"
	"github.com/BearBump/TrackBox/internal/services/poller"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
)

type workerHTTPOpts struct {
	httpAddr   string
	swaggerPath string
	onListen   func(httpAddr string)

	poller *poller.Poller
	cfg    *config.Config
}

func runWorkerHTTPServer(ctx context.Context, opts workerHTTPOpts) error {
	if opts.httpAddr == "" {
		opts.httpAddr = ":8082"
	}
	if opts.swaggerPath == "" {
		return fmt.Errorf("worker swaggerPath env var is required")
	}
	if _, err := os.Stat(opts.swaggerPath); os.IsNotExist(err) {
		return fmt.Errorf("worker swagger file not found: %s", opts.swaggerPath)
	}

	lis, err := net.Listen("tcp", opts.httpAddr)
	if err != nil {
		return err
	}
	if opts.onListen != nil {
		opts.onListen(lis.Addr().String())
	}

	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if opts.poller == nil {
			_, _ = w.Write([]byte(`{"error":"poller not wired"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(opts.poller.Stats())
	})

	r.Get("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if opts.cfg == nil {
			_, _ = w.Write([]byte(`{"error":"config not wired"}`))
			return
		}
		// Avoid dumping secrets; show only operational worker settings.
		out := map[string]any{
			"pollIntervalSeconds": opts.cfg.TrackBox.WorkerPollIntervalSeconds,
			"batchSize":           opts.cfg.TrackBox.WorkerBatchSize,
			"concurrency":         opts.cfg.TrackBox.WorkerConcurrency,
			"leaseSeconds":        opts.cfg.TrackBox.WorkerLeaseSeconds,
			"rateLimitPerMinute":  opts.cfg.TrackBox.WorkerRateLimitPerMinute,
			"rateLimitCDEKPerMinute":   opts.cfg.TrackBox.WorkerRateLimitCDEKPerMinute,
			"rateLimitPostRuPerMinute": opts.cfg.TrackBox.WorkerRateLimitPostRuPerMinute,
			"nextCheckInTransitMinSeconds": opts.cfg.TrackBox.WorkerNextCheckInTransitMinSeconds,
			"nextCheckInTransitMaxSeconds": opts.cfg.TrackBox.WorkerNextCheckInTransitMaxSeconds,
			"nextCheckUnknownSeconds":      opts.cfg.TrackBox.WorkerNextCheckUnknownSeconds,
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	r.Post("/trigger", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if opts.poller == nil {
			_, _ = w.Write([]byte(`{"error":"poller not wired"}`))
			return
		}
		opts.poller.Trigger()
		_, _ = w.Write([]byte(`{"triggered":true}`))
	})

	// Serve swagger with no-cache + cachebuster (same trick as track-api).
	r.Get("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, opts.swaggerPath)
	})

	swaggerURL := "/swagger.json"
	if fi, err := os.Stat(opts.swaggerPath); err == nil {
		swaggerURL = fmt.Sprintf("/swagger.json?v=%d", fi.ModTime().Unix())
	}
	r.Get("/docs/*", httpSwagger.Handler(httpSwagger.URL(swaggerURL)))

	srv := &http.Server{Handler: r}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		_ = lis.Close()
	}()

	return srv.Serve(lis)
}


