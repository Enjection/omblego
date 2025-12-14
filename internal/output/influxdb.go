package output

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"omblego/internal/device"
)

// InfluxDBConfig holds InfluxDB connection settings
type InfluxDBConfig struct {
	URL    string
	Token  string
	Org    string
	Bucket string
}

// InfluxDBWriter writes blood pressure records to InfluxDB
type InfluxDBWriter struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	config   InfluxDBConfig
}

// NewInfluxDBWriter creates a new InfluxDB writer
func NewInfluxDBWriter(config InfluxDBConfig) *InfluxDBWriter {
	client := influxdb2.NewClient(config.URL, config.Token)
	writeAPI := client.WriteAPIBlocking(config.Org, config.Bucket)

	return &InfluxDBWriter{
		client:   client,
		writeAPI: writeAPI,
		config:   config,
	}
}

// Close closes the InfluxDB client
func (w *InfluxDBWriter) Close() {
	w.client.Close()
}

// WriteRecords writes blood pressure records to InfluxDB
func (w *InfluxDBWriter) WriteRecords(ctx context.Context, records [][]device.BloodPressureRecord, deviceName string) error {
	for userIdx, userRecords := range records {
		for _, record := range userRecords {
			// Create a point with blood pressure data
			point := influxdb2.NewPoint(
				"blood_pressure",
				map[string]string{
					"device": deviceName,
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

			if err := w.writeAPI.WritePoint(ctx, point); err != nil {
				return fmt.Errorf("failed to write point: %w", err)
			}
		}
	}

	return nil
}

// WriteLatestRecord writes only the most recent record to InfluxDB
// Useful for daemon mode to avoid duplicate writes
func (w *InfluxDBWriter) WriteLatestRecord(ctx context.Context, records [][]device.BloodPressureRecord, deviceName string) error {
	var latestRecord *device.BloodPressureRecord
	var latestUser int

	for userIdx, userRecords := range records {
		for i := range userRecords {
			if latestRecord == nil || userRecords[i].Timestamp.After(latestRecord.Timestamp) {
				latestRecord = &userRecords[i]
				latestUser = userIdx
			}
		}
	}

	if latestRecord == nil {
		return nil // No records
	}

	point := influxdb2.NewPoint(
		"blood_pressure",
		map[string]string{
			"device": deviceName,
			"user":   fmt.Sprintf("user%d", latestUser+1),
		},
		map[string]interface{}{
			"systolic":  latestRecord.Systolic,
			"diastolic": latestRecord.Diastolic,
			"pulse":     latestRecord.Pulse,
			"mov":       boolToInt(latestRecord.Movement),
			"ihb":       boolToInt(latestRecord.IHB),
		},
		latestRecord.Timestamp,
	)

	if err := w.writeAPI.WritePoint(ctx, point); err != nil {
		return fmt.Errorf("failed to write point: %w", err)
	}

	return nil
}

// Ping checks if InfluxDB is reachable
func (w *InfluxDBWriter) Ping(ctx context.Context) error {
	health, err := w.client.Health(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping InfluxDB: %w", err)
	}
	if health.Status != "pass" {
		return fmt.Errorf("InfluxDB health check failed: %s", health.Status)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SentTracker keeps track of which records have been sent (in-memory)
// Note: InfluxDB deduplicates points with same timestamp+tags automatically
type SentTracker struct {
	lastSentTimestamp time.Time
}

// NewSentTracker creates a new tracker
func NewSentTracker() *SentTracker {
	return &SentTracker{}
}

// FilterNewRecords returns only records newer than the last sent
func (t *SentTracker) FilterNewRecords(records [][]device.BloodPressureRecord) [][]device.BloodPressureRecord {
	result := make([][]device.BloodPressureRecord, len(records))

	for userIdx, userRecords := range records {
		var newRecords []device.BloodPressureRecord
		for _, record := range userRecords {
			if record.Timestamp.After(t.lastSentTimestamp) {
				newRecords = append(newRecords, record)
			}
		}
		result[userIdx] = newRecords
	}

	return result
}

// UpdateLastSent updates the last sent timestamp
func (t *SentTracker) UpdateLastSent(records [][]device.BloodPressureRecord) {
	for _, userRecords := range records {
		for _, record := range userRecords {
			if record.Timestamp.After(t.lastSentTimestamp) {
				t.lastSentTimestamp = record.Timestamp
			}
		}
	}
}
