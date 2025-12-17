package output

import (
	"context"
	"time"

	"omblego/internal/device"
)

// Record is an alias for device.BloodPressureRecord for convenience
type Record = device.BloodPressureRecord

// Backend is the interface all output backends must implement.
// Backends are responsible for persisting blood pressure records
// to various destinations (files, databases, APIs, etc.)
type Backend interface {
	// Name returns a unique identifier for this backend instance
	Name() string

	// Type returns the backend type (e.g., "csv", "influxdb", "webhook")
	Type() string

	// Write writes records to the backend.
	// userRecords is a slice of records per user (index 0 = user 1, etc.)
	// metadata contains additional context about the sync operation
	Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error

	// Health checks if the backend is operational
	Health(ctx context.Context) error

	// Close releases any resources held by the backend
	Close() error
}

// Metadata provides context for the write operation
type Metadata struct {
	DeviceName    string    // Device model name (e.g., "BP7000")
	SyncTimestamp time.Time // When the sync was performed
	IsFullSync    bool      // true if full sync, false if incremental
	SessionID     string    // Unique ID for this sync session
	BatteryLevel  *uint8    // Battery level percentage (nil if not available)
}

// RetryableBackend is an optional interface for backends that support retries
type RetryableBackend interface {
	Backend

	// RetryConfig returns the retry configuration for this backend
	RetryConfig() RetryConfig
}

// RetryConfig defines retry behavior for a backend
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64 // For exponential backoff
}

// BatchableBackend is an optional interface for backends that support batching
type BatchableBackend interface {
	Backend

	// BatchSize returns the preferred batch size
	BatchSize() int

	// Flush forces a flush of any buffered records
	Flush(ctx context.Context) error
}

// WriteResult contains the result of a write operation to a single backend
type WriteResult struct {
	Backend        string // Name of the backend
	RecordsWritten int    // Number of records written
	Error          error  // Error if write failed
}
