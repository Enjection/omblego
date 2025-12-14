package omblego

import (
	"log/slog"
	"time"
)

// Option is a functional option for configuring a Client.
type Option func(*clientOptions)

// clientOptions holds all configurable options for a Client.
type clientOptions struct {
	// Device identification
	deviceName string
	deviceUUID string
	deviceMAC  string

	// Behavior
	timeSync   bool
	newRecOnly bool

	// Timeouts
	connectTimeout time.Duration
	readTimeout    time.Duration
	scanDuration   time.Duration

	// Logging
	logger *slog.Logger
	debug  bool
}

// defaultOptions returns the default client options.
func defaultOptions() *clientOptions {
	return &clientOptions{
		connectTimeout: 30 * time.Second,
		readTimeout:    60 * time.Second,
		scanDuration:   10 * time.Second,
		logger:         slog.Default(),
	}
}

// WithDeviceUUID sets the specific device UUID to connect to.
// Use this when you know the exact device UUID (e.g., from a previous scan).
func WithDeviceUUID(uuid string) Option {
	return func(o *clientOptions) {
		o.deviceUUID = uuid
	}
}

// WithDeviceMAC sets the device MAC address for direct connection.
// Note: MAC address based connection is platform-dependent.
func WithDeviceMAC(mac string) Option {
	return func(o *clientOptions) {
		o.deviceMAC = mac
	}
}

// WithTimeSync enables clock synchronization after reading records.
// This updates the device's clock to match the system time.
func WithTimeSync(enabled bool) Option {
	return func(o *clientOptions) {
		o.timeSync = enabled
	}
}

// WithNewRecordsOnly configures the client to only read new/unread records.
// When enabled, previously read records will not be returned.
func WithNewRecordsOnly(enabled bool) Option {
	return func(o *clientOptions) {
		o.newRecOnly = enabled
	}
}

// WithConnectTimeout sets the timeout for establishing a connection.
func WithConnectTimeout(d time.Duration) Option {
	return func(o *clientOptions) {
		o.connectTimeout = d
	}
}

// WithReadTimeout sets the timeout for read operations.
func WithReadTimeout(d time.Duration) Option {
	return func(o *clientOptions) {
		o.readTimeout = d
	}
}

// WithScanDuration sets how long to scan for devices.
func WithScanDuration(d time.Duration) Option {
	return func(o *clientOptions) {
		o.scanDuration = d
	}
}

// WithLogger sets a custom logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(o *clientOptions) {
		o.logger = logger
	}
}

// WithDebug enables debug mode for verbose logging.
func WithDebug(enabled bool) Option {
	return func(o *clientOptions) {
		o.debug = enabled
	}
}

// SyncOptions holds options for a single sync operation.
type SyncOptions struct {
	// TimeSync enables clock synchronization after reading.
	TimeSync bool

	// NewRecordsOnly only returns new/unread records.
	NewRecordsOnly bool

	// ConnectTimeout overrides the default connection timeout.
	ConnectTimeout time.Duration

	// ReadTimeout overrides the default read timeout.
	ReadTimeout time.Duration
}

// DefaultSyncOptions returns the default sync options.
func DefaultSyncOptions() SyncOptions {
	return SyncOptions{
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    60 * time.Second,
	}
}

// DaemonOptions holds options for running in daemon mode.
type DaemonOptions struct {
	// Interval is the time between automatic syncs.
	Interval time.Duration

	// SyncOptions are the options used for each sync operation.
	SyncOptions SyncOptions

	// OnSync is called after each sync attempt with the result.
	OnSync func(result *SyncResult, err error)

	// OnError is called when a sync error occurs (in addition to OnSync).
	OnError func(err error)
}

// DefaultDaemonOptions returns the default daemon options.
func DefaultDaemonOptions() DaemonOptions {
	return DaemonOptions{
		Interval:    3 * time.Hour,
		SyncOptions: DefaultSyncOptions(),
	}
}
