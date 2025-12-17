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
	daemonDeviceName string
	daemonDeviceUUID string
	daemonTimeSync   bool
	daemonUseEEPROM  bool
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run as a daemon, syncing when device connects",
	Long: `Runs omblego as a daemon that syncs blood pressure records.

The daemon syncs blood pressure records whenever the Omron device advertises.
The device advertises when:
1. User presses the Bluetooth button
2. User takes a new measurement (device auto-advertises for ~60 seconds)

How it works:
1. Daemon waits passively for device to advertise (energy-efficient)
2. When device advertises, daemon checks the sequence number in advertisement data
3. If sequence number changed: connect and sync new records
4. If sequence number unchanged: skip sync (no new data)

This approach avoids unnecessary syncs by checking the advertisement data
before connecting - the same technique used by the SpeziDevices Swift library.

Output backends (CSV, InfluxDB, Prometheus, etc.) are configured via config file.

Signals:
- SIGTERM/SIGINT: Graceful shutdown

Examples:
  # Run daemon with config file
  omblego daemon -d BP7000 --config ~/.config/omblego/config.yaml`,
	RunE: runDaemon,
}

func init() {
	daemonCmd.Flags().StringVarP(&daemonDeviceName, "device", "d", "", "Device model name (e.g., BP7000)")
	daemonCmd.Flags().StringVar(&daemonDeviceUUID, "uuid", "", "Device UUID")
	daemonCmd.Flags().BoolVarP(&daemonTimeSync, "time-sync", "t", false, "Synchronize device time on each sync")
	daemonCmd.Flags().BoolVar(&daemonUseEEPROM, "eeprom", false, "Use legacy EEPROM protocol instead of standard BLE GATT")

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
	go func() {
		<-sigChan
		slog.Info("shutdown signal received, stopping daemon...")
		cancel()
	}()

	// Get device driver
	driver, err := device.GetDriver(daemonDeviceName)
	if err != nil {
		return fmt.Errorf("unsupported device: %w", err)
	}

	// Load config if available
	if cfgFile := GetConfigFile(); cfgFile != "" {
		loadDaemonConfig(cfgFile)
	}

	sentTracker := output.NewSentTracker()
	cfg := driver.GetConfig()

	// Load output backends from config
	var backends []output.Backend
	if cfgFile := GetConfigFile(); cfgFile != "" {
		appCfg, err := config.Load(cfgFile)
		if err == nil {
			for _, outCfg := range appCfg.Outputs {
				if !outCfg.Enabled {
					continue
				}
				backend, err := output.Create(output.BackendConfig{
					Type:     outCfg.Type,
					Name:     outCfg.Name,
					Enabled:  outCfg.Enabled,
					Settings: outCfg.Settings,
				})
				if err != nil {
					slog.Warn("failed to create output backend", "type", outCfg.Type, "error", err)
					continue
				}
				backends = append(backends, backend)
				slog.Info("enabled output backend", "type", outCfg.Type, "name", backend.Name())
			}
		}
	}
	// Ensure backends are closed on exit
	defer func() {
		for _, b := range backends {
			b.Close()
		}
	}()

	slog.Info("starting daemon", "device", cfg.Name, "backends", len(backends))

	// Get target UUID
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

	// Create AutoClient for the daemon lifetime
	autoClient, err := ble.NewAutoClient(IsDebugMode())
	if err != nil {
		return fmt.Errorf("failed to create auto client: %w", err)
	}
	defer autoClient.Disconnect()

	if err := autoClient.WaitReady(ctx); err != nil {
		return fmt.Errorf("Bluetooth not ready: %w", err)
	}

	slog.Info("daemon ready")

	// Use GATT protocol by default, EEPROM only if explicitly requested
	if !daemonUseEEPROM {
		return runGATTDaemonLoop(ctx, autoClient, driver, cfg, targetUUID, sentTracker, backends)
	}

	// Legacy EEPROM protocol (only when --eeprom flag is used)
	// Track last synced sequence number (for active scan) and use cooldown (for passive)
	var lastSyncedSeqNum uint16
	var lastEmptySyncTime time.Time
	const cooldownAfterEmptySync = 60 * time.Second

	for {
		// Check if we're in cooldown period (recently synced with no new data)
		if !lastEmptySyncTime.IsZero() {
			elapsed := time.Since(lastEmptySyncTime)
			if elapsed < cooldownAfterEmptySync {
				remaining := cooldownAfterEmptySync - elapsed
				slog.Info("cooldown period (no new data last sync), waiting...", "remaining", remaining.Round(time.Second))
				select {
				case <-time.After(remaining):
				case <-ctx.Done():
					slog.Info("daemon stopped")
					return nil
				}
				lastEmptySyncTime = time.Time{} // Reset cooldown
			}
		}

		// Passive wait for device to connect (user presses Bluetooth button or takes measurement)
		slog.Info("waiting for device... (press Bluetooth button or take a measurement)")
		connErr := autoClient.ConnectByUUID(ctx, targetUUID, 0)

		if connErr != nil {
			if ctx.Err() != nil {
				slog.Info("daemon stopped")
				return nil
			}
			slog.Error("connection failed", "error", connErr)
			time.Sleep(5 * time.Second)
			continue
		}

		// Check advertisement data if available (only works with active scanning)
		advData := autoClient.GetLastAdvData()
		if advData.Valid {
			slog.Debug("advertisement data", "seqNum", advData.SequenceNumber, "records", advData.RecordsNumber, "pairing", advData.PairingMode)

			// Skip if sequence number hasn't changed (no new records)
			if advData.SequenceNumber == lastSyncedSeqNum && lastSyncedSeqNum != 0 {
				slog.Info("no new records (seqNum unchanged), skipping sync", "seqNum", advData.SequenceNumber)
				autoClient.Disconnect()
				lastEmptySyncTime = time.Now()
				continue
			}
		}

		slog.Info("device connected")

		// Run connected session - sync records
		foundNewRecords := runConnectedLoop(ctx, autoClient, driver, cfg, sentTracker, backends)

		if ctx.Err() != nil {
			slog.Info("daemon stopped")
			return nil
		}

		// Update tracking state
		if advData.Valid {
			lastSyncedSeqNum = advData.SequenceNumber
		}

		if foundNewRecords {
			slog.Info("sync done, found new records")
			lastEmptySyncTime = time.Time{} // Clear cooldown
		} else {
			slog.Info("sync done, no new data - entering cooldown")
			lastEmptySyncTime = time.Now() // Start cooldown
		}

		// Brief pause for device to disconnect
		time.Sleep(3 * time.Second)
	}
}

// runConnectedLoop handles a single device connection session.
// The Omron device disconnects itself ~3 seconds after EndTransmission - this is
// expected firmware behavior, not a bug. There is no "idle but connected" state.
// Returns true if new records were found and synced.
func runConnectedLoop(ctx context.Context, autoClient *ble.AutoClient, driver device.Driver, cfg device.Config, sentTracker *output.SentTracker, backends []output.Backend) bool {
	// Setup: discover services and setup notifications
	handler, err := setupConnection(ctx, autoClient)
	if err != nil {
		slog.Error("failed to setup connection", "error", err)
		autoClient.Disconnect() // Clean up connection state
		return false
	}

	// Perform sync
	newRecords, err := performSync(ctx, handler, driver, cfg, sentTracker, backends)

	// Always disconnect to ensure clean state for next connection
	autoClient.Disconnect()

	if err != nil {
		slog.Error("sync failed", "error", err)
		return false
	}

	return newRecords > 0
}

// setupConnection performs one-time BLE setup: discover services and enable notifications
func setupConnection(ctx context.Context, autoClient *ble.AutoClient) (*ble.AutoTxRxHandler, error) {
	slog.Debug("discovering services...")
	if err := autoClient.DiscoverOmronServices(ctx); err != nil {
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	handler := ble.NewAutoTxRxHandler(autoClient, IsDebugMode())

	slog.Debug("setting up notifications...")
	if err := handler.SetupNotifications(); err != nil {
		return nil, fmt.Errorf("failed to setup notifications: %w", err)
	}

	// Wait for connection to stabilize
	slog.Debug("waiting for connection to stabilize...")
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return handler, nil
}

// performSync does a complete sync cycle: unlock -> start transmission -> read -> end transmission.
// After EndTransmission, the device will disconnect itself in ~3 seconds (Omron firmware behavior).
// Returns the number of new records synced and any error.
func performSync(ctx context.Context, handler *ble.AutoTxRxHandler, driver device.Driver, cfg device.Config, sentTracker *output.SentTracker, backends []output.Backend) (int, error) {
	// Enter transmission mode
	slog.Debug("unlocking device...")
	if err := unlockWithRetry(ctx, handler); err != nil {
		return 0, fmt.Errorf("unlock failed: %w", err)
	}

	slog.Debug("starting transmission mode...")
	if err := startTransmissionWithRetry(ctx, handler); err != nil {
		return 0, fmt.Errorf("start transmission failed: %w", err)
	}

	// Read records
	slog.Info("reading records...")
	newCount, err := readAndPushRecords(ctx, handler, driver, cfg, sentTracker, backends)

	// Always end transmission to let device go idle, even if read failed
	slog.Debug("ending transmission mode...")
	if endErr := handler.EndTransmission(ctx); endErr != nil {
		slog.Warn("end transmission failed", "error", endErr)
	}

	if err != nil {
		return 0, err
	}

	slog.Info("sync completed", "new_records", newCount)
	return newCount, nil
}


// readAndPushRecords reads records from device (assumes session already established)
// Returns the number of new records pushed and any error.
func readAndPushRecords(ctx context.Context, handler *ble.AutoTxRxHandler, driver device.Driver, cfg device.Config, sentTracker *output.SentTracker, backends []output.Backend) (int, error) {
	// Time sync (optional)
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
		return 0, fmt.Errorf("failed to read records: %w", err)
	}

	// Parse and push records
	records := parseRecordsFromData(recordData, driver, cfg)
	slog.Info("read records from device", "count", len(records))

	if len(records) == 0 {
		return 0, nil
	}

	allRecords := [][]device.BloodPressureRecord{records}
	newRecords := sentTracker.FilterNewRecords(allRecords)
	newCount := 0
	for _, userRecords := range newRecords {
		newCount += len(userRecords)
	}

	if newCount == 0 {
		slog.Info("no new records to push")
		return 0, nil
	}

	slog.Info("found new records", "count", newCount)

	// Write to all configured backends
	metadata := output.Metadata{
		DeviceName:    cfg.Name,
		SyncTimestamp: time.Now(),
		IsFullSync:    true,
	}

	// Convert to output.Record format
	var outputRecords [][]output.Record
	for _, userRecords := range newRecords {
		var userOutputRecords []output.Record
		for _, r := range userRecords {
			userOutputRecords = append(userOutputRecords, output.Record(r))
		}
		outputRecords = append(outputRecords, userOutputRecords)
	}

	for _, backend := range backends {
		if err := backend.Write(ctx, outputRecords, metadata); err != nil {
			slog.Warn("failed to write to backend", "backend", backend.Name(), "error", err)
		} else {
			slog.Debug("wrote to backend", "backend", backend.Name(), "count", newCount)
		}
	}

	sentTracker.UpdateLastSent(newRecords)
	return newCount, nil
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
	if cfg.Daemon.TimeSync {
		daemonTimeSync = true
	}
}

// runGATTDaemonLoop runs the daemon using standard BLE GATT protocol.
// This approach is simpler than EEPROM:
// 1. Connect to device
// 2. Discover GATT services (Blood Pressure, Current Time, Battery)
// 3. Enable notifications
// 4. Sync time (device expects this)
// 5. Wait for measurements via notifications
// 6. Device disconnects itself after sending data
func runGATTDaemonLoop(ctx context.Context, autoClient *ble.AutoClient, driver device.Driver, cfg device.Config, targetUUID string, sentTracker *output.SentTracker, backends []output.Backend) error {
	slog.Info("using standard BLE GATT protocol")

	for {
		// Wait for device to connect
		slog.Info("waiting for device... (press Bluetooth button or take a measurement)")
		connErr := autoClient.ConnectByUUID(ctx, targetUUID, 0)

		if connErr != nil {
			if ctx.Err() != nil {
				slog.Info("daemon stopped")
				return nil
			}
			slog.Error("connection failed", "error", connErr)
			time.Sleep(5 * time.Second)
			continue
		}

		slog.Info("device connected, discovering GATT services...")

		// Run GATT session
		newRecords := runGATTSession(ctx, autoClient, driver, cfg, sentTracker, backends)

		if ctx.Err() != nil {
			slog.Info("daemon stopped")
			return nil
		}

		if newRecords > 0 {
			slog.Info("sync done", "new_records", newRecords)
		} else {
			slog.Info("sync done, no new data")
		}

		// Wait for device to disconnect (Omron devices disconnect themselves)
		autoClient.WaitForDisconnect(ctx, 10*time.Second)
		autoClient.Disconnect() // Ensure cleanup

		// Brief pause before waiting for next connection
		time.Sleep(2 * time.Second)
	}
}

// runGATTSession handles a single GATT connection session.
// Returns the number of new records synced.
func runGATTSession(ctx context.Context, autoClient *ble.AutoClient, driver device.Driver, cfg device.Config, sentTracker *output.SentTracker, backends []output.Backend) int {
	// Clear any previous measurements
	autoClient.ClearCollectedMeasurements()

	// Track battery level
	var batteryLevel *uint8

	// Discover GATT services
	if err := autoClient.DiscoverGATTServices(ctx); err != nil {
		slog.Error("failed to discover GATT services", "error", err)
		autoClient.Disconnect()
		return 0
	}

	if !autoClient.HasGATTBloodPressureService() {
		slog.Error("Blood Pressure Service not found on device")
		autoClient.Disconnect()
		return 0
	}

	slog.Debug("GATT services discovered")

	// Enable notifications - this is key for receiving measurements
	if err := autoClient.EnableGATTNotifications(); err != nil {
		slog.Error("failed to enable GATT notifications", "error", err)
		autoClient.Disconnect()
		return 0
	}

	slog.Debug("GATT notifications enabled")

	// Sync time - Omron devices expect this on every connection
	// The device will start sending measurements after time sync
	if err := autoClient.SyncCurrentTime(); err != nil {
		slog.Warn("time sync failed", "error", err)
		// Continue anyway, device might still send measurements
	} else {
		slog.Debug("time synchronized")
	}

	// Wait for measurements via notifications
	// Omron devices send all stored measurements automatically after time sync
	slog.Info("waiting for measurements...")

	// Wait with timeout for measurements
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	// Also wait for disconnect signal
	measurementsDone := false
	lastMeasurementTime := time.Now()

	for !measurementsDone {
		select {
		case measurement := <-autoClient.GetBPMeasurementChannel():
			lastMeasurementTime = time.Now()
			slog.Info("received measurement",
				"systolic", int(measurement.Systolic),
				"diastolic", int(measurement.Diastolic),
				"pulse", int(measurement.PulseRate),
				"timestamp", measurement.Timestamp)

			// Reset timeout - more measurements might come
			timeout.Reset(5 * time.Second)

		case level := <-autoClient.GetBatteryLevelChannel():
			batteryLevel = &level
			slog.Info("battery level", "percent", level)

		case <-timeout.C:
			// Timeout - check if we received any measurements recently
			if time.Since(lastMeasurementTime) > 5*time.Second {
				measurementsDone = true
			}

		case <-ctx.Done():
			return 0
		}
	}

	// Get all collected measurements
	gattMeasurements := autoClient.GetCollectedMeasurements()
	if len(gattMeasurements) == 0 {
		slog.Info("no measurements received")
		return 0
	}

	slog.Info("received measurements from device", "count", len(gattMeasurements))

	// Convert GATT measurements to device records
	records := convertGATTMeasurements(gattMeasurements, cfg)

	// Filter for new records
	allRecords := [][]device.BloodPressureRecord{records}
	newRecords := sentTracker.FilterNewRecords(allRecords)
	newCount := 0
	for _, userRecords := range newRecords {
		newCount += len(userRecords)
	}

	if newCount == 0 {
		slog.Info("no new records to push")
		return 0
	}

	slog.Info("found new records", "count", newCount)

	// Write to backends
	metadata := output.Metadata{
		DeviceName:    cfg.Name,
		SyncTimestamp: time.Now(),
		IsFullSync:    true,
		BatteryLevel:  batteryLevel,
	}

	var outputRecords [][]output.Record
	for _, userRecords := range newRecords {
		var userOutputRecords []output.Record
		for _, r := range userRecords {
			userOutputRecords = append(userOutputRecords, output.Record(r))
		}
		outputRecords = append(outputRecords, userOutputRecords)
	}

	for _, backend := range backends {
		if err := backend.Write(ctx, outputRecords, metadata); err != nil {
			slog.Warn("failed to write to backend", "backend", backend.Name(), "error", err)
		} else {
			slog.Debug("wrote to backend", "backend", backend.Name(), "count", newCount)
		}
	}

	sentTracker.UpdateLastSent(newRecords)
	return newCount
}

// convertGATTMeasurements converts GATT measurements to device records
func convertGATTMeasurements(gattMeasurements []*ble.GATTBloodPressureMeasurement, cfg device.Config) []device.BloodPressureRecord {
	records := make([]device.BloodPressureRecord, 0, len(gattMeasurements))

	for _, m := range gattMeasurements {
		record := device.BloodPressureRecord{
			Systolic:  int(m.Systolic),
			Diastolic: int(m.Diastolic),
			Pulse:     int(m.PulseRate),
			Timestamp: m.Timestamp,
			Movement:  m.BodyMovement,
			IHB:       m.IrregularPulse,
		}

		// Skip invalid records
		if record.Systolic == 0 || record.Diastolic == 0 {
			continue
		}

		// Use sync time if no timestamp
		if record.Timestamp.IsZero() {
			record.Timestamp = time.Now()
		}

		records = append(records, record)
	}

	return records
}
