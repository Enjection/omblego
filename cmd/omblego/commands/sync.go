package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"omblego/internal/ble"
	"omblego/internal/config"
	"omblego/internal/device"
	"omblego/internal/output"
)

var (
	// Sync command flags
	syncDeviceName  string
	syncDeviceUUID  string
	syncDeviceMAC   string
	syncNewRecOnly  bool
	syncTimeSync    bool
	syncAutoMode    bool
	syncAutoTimeout time.Duration
	syncOutputFormat string
	syncInteractive bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync blood pressure records from device",
	Long: `Syncs blood pressure measurement records from an Omron BLE device.

The device must already be paired. Use 'omblego pair' for first-time setup.

Examples:
  # Basic sync (interactive mode - prompts for device selection)
  omblego sync -d BP7000 --interactive

  # Sync with known UUID (non-interactive)
  omblego sync -d BP7000 --uuid <device-uuid>

  # Auto mode - waits for device to connect (macOS only)
  omblego sync -d BP7000 --auto

  # Sync and update device time
  omblego sync -d BP7000 --auto --time-sync

  # Only get new records
  omblego sync -d BP7000 --auto --new-only`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().StringVarP(&syncDeviceName, "device", "d", "", "Device model name (e.g., BP7000)")
	syncCmd.Flags().StringVar(&syncDeviceUUID, "uuid", "", "Device UUID (skips scanning)")
	syncCmd.Flags().StringVarP(&syncDeviceMAC, "mac", "m", "", "Device MAC address")
	syncCmd.Flags().BoolVarP(&syncNewRecOnly, "new-only", "n", false, "Only read new/unread records")
	syncCmd.Flags().BoolVarP(&syncTimeSync, "time-sync", "t", false, "Synchronize device time")
	syncCmd.Flags().BoolVarP(&syncAutoMode, "auto", "a", false, "Auto mode: wait for device (macOS only)")
	syncCmd.Flags().DurationVar(&syncAutoTimeout, "timeout", 0, "Timeout for auto mode (0 = indefinite)")
	syncCmd.Flags().StringVarP(&syncOutputFormat, "output", "o", "csv", "Output format: csv, json, or both")
	syncCmd.Flags().BoolVarP(&syncInteractive, "interactive", "i", false, "Interactive mode: scan and select device")

	syncCmd.MarkFlagRequired("device")

	AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		slog.Info("interrupted, cleaning up...")
		cancel()
	}()

	// Load config if available
	if cfgFile := GetConfigFile(); cfgFile != "" {
		loadSyncConfig(cfgFile)
	}

	// Get device driver
	driver, err := device.GetDriver(syncDeviceName)
	if err != nil {
		return fmt.Errorf("unsupported device: %w", err)
	}
	slog.Info("using device driver", "device", driver.GetConfig().Name)

	// Auto mode
	if syncAutoMode {
		return runSyncAutoMode(ctx, driver)
	}

	// Interactive/Manual mode
	return runSyncManualMode(ctx, driver)
}

func loadSyncConfig(cfgFile string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return
	}

	// Apply config values only if CLI flags weren't explicitly set
	if syncDeviceName == "" && cfg.Device.Model != "" {
		syncDeviceName = cfg.Device.Model
	}
	if syncDeviceUUID == "" && cfg.Device.UUID != "" && cfg.Device.UUID != "auto" {
		syncDeviceUUID = cfg.Device.UUID
	}
}

func runSyncManualMode(ctx context.Context, driver device.Driver) error {
	slog.Info("initializing Bluetooth...")

	client, err := ble.NewClient(IsDebugMode())
	if err != nil {
		return fmt.Errorf("failed to initialize Bluetooth: %w", err)
	}
	defer client.Close()

	// Get device address
	var deviceAddr string
	if syncDeviceMAC != "" {
		deviceAddr = syncDeviceMAC
	} else if syncDeviceUUID != "" {
		deviceAddr = syncDeviceUUID
	} else if syncInteractive {
		// Interactive scan mode
		var err error
		deviceAddr, err = interactiveScan(ctx, client)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("specify --uuid, --mac, or use --interactive for device selection")
	}

	// Connect to device
	slog.Info("connecting to device...", "address", deviceAddr)
	if err := client.Connect(ctx, deviceAddr); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer client.Disconnect()

	cfg := driver.GetConfig()

	// Wait for OS Bluetooth connection
	if cfg.Protocol != device.ProtocolStandardGATT {
		slog.Info("waiting for OS Bluetooth connection...")
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Read records
	slog.Info("reading records from device...")
	options := device.ReadOptions{
		UseUnreadCounter: syncNewRecOnly,
		SyncTime:         syncTimeSync,
		Debug:            IsDebugMode(),
	}

	var records [][]device.BloodPressureRecord

	if cfg.Protocol == device.ProtocolStandardGATT {
		gattClient := ble.NewGATTClient(client, IsDebugMode())
		records, err = device.GetRecordsGATT(ctx, gattClient, driver, options)
	} else {
		handler := ble.NewTxRxHandler(client, IsDebugMode())
		records, err = device.GetRecords(ctx, handler, driver, options)
	}

	if err != nil {
		return fmt.Errorf("failed to read records: %w", err)
	}

	return outputRecords(records, cfg.Name)
}

func runSyncAutoMode(ctx context.Context, driver device.Driver) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("auto mode is only available on macOS")
	}

	cfg := driver.GetConfig()

	// Get UUID
	targetUUID := syncDeviceUUID
	if targetUUID == "" && syncDeviceMAC != "" {
		targetUUID = syncDeviceMAC
	}
	if targetUUID == "" {
		// Try loading from saved config
		savedUUID, err := config.GetDeviceUUID(cfg.Name)
		if err != nil {
			return fmt.Errorf("failed to load saved config: %w", err)
		}
		if savedUUID == "" {
			return fmt.Errorf("no saved UUID for %s. Run 'omblego pair' first, or use --uuid", cfg.Name)
		}
		targetUUID = savedUUID
	}

	slog.Info("using device UUID", "uuid", targetUUID)

	// Create auto client
	autoClient, err := ble.NewAutoClient(IsDebugMode())
	if err != nil {
		return fmt.Errorf("failed to create auto client: %w", err)
	}
	defer autoClient.Disconnect()

	// Wait for Bluetooth
	slog.Info("initializing Bluetooth...")
	if err := autoClient.WaitReady(ctx); err != nil {
		return fmt.Errorf("Bluetooth not ready: %w", err)
	}

	// Connect
	if syncAutoTimeout > 0 {
		slog.Info("waiting for device...", "timeout", syncAutoTimeout)
	} else {
		slog.Info("waiting for device (press Ctrl+C to cancel)...")
	}
	slog.Info("press the Bluetooth button on your device when ready")

	if err := autoClient.ConnectByUUID(ctx, targetUUID, syncAutoTimeout); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Discover services
	slog.Info("discovering services...")
	if err := autoClient.DiscoverOmronServices(ctx); err != nil {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	// Setup handler
	handler := ble.NewAutoTxRxHandler(autoClient, IsDebugMode())
	if err := handler.SetupNotifications(); err != nil {
		return fmt.Errorf("failed to setup notifications: %w", err)
	}

	// Wait for stabilization
	slog.Info("waiting for connection to stabilize...")
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Unlock device
	if err := unlockWithRetry(ctx, handler); err != nil {
		return err
	}

	// Start transmission
	if err := startTransmissionWithRetry(ctx, handler); err != nil {
		return err
	}

	// Read settings if needed for time sync
	var cachedSettings []byte
	if syncTimeSync {
		slog.Info("reading settings...")
		settingsSize := int(cfg.SettingsWriteAddress - cfg.SettingsReadAddress)
		cachedSettings, err = handler.ReadEEPROM(ctx, cfg.SettingsReadAddress, settingsSize, cfg.TransmissionBlockSize)
		if err != nil {
			slog.Warn("failed to read settings", "error", err)
		}
	}

	// Read records
	slog.Info("reading records...")
	totalSize := cfg.RecordsPerUser[0] * cfg.RecordByteSize
	recordData, err := handler.ReadEEPROM(ctx, cfg.UserStartAddresses[0], totalSize, cfg.TransmissionBlockSize)
	if err != nil {
		handler.EndTransmission(ctx)
		return fmt.Errorf("failed to read records: %w", err)
	}

	// Time sync
	if syncTimeSync && len(cachedSettings) > 0 {
		slog.Info("syncing time...")
		syncBytes, err := driver.BuildTimeSyncBytes(cachedSettings, time.Now())
		if err == nil {
			writeAddr := cfg.SettingsWriteAddress + uint16(cfg.TimeSyncRange[0])
			if err := handler.WriteEEPROM(ctx, writeAddr, syncBytes, len(syncBytes)); err != nil {
				slog.Warn("failed to sync time", "error", err)
			} else {
				slog.Info("time synchronized successfully")
			}
		}
	}

	handler.EndTransmission(ctx)

	// Parse records
	records := parseRecordsFromData(recordData, driver, cfg)
	slog.Info("records read", "user", 1, "count", len(records))

	if len(records) == 0 {
		slog.Info("no records found on device")
		return nil
	}

	return outputRecords([][]device.BloodPressureRecord{records}, cfg.Name)
}

func unlockWithRetry(ctx context.Context, handler *ble.AutoTxRxHandler) error {
	var err error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			slog.Info("retrying unlock...", "attempt", retry+1)
			time.Sleep(2 * time.Second)
		}
		if err = handler.UnlockWithKey(ctx, nil); err == nil {
			return nil
		}
		slog.Debug("unlock attempt failed", "attempt", retry+1, "error", err)
	}
	return fmt.Errorf("failed to unlock device: %w", err)
}

func startTransmissionWithRetry(ctx context.Context, handler *ble.AutoTxRxHandler) error {
	var err error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			slog.Info("retrying start transmission...", "attempt", retry+1)
			time.Sleep(1 * time.Second)
		}
		if err = handler.StartTransmission(ctx); err == nil {
			return nil
		}
		slog.Debug("start transmission failed", "attempt", retry+1, "error", err)
	}
	return fmt.Errorf("failed to start transmission: %w", err)
}

func parseRecordsFromData(recordData []byte, driver device.Driver, cfg device.Config) []device.BloodPressureRecord {
	var records []device.BloodPressureRecord

	for offset := 0; offset < len(recordData); offset += cfg.RecordByteSize {
		if offset+cfg.RecordByteSize > len(recordData) {
			break
		}

		data := recordData[offset : offset+cfg.RecordByteSize]

		// Skip empty records
		isEmpty := true
		for _, b := range data {
			if b != 0xFF {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			continue
		}

		record, err := driver.ParseRecord(data)
		if err != nil {
			slog.Debug("failed to parse record", "offset", offset, "error", err)
			continue
		}
		records = append(records, *record)
	}

	return records
}

func outputRecords(records [][]device.BloodPressureRecord, deviceName string) error {
	totalRecords := 0
	for userIdx, userRecords := range records {
		totalRecords += len(userRecords)
		slog.Info("user records", "user", userIdx+1, "count", len(userRecords))
	}

	if totalRecords == 0 {
		slog.Info("no records found on device")
		return nil
	}

	switch strings.ToLower(syncOutputFormat) {
	case "json":
		if err := output.WriteJSON(records); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
		slog.Info("records written", "file", "ubpm.json")
	case "both":
		if err := output.WriteCSV(records); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		if err := output.WriteJSON(records); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
		slog.Info("records written", "files", "user*.csv and ubpm.json")
	default:
		if err := output.WriteCSV(records); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		slog.Info("records written", "file", "user*.csv")
	}

	return nil
}

func interactiveScan(ctx context.Context, client *ble.Client) (string, error) {
	fmt.Println("\nTo improve your chance of a successful connection:")
	fmt.Println("  - Remove previous device pairings in your OS's Bluetooth settings")
	fmt.Println("  - Enable Bluetooth on your Omron device")
	fmt.Println("  - Press the Bluetooth button briefly")
	fmt.Println()

	fmt.Println("Scanning for Bluetooth devices...")

	devices, err := client.Scan(ctx, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("scan failed: %w", err)
	}

	if len(devices) == 0 {
		return "", fmt.Errorf("no Bluetooth devices found")
	}

	fmt.Println("\nFound devices:")
	fmt.Println("ID  | MAC Address        | Name                 | RSSI")
	fmt.Println("----+--------------------+----------------------+------")
	for i, dev := range devices {
		name := dev.Name
		if name == "" {
			name = "(unknown)"
		}
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		fmt.Printf("%-3d | %-18s | %-20s | %d\n", i, dev.Address, name, dev.RSSI)
	}

	fmt.Print("\nEnter device ID: ")
	var idx int
	if _, err := fmt.Scanf("%d", &idx); err != nil || idx < 0 || idx >= len(devices) {
		return "", fmt.Errorf("invalid selection")
	}

	return devices[idx].Address, nil
}
