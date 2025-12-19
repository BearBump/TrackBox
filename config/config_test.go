package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`
database:
  host: "localhost"
  port: 5432
  username: "u"
  password: "p"
  name: "db"
kafka:
  host: "localhost"
  port: 9092
  tracking_updated_topic_name: "tracking.updated"
redis:
  host: "localhost"
  port: 6379
trackbox:
  grpc_addr: ":50051"
  http_addr: ":8080"
  kafka_consumer_group: "track-api"
  current_status_ttl_seconds: 600
`), 0o600))

	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	require.Equal(t, "u", cfg.Database.Username)
	require.Equal(t, "tracking.updated", cfg.Kafka.TrackingUpdatedTopicName)
	require.Equal(t, 6379, cfg.Redis.Port)
	require.Equal(t, ":8080", cfg.TrackBox.HTTPAddr)
}


