package emulatorv1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_GetTracking_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tracking/CDEK/123", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "carrier": "CDEK",
  "track_number": "123",
  "status": "IN_TRANSIT",
  "status_raw": "raw",
  "status_at": "2025-01-01T00:00:00Z",
  "events": [{"status":"IN_TRANSIT","status_raw":"raw","event_time":"2025-01-01T00:00:00Z"}]
}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k")
	res, err := c.GetTracking(context.Background(), "CDEK", "123")
	require.NoError(t, err)
	require.Equal(t, "IN_TRANSIT", res.Status)
	require.NotNil(t, res.StatusAt)
	require.WithinDuration(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), *res.StatusAt, time.Second)
	require.Len(t, res.Events, 1)
}

func TestClient_GetTracking_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer srv.Close()

	c := New(srv.URL, "k")
	_, err := c.GetTracking(context.Background(), "CDEK", "123")
	require.Error(t, err)
}


