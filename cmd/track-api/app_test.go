package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	trackingsapi "github.com/BearBump/TrackBox/internal/api/trackings_api"
	"github.com/BearBump/TrackBox/internal/models"
	"github.com/BearBump/TrackBox/internal/services/trackings"
	"github.com/BearBump/TrackBox/internal/storage/pgtracking"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct{}

func (r *fakeRepo) CreateOrGetTrackings(ctx context.Context, items []models.TrackingCreateInput) ([]*models.Tracking, error) {
	return []*models.Tracking{}, nil
}
func (r *fakeRepo) GetTrackingsByIDs(ctx context.Context, ids []uint64) ([]*models.Tracking, error) {
	return []*models.Tracking{}, nil
}
func (r *fakeRepo) ListTrackingEvents(ctx context.Context, trackingID uint64, limit, offset int) ([]*models.TrackingEvent, error) {
	return []*models.TrackingEvent{}, nil
}
func (r *fakeRepo) RefreshTracking(ctx context.Context, trackingID uint64) error { return nil }
func (r *fakeRepo) ApplyTrackingUpdate(ctx context.Context, upd pgtracking.TrackingUpdate) error {
	return nil
}

func TestRunServers_SwaggerServed(t *testing.T) {
	dir := t.TempDir()
	sw := filepath.Join(dir, "swagger.json")
	require.NoError(t, os.WriteFile(sw, []byte(`{"swagger":"2.0"}`), 0o600))

	svc := trackings.New(&fakeRepo{}, nil, time.Minute)
	api := trackingsapi.New(svc)

	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grpcErr := make(chan error, 1)
	go func() { grpcErr <- runGRPCServer(ctx, grpcLis, api) }()

	httpErr := make(chan error, 1)
	go func() { httpErr <- runGatewayServer(ctx, httpLis, grpcLis.Addr().String(), sw) }()

	// ждём, пока gateway поднимется (очень коротко)
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + httpLis.Addr().String() + "/swagger.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "\"swagger\"")

	cancel()

	select {
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting servers to stop")
	case <-grpcErr:
	}
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting gateway to stop")
	case <-httpErr:
	}
}

func TestRunTrackAPI_SwaggerServed(t *testing.T) {
	dir := t.TempDir()
	sw := filepath.Join(dir, "swagger.json")
	require.NoError(t, os.WriteFile(sw, []byte(`{"swagger":"2.0"}`), 0o600))

	svc := trackings.New(&fakeRepo{}, nil, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrCh := make(chan string, 1)

	opts := trackAPIOpts{
		grpcAddr:     "127.0.0.1:0",
		httpAddr:     "127.0.0.1:0",
		grpcDialAddr: "127.0.0.1:0", // будет подменён внутри runTrackAPI
		swaggerPath:  sw,
		topic:        "t",
		consumerGroup:"g",
		onListen: func(_grpcAddr, httpAddr string) { addrCh <- httpAddr },
	}

	cons := fakeConsumer{}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runTrackAPI(ctx, opts, svc, cons)
	}()

	httpAddr := <-addrCh

	resp, err := http.Get("http://" + httpAddr + "/swagger.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	cancel()
	require.Error(t, <-errCh)
}

type fakeConsumer struct{}

func (c fakeConsumer) Consume(ctx context.Context, handler func(key, value []byte) error) error {
	<-ctx.Done()
	return ctx.Err()
}


