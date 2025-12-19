package track24http

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
		require.Equal(t, "/tracking.json.php", r.URL.Path)
		require.Equal(t, "demo", r.URL.Query().Get("apiKey"))
		require.Equal(t, "d", r.URL.Query().Get("domain"))
		require.Equal(t, "CODE", r.URL.Query().Get("code"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "status": "ok",
  "data": {
    "events": [
      {"operationDateTime":"01.01.2025 00:00:00","operationAttribute":"Accepted","operationType":"ACCEPTED","operationPlaceName":"Moscow","operationPlacePostalCode":"000000","source":"emulator"},
      {"operationDateTime":"01.01.2025 00:10:00","operationAttribute":"Delivered","operationType":"DELIVERED","operationPlaceName":"Moscow","operationPlacePostalCode":"000000","source":"emulator"}
    ]
  }
}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "demo", "d")
	res, err := c.GetTracking(context.Background(), "IGNORED", "CODE")
	require.NoError(t, err)
	require.Equal(t, "DELIVERED", res.Status)
	require.NotNil(t, res.StatusAt)
	require.Len(t, res.Events, 2)
	require.WithinDuration(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), res.Events[0].EventTime, time.Second)
}

func TestContainsDeliveredHint(t *testing.T) {
	require.True(t, containsDeliveredHint("Delivered"))
	require.True(t, containsDeliveredHint("Прибыло в место вручения"))
	require.False(t, containsDeliveredHint("In transit"))
}


