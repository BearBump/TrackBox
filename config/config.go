package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	Database               DatabaseConfig         `yaml:"database"`
	Kafka                  KafkaConfig            `yaml:"kafka"`
	Redis                  RedisConfig            `yaml:"redis"`
	TrackBox               TrackBoxConfig         `yaml:"trackbox"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DBName   string `yaml:"name"`
	SSLMode  string `yaml:"ssl_mode"`
}

type KafkaConfig struct {
	Host                       string `yaml:"host"`
	Port                       int    `yaml:"port"`
	TrackingUpdatedTopicName   string `yaml:"tracking_updated_topic_name"`
}

type RedisConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type TrackBoxConfig struct {
	GRPCAddr          string `yaml:"grpc_addr"`
	HTTPAddr          string `yaml:"http_addr"`
	KafkaConsumerGroup string `yaml:"kafka_consumer_group"`
	CurrentStatusTTLSeconds int `yaml:"current_status_ttl_seconds"`

	WorkerPollIntervalSeconds int `yaml:"worker_poll_interval_seconds"`
	WorkerBatchSize           int `yaml:"worker_batch_size"`
	WorkerConcurrency         int `yaml:"worker_concurrency"`
	WorkerLeaseSeconds        int `yaml:"worker_lease_seconds"`
	WorkerRateLimitPerMinute  int `yaml:"worker_rate_limit_per_minute"`
	WorkerRateLimitCDEKPerMinute   int `yaml:"worker_rate_limit_cdek_per_minute"`
	WorkerRateLimitPostRuPerMinute int `yaml:"worker_rate_limit_post_ru_per_minute"`

	WorkerHTTPAddr string `yaml:"worker_http_addr"`

	// Worker scheduling (optional). If not set, defaults are "prod-like" minutes/hours:
	// IN_TRANSIT: 30..120 minutes, UNKNOWN: 90 minutes, backoff: 5/15/30/60 minutes.
	WorkerNextCheckInTransitMinSeconds int `yaml:"worker_next_check_in_transit_min_seconds"`
	WorkerNextCheckInTransitMaxSeconds int `yaml:"worker_next_check_in_transit_max_seconds"`
	WorkerNextCheckUnknownSeconds      int `yaml:"worker_next_check_unknown_seconds"`
	WorkerBackoff1Seconds              int `yaml:"worker_backoff_1_seconds"`
	WorkerBackoff2Seconds              int `yaml:"worker_backoff_2_seconds"`
	WorkerBackoff3Seconds              int `yaml:"worker_backoff_3_seconds"`
	WorkerBackoff4Seconds              int `yaml:"worker_backoff_4_seconds"`

	CarrierEmulatorBaseURL string `yaml:"carrier_emulator_base_url"`
	CarrierEmulatorMode    string `yaml:"carrier_emulator_mode"` // "track24" | "gdeposylka"
	CarrierEmulatorAPIKey  string `yaml:"carrier_emulator_api_key"`
	CarrierEmulatorDomain  string `yaml:"carrier_emulator_domain"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return &config, nil
}
