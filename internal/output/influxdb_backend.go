package output

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

// InfluxDBBackendConfig holds configuration for the InfluxDB backend
type InfluxDBBackendConfig struct {
	URL         string
	Token       string
	Org         string
	Bucket      string
	Measurement string // Default: "blood_pressure"
}

// InfluxDBBackend implements the Backend interface for InfluxDB output
type InfluxDBBackend struct {
	name     string
	config   InfluxDBBackendConfig
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
}

// NewInfluxDBBackend creates a new InfluxDB backend
func NewInfluxDBBackend(name string, config InfluxDBBackendConfig) (*InfluxDBBackend, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("influxdb url is required")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("influxdb token is required")
	}
	if config.Org == "" {
		return nil, fmt.Errorf("influxdb org is required")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("influxdb bucket is required")
	}
	if config.Measurement == "" {
		config.Measurement = "blood_pressure"
	}

	client := influxdb2.NewClient(config.URL, config.Token)
	writeAPI := client.WriteAPIBlocking(config.Org, config.Bucket)

	return &InfluxDBBackend{
		name:     name,
		config:   config,
		client:   client,
		writeAPI: writeAPI,
	}, nil
}

// Name returns the backend instance name
func (b *InfluxDBBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *InfluxDBBackend) Type() string {
	return "influxdb"
}

// Write writes records to InfluxDB
func (b *InfluxDBBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	for userIdx, records := range userRecords {
		for _, record := range records {
			point := influxdb2.NewPoint(
				b.config.Measurement,
				map[string]string{
					"device": metadata.DeviceName,
					"user":   fmt.Sprintf("user%d", userIdx+1),
				},
				map[string]interface{}{
					"systolic":  record.Systolic,
					"diastolic": record.Diastolic,
					"pulse":     record.Pulse,
					"mov":       boolToInt(record.Movement),
					"ihb":       boolToInt(record.IHB),
				},
				record.Timestamp,
			)

			if err := b.writeAPI.WritePoint(ctx, point); err != nil {
				return fmt.Errorf("failed to write point: %w", err)
			}
		}
	}
	return nil
}

// Health checks if the backend is operational
func (b *InfluxDBBackend) Health(ctx context.Context) error {
	health, err := b.client.Health(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping InfluxDB: %w", err)
	}
	if health.Status != "pass" {
		return fmt.Errorf("InfluxDB health check failed: %s", health.Status)
	}
	return nil
}

// Close releases resources
func (b *InfluxDBBackend) Close() error {
	b.client.Close()
	return nil
}

// RetryConfig returns retry configuration for InfluxDB
func (b *InfluxDBBackend) RetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

func init() {
	Register("influxdb", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		url, _ := settings["url"].(string)
		token, _ := settings["token"].(string)
		org, _ := settings["org"].(string)
		bucket, _ := settings["bucket"].(string)
		measurement, _ := settings["measurement"].(string)

		return NewInfluxDBBackend(cfg.Name, InfluxDBBackendConfig{
			URL:         url,
			Token:       token,
			Org:         org,
			Bucket:      bucket,
			Measurement: measurement,
		})
	})
}
