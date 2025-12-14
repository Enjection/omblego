// Package sync provides the core sync orchestration for omblego.
// It coordinates BLE communication, device drivers, and output backends.
package sync

import (
	"log/slog"
	"time"

	"omblego/internal/ble"
	"omblego/internal/device"
	"omblego/internal/output"
)

// Options configures a sync operation
type Options struct {
	// Device identification (one required)
	DeviceName string // Device model name (e.g., "BP7000")
	DeviceUUID string // BLE peripheral UUID (for auto mode)
	DeviceMAC  string // MAC address (for direct connection)

	// Behavior options
	TimeSync   bool // Sync device clock after reading
	NewRecOnly bool // Only read new/unread records

	// Timeouts
	ConnectTimeout time.Duration // Connection timeout (0 = use default)
	ReadTimeout    time.Duration // Read operation timeout (0 = use default)

	// Logging
	Logger *slog.Logger

	// Debug
	Debug bool
}

// DefaultOptions returns options with sensible defaults
func DefaultOptions() Options {
	return Options{
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    60 * time.Second,
		Logger:         slog.Default(),
	}
}

// Result contains the results of a sync operation
type Result struct {
	Records       [][]device.BloodPressureRecord
	DeviceName    string
	DeviceUUID    string
	SyncTimestamp time.Time
	RecordCount   int
	IsFullSync    bool
}

// Syncer orchestrates the sync workflow
type Syncer struct {
	adapter ble.Adapter
	outputs *output.Manager
	logger  *slog.Logger
}

// New creates a new Syncer with the given adapter and output manager.
// If outputs is nil, no output will be written.
func New(adapter ble.Adapter, outputs *output.Manager) *Syncer {
	logger := slog.Default()
	return &Syncer{
		adapter: adapter,
		outputs: outputs,
		logger:  logger,
	}
}

// NewWithLogger creates a new Syncer with custom logger.
func NewWithLogger(adapter ble.Adapter, outputs *output.Manager, logger *slog.Logger) *Syncer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{
		adapter: adapter,
		outputs: outputs,
		logger:  logger,
	}
}

// SetLogger updates the logger
func (s *Syncer) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

// Close releases resources
func (s *Syncer) Close() error {
	var errs []error

	if s.outputs != nil {
		if err := s.outputs.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.adapter != nil {
		if err := s.adapter.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
