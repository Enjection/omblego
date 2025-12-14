package omblego

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	gosync "sync"
	"time"

	"omblego/internal/ble"
	"omblego/internal/device"
	intsync "omblego/internal/sync"

	// Import devices to register all drivers
	_ "omblego/internal/device/devices"
)

// Client is the main entry point for the omblego library.
// It provides methods to sync blood pressure data from Omron devices.
type Client struct {
	deviceName string
	opts       *clientOptions

	mu      gosync.Mutex
	syncing bool
}

// New creates a new Client for the specified device model.
// The deviceName should be one of the supported Omron device models
// (e.g., "BP7000", "HEM-7322T", etc.).
//
// Example:
//
//	client, err := omblego.New("BP7000",
//	    omblego.WithTimeSync(true),
//	    omblego.WithConnectTimeout(60*time.Second),
//	)
func New(deviceName string, options ...Option) (*Client, error) {
	opts := defaultOptions()
	opts.deviceName = deviceName

	for _, opt := range options {
		opt(opts)
	}

	if deviceName == "" {
		return nil, ErrInvalidConfig
	}

	return &Client{
		deviceName: deviceName,
		opts:       opts,
	}, nil
}

// DeviceName returns the configured device model name.
func (c *Client) DeviceName() string {
	return c.deviceName
}

// Logger returns the configured logger.
func (c *Client) Logger() *slog.Logger {
	return c.opts.logger
}

// SyncResult contains the results of a sync operation.
type SyncResult struct {
	// Records contains the blood pressure records organized by user.
	Records UserRecords

	// DeviceName is the model name of the synced device.
	DeviceName string

	// DeviceUUID is the BLE UUID of the synced device (if known).
	DeviceUUID string

	// SyncTimestamp is when the sync was performed.
	SyncTimestamp time.Time

	// Duration is how long the sync took.
	Duration time.Duration

	// RecordCount is the total number of records retrieved.
	RecordCount int

	// IsFullSync indicates if this was a full sync (vs. new records only).
	IsFullSync bool
}

// Sync performs a sync operation to retrieve blood pressure records from the device.
// It blocks until the sync is complete or the context is cancelled.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//
//	result, err := client.Sync(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for _, record := range result.Records[0] { // User 1
//	    fmt.Printf("%s: %d/%d mmHg, %d bpm\n",
//	        record.Timestamp.Format("2006-01-02 15:04"),
//	        record.Systolic, record.Diastolic, record.Pulse)
//	}
func (c *Client) Sync(ctx context.Context) (*SyncResult, error) {
	return c.SyncWithOptions(ctx, SyncOptions{
		TimeSync:       c.opts.timeSync,
		NewRecordsOnly: c.opts.newRecOnly,
		ConnectTimeout: c.opts.connectTimeout,
		ReadTimeout:    c.opts.readTimeout,
	})
}

// SyncWithOptions performs a sync operation with custom options.
func (c *Client) SyncWithOptions(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	c.mu.Lock()
	if c.syncing {
		c.mu.Unlock()
		return nil, ErrSyncInProgress
	}
	c.syncing = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.syncing = false
		c.mu.Unlock()
	}()

	startTime := time.Now()

	c.opts.logger.Info("sync started",
		"device", c.deviceName,
		"uuid", c.opts.deviceUUID,
	)

	// Create syncer and run sync
	syncer := intsync.NewWithLogger(nil, nil, c.opts.logger)
	syncOpts := intsync.Options{
		DeviceName:     c.deviceName,
		DeviceUUID:     c.opts.deviceUUID,
		TimeSync:       opts.TimeSync,
		NewRecOnly:     opts.NewRecordsOnly,
		ConnectTimeout: opts.ConnectTimeout,
		Debug:          c.opts.debug,
	}

	intResult, err := syncer.Run(ctx, syncOpts)
	if err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}

	// Convert internal result to public result
	result := &SyncResult{
		DeviceName:    intResult.DeviceName,
		DeviceUUID:    intResult.DeviceUUID,
		SyncTimestamp: intResult.SyncTimestamp,
		Duration:      time.Since(startTime),
		RecordCount:   intResult.RecordCount,
		IsFullSync:    intResult.IsFullSync,
		Records:       FromInternalUserRecords(intResult.Records),
	}

	c.opts.logger.Info("sync completed",
		"device", c.deviceName,
		"records", result.RecordCount,
		"duration", result.Duration,
	)

	return result, nil
}

// Pair initiates pairing with the device.
// This is required before the first sync with a new device.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
//	defer cancel()
//
//	result, err := client.Pair(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Paired with device: %s\n", result.DeviceUUID)
func (c *Client) Pair(ctx context.Context) (*PairResult, error) {
	c.opts.logger.Info("pairing started", "device", c.deviceName)

	// Get driver to verify device is supported
	driver, err := device.GetDriver(c.deviceName)
	if err != nil {
		return nil, fmt.Errorf("unsupported device: %w", err)
	}

	// Scan for device
	scanResults, err := c.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	if len(scanResults) == 0 {
		return nil, ErrDeviceNotFound
	}

	// Use first matching device
	targetUUID := scanResults[0].UUID

	c.opts.logger.Info("device found, pairing",
		"uuid", targetUUID,
		"name", scanResults[0].Name,
	)

	// Connect to initiate pairing (OS handles pairing dialog)
	if runtime.GOOS == "darwin" {
		autoClient, err := ble.NewAutoClient(c.opts.debug)
		if err != nil {
			return nil, fmt.Errorf("failed to create BLE client: %w", err)
		}
		defer autoClient.Disconnect()

		if err := autoClient.WaitReady(ctx); err != nil {
			return nil, fmt.Errorf("bluetooth not ready: %w", err)
		}

		timeout := c.opts.connectTimeout
		if timeout == 0 {
			timeout = 2 * time.Minute
		}

		if err := autoClient.ConnectByUUID(ctx, targetUUID, timeout); err != nil {
			return nil, fmt.Errorf("connection failed: %w", err)
		}
	} else {
		bleClient, err := ble.NewClient(c.opts.debug)
		if err != nil {
			return nil, fmt.Errorf("failed to create BLE client: %w", err)
		}
		defer bleClient.Close()

		if err := bleClient.Connect(ctx, targetUUID); err != nil {
			return nil, fmt.Errorf("connection failed: %w", err)
		}
		defer bleClient.Disconnect()
	}

	c.opts.logger.Info("pairing completed",
		"device", driver.GetConfig().Name,
		"uuid", targetUUID,
	)

	return &PairResult{
		DeviceName: driver.GetConfig().Name,
		DeviceUUID: targetUUID,
	}, nil
}

// PairResult contains the results of a pairing operation.
type PairResult struct {
	// DeviceName is the model name of the paired device.
	DeviceName string

	// DeviceUUID is the BLE UUID of the paired device.
	DeviceUUID string
}

// Close releases any resources held by the client.
func (c *Client) Close() error {
	// Currently no persistent resources to release
	return nil
}

// ScanResult represents a discovered device from scanning.
type ScanResult struct {
	// Name is the advertised device name.
	Name string

	// UUID is the BLE peripheral UUID.
	UUID string

	// RSSI is the signal strength indicator.
	RSSI int

	// ManufacturerData contains any manufacturer-specific data.
	ManufacturerData []byte
}

// Scan scans for nearby Omron blood pressure monitors.
// Returns a list of discovered devices that match the configured device type.
func (c *Client) Scan(ctx context.Context) ([]ScanResult, error) {
	c.opts.logger.Info("scanning for devices",
		"device_type", c.deviceName,
		"duration", c.opts.scanDuration,
	)

	// Get driver to get the BLE name prefix
	driver, err := device.GetDriver(c.deviceName)
	if err != nil {
		return nil, fmt.Errorf("unsupported device: %w", err)
	}

	cfg := driver.GetConfig()
	duration := c.opts.scanDuration
	if duration == 0 {
		duration = 10 * time.Second
	}

	// Use BLE client to scan
	bleClient, err := ble.NewClient(c.opts.debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create BLE client: %w", err)
	}
	defer bleClient.Close()

	bleScanResults, err := bleClient.Scan(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// Filter and convert to public result type
	var results []ScanResult
	for _, sr := range bleScanResults {
		// Filter by device name prefix if configured
		if cfg.BLENamePrefix != "" {
			if len(sr.Name) < len(cfg.BLENamePrefix) {
				continue
			}
			if sr.Name[:len(cfg.BLENamePrefix)] != cfg.BLENamePrefix {
				continue
			}
		}
		results = append(results, ScanResult{
			Name: sr.Name,
			UUID: sr.Address,
			RSSI: int(sr.RSSI),
		})
	}

	c.opts.logger.Info("scan completed", "devices_found", len(results))

	return results, nil
}

// ListSupportedDevices returns a list of all supported device model names.
func ListSupportedDevices() []string {
	return []string{
		"BP7000",
		"HEM-7322T",
		"HEM-7600T",
		"HEM-6232T",
		"HEM-7530T",
		"HEM-7155T",
		"HEM-7150T",
		"HEM-7342T",
		"HEM-7361T",
		"BP5250",
		"EVOLV",
	}
}
