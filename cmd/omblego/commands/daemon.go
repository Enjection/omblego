package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"omblego/internal/ble"
	"omblego/internal/config"
	"omblego/internal/device"
	"omblego/internal/output"
)

var (
	daemonDeviceName  string
	daemonDeviceUUID  string
	daemonInterval    time.Duration
	daemonTimeSync    bool
	daemonInfluxURL   string
	daemonInfluxToken string
	daemonInfluxOrg   string
	daemonInfluxBucket string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run as a daemon, syncing on interval",
	Long: `Runs omblego as a daemon that periodically syncs blood pressure records.

The daemon will:
- Wait for the device to connect
- Read all records
- Push new records to InfluxDB (if configured)
- Save records to CSV
- Repeat at the configured interval

Signals:
- SIGHUP: Force an immediate sync (bypass interval wait)
- SIGTERM/SIGINT: Graceful shutdown

Examples:
  # Run daemon with 3-hour sync interval
  omblego daemon -d BP7000 --interval 3h

  # Daemon with InfluxDB output
  omblego daemon -d BP7000 --interval 3h \
    --influx-url http://localhost:8086 \
    --influx-token YOUR_TOKEN \
    --influx-org myorg --influx-bucket health

  # Force sync with signal (from another terminal)
  kill -HUP $(pgrep omblego)`,
	RunE: runDaemon,
}

func init() {
	daemonCmd.Flags().StringVarP(&daemonDeviceName, "device", "d", "", "Device model name (e.g., BP7000)")
	daemonCmd.Flags().StringVar(&daemonDeviceUUID, "uuid", "", "Device UUID")
	daemonCmd.Flags().DurationVar(&daemonInterval, "interval", 3*time.Hour, "Sync interval")
	daemonCmd.Flags().BoolVarP(&daemonTimeSync, "time-sync", "t", false, "Synchronize device time on each sync")

	// InfluxDB flags
	daemonCmd.Flags().StringVar(&daemonInfluxURL, "influx-url", "", "InfluxDB URL")
	daemonCmd.Flags().StringVar(&daemonInfluxToken, "influx-token", "", "InfluxDB API token")
	daemonCmd.Flags().StringVar(&daemonInfluxOrg, "influx-org", "", "InfluxDB organization")
	daemonCmd.Flags().StringVar(&daemonInfluxBucket, "influx-bucket", "", "InfluxDB bucket")

	daemonCmd.MarkFlagRequired("device")

	AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("daemon mode requires auto mode which is only available on macOS")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGTERM/SIGINT for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle SIGHUP for forced sync
	sighupChan := make(chan os.Signal, 1)
	signal.Notify(sighupChan, syscall.SIGHUP)

	// Get device driver
	driver, err := device.GetDriver(daemonDeviceName)
	if err != nil {
		return fmt.Errorf("unsupported device: %w", err)
	}

	// Load config if available
	if cfgFile := GetConfigFile(); cfgFile != "" {
		loadDaemonConfig(cfgFile)
	}

	// Setup InfluxDB if configured
	var influxWriter *output.InfluxDBWriter
	if daemonInfluxURL != "" {
		if daemonInfluxToken == "" || daemonInfluxOrg == "" || daemonInfluxBucket == "" {
			return fmt.Errorf("InfluxDB requires --influx-url, --influx-token, --influx-org, and --influx-bucket")
		}

		influxWriter = output.NewInfluxDBWriter(output.InfluxDBConfig{
			URL:    daemonInfluxURL,
			Token:  daemonInfluxToken,
			Org:    daemonInfluxOrg,
			Bucket: daemonInfluxBucket,
		})
		defer influxWriter.Close()

		if err := influxWriter.Ping(ctx); err != nil {
			return fmt.Errorf("failed to connect to InfluxDB: %w", err)
		}
		slog.Info("connected to InfluxDB", "url", daemonInfluxURL, "org", daemonInfluxOrg, "bucket", daemonInfluxBucket)
	}

	sentTracker := output.NewSentTracker()
	cfg := driver.GetConfig()

	slog.Info("starting daemon", "interval", daemonInterval, "device", cfg.Name)
	if influxWriter != nil {
		slog.Info("pushing to InfluxDB", "url", daemonInfluxURL)
	}

	// Force sync channel
	forceSyncCh := make(chan struct{}, 1)

	// Handle SIGHUP for forced sync
	go func() {
		for range sighupChan {
			slog.Info("SIGHUP received, forcing sync...")
			select {
			case forceSyncCh <- struct{}{}:
			default:
				slog.Debug("force sync already pending")
			}
		}
	}()

	// Run initial sync
	if err := runDaemonSyncOnce(ctx, driver, cfg, influxWriter, sentTracker); err != nil {
		slog.Error("initial sync failed", "error", err)
	}

	// Run on interval
	ticker := time.NewTicker(daemonInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon stopped")
			return nil

		case <-sigChan:
			slog.Info("shutdown signal received, stopping daemon...")
			return nil

		case <-ticker.C:
			slog.Info("running scheduled sync...")
			if err := runDaemonSyncOnce(ctx, driver, cfg, influxWriter, sentTracker); err != nil {
				slog.Error("sync failed", "error", err)
			}

		case <-forceSyncCh:
			slog.Info("running forced sync...")
			if err := runDaemonSyncOnce(ctx, driver, cfg, influxWriter, sentTracker); err != nil {
				slog.Error("forced sync failed", "error", err)
			}
			// Reset ticker after forced sync
			ticker.Reset(daemonInterval)
		}
	}
}

func loadDaemonConfig(cfgFile string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return
	}

	// Apply config values only if CLI flags weren't explicitly set
	if daemonDeviceName == "" && cfg.Device.Model != "" {
		daemonDeviceName = cfg.Device.Model
	}
	if daemonDeviceUUID == "" && cfg.Device.UUID != "" && cfg.Device.UUID != "auto" {
		daemonDeviceUUID = cfg.Device.UUID
	}
	if cfg.Daemon.Interval > 0 {
		daemonInterval = cfg.Daemon.Interval
	}
	if cfg.Daemon.TimeSync {
		daemonTimeSync = true
	}

	// InfluxDB from config
	for _, out := range cfg.Outputs {
		if out.Type == "influxdb" && out.Enabled {
			if daemonInfluxURL == "" {
				daemonInfluxURL, _ = out.Settings["url"].(string)
			}
			if daemonInfluxToken == "" {
				daemonInfluxToken, _ = out.Settings["token"].(string)
			}
			if daemonInfluxOrg == "" {
				daemonInfluxOrg, _ = out.Settings["org"].(string)
			}
			if daemonInfluxBucket == "" {
				daemonInfluxBucket, _ = out.Settings["bucket"].(string)
			}
		}
	}
}

func runDaemonSyncOnce(ctx context.Context, driver device.Driver, cfg device.Config, influxWriter *output.InfluxDBWriter, sentTracker *output.SentTracker) error {
	// Create auto client
	autoClient, err := ble.NewAutoClient(IsDebugMode())
	if err != nil {
		return fmt.Errorf("failed to create auto client: %w", err)
	}
	defer autoClient.Disconnect()

	// Get UUID
	targetUUID := daemonDeviceUUID
	if targetUUID == "" {
		savedUUID, err := config.GetDeviceUUID(cfg.Name)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if savedUUID == "" {
			return fmt.Errorf("no saved UUID for %s. Run 'omblego pair' first", cfg.Name)
		}
		targetUUID = savedUUID
	}

	slog.Debug("using device UUID", "uuid", targetUUID)

	// Wait for Bluetooth
	if err := autoClient.WaitReady(ctx); err != nil {
		return fmt.Errorf("Bluetooth not ready: %w", err)
	}

	// Connect with timeout
	syncTimeout := 2 * time.Minute
	slog.Info("waiting for device...", "timeout", syncTimeout)
	if err := autoClient.ConnectByUUID(ctx, targetUUID, syncTimeout); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Discover services
	if err := autoClient.DiscoverOmronServices(ctx); err != nil {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	// Setup handler
	handler := ble.NewAutoTxRxHandler(autoClient, IsDebugMode())
	if err := handler.SetupNotifications(); err != nil {
		return fmt.Errorf("failed to setup notifications: %w", err)
	}

	// Wait for stabilization
	time.Sleep(10 * time.Second)

	// Unlock
	if err := handler.UnlockWithKey(ctx, nil); err != nil {
		return fmt.Errorf("failed to unlock device: %w", err)
	}

	// Start transmission
	if err := handler.StartTransmission(ctx); err != nil {
		return fmt.Errorf("failed to start transmission: %w", err)
	}

	// Time sync
	if daemonTimeSync {
		settingsSize := int(cfg.SettingsWriteAddress - cfg.SettingsReadAddress)
		cachedSettings, err := handler.ReadEEPROM(ctx, cfg.SettingsReadAddress, settingsSize, cfg.TransmissionBlockSize)
		if err == nil {
			syncBytes, err := driver.BuildTimeSyncBytes(cachedSettings, time.Now())
			if err == nil {
				writeAddr := cfg.SettingsWriteAddress + uint16(cfg.TimeSyncRange[0])
				if err := handler.WriteEEPROM(ctx, writeAddr, syncBytes, len(syncBytes)); err != nil {
					slog.Warn("time sync failed", "error", err)
				}
			}
		}
	}

	// Read records
	totalSize := cfg.RecordsPerUser[0] * cfg.RecordByteSize
	recordData, err := handler.ReadEEPROM(ctx, cfg.UserStartAddresses[0], totalSize, cfg.TransmissionBlockSize)
	if err != nil {
		handler.EndTransmission(ctx)
		return fmt.Errorf("failed to read records: %w", err)
	}

	handler.EndTransmission(ctx)

	// Parse records
	records := parseRecordsFromData(recordData, driver, cfg)
	slog.Info("read records from device", "count", len(records))

	if len(records) == 0 {
		return nil
	}

	allRecords := [][]device.BloodPressureRecord{records}

	// Filter to only new records
	newRecords := sentTracker.FilterNewRecords(allRecords)
	newCount := 0
	for _, userRecords := range newRecords {
		newCount += len(userRecords)
	}

	if newCount == 0 {
		slog.Info("no new records to push")
		return nil
	}

	slog.Info("found new records", "count", newCount)

	// Push to InfluxDB
	if influxWriter != nil {
		if err := influxWriter.WriteRecords(ctx, newRecords, cfg.Name); err != nil {
			return fmt.Errorf("failed to write to InfluxDB: %w", err)
		}
		slog.Info("pushed records to InfluxDB", "count", newCount)
	}

	// Write to CSV
	if err := output.WriteCSV(allRecords); err != nil {
		slog.Warn("failed to write CSV", "error", err)
	}

	// Update tracker
	sentTracker.UpdateLastSent(newRecords)

	return nil
}
