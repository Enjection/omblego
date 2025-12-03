package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"omblego/internal/ble"
	"omblego/internal/config"
	"omblego/internal/device"
	"omblego/internal/output"

	// Import device drivers to register them
	_ "omblego/internal/device/devices"
)

var (
	deviceName   string
	macAddress   string
	pairMode     bool
	autoMode     bool
	autoTimeout  time.Duration
	newRecOnly   bool
	timeSync     bool
	debugMode    bool
	outputFormat string

	// Daemon mode
	daemonMode     bool
	daemonInterval time.Duration

	// InfluxDB
	influxURL    string
	influxToken  string
	influxOrg    string
	influxBucket string

	// Config file
	configFile string
	deviceUUID string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "omblego",
		Short: "Read records from Omron BLE blood pressure monitors",
		Long: `omblego is a CLI tool to read blood pressure measurement records
from Omron Bluetooth Low Energy (BLE) medical devices.

Supported devices: HEM-7322T, HEM-7600T, HEM-6232T, HEM-7530T,
                   HEM-7155T, HEM-7150T, HEM-7342T, HEM-7361T,
                   BP7000, BP5250, EVOLV

First-time pairing (device must show -P- on display):
  omblego -d BP7000 -p

Normal usage after pairing:
  omblego -d BP7000

Auto mode (energy-efficient, waits for device - macOS only):
  omblego -d BP7000 --auto -t

Daemon mode (runs continuously, syncs on interval, pushes to InfluxDB):
  omblego -d BP7000 --daemon --interval 3h \
    --influx-url http://localhost:8086 \
    --influx-token YOUR_TOKEN \
    --influx-org myorg --influx-bucket health`,
		RunE: run,
	}

	rootCmd.Flags().StringVarP(&deviceName, "device", "d", "", "Device model name (e.g., BP7000)")
	rootCmd.Flags().StringVarP(&macAddress, "mac", "m", "", "Bluetooth MAC address (skip device scan)")
	rootCmd.Flags().BoolVarP(&pairMode, "pair", "p", false, "Enable pairing mode (first-time setup)")
	rootCmd.Flags().BoolVarP(&autoMode, "auto", "a", false, "Auto mode: wait passively for device (macOS only)")
	rootCmd.Flags().DurationVar(&autoTimeout, "timeout", 0, "Timeout for auto mode (0 = wait indefinitely)")
	rootCmd.Flags().BoolVarP(&newRecOnly, "newRecOnly", "n", false, "Only read new/unread records")
	rootCmd.Flags().BoolVarP(&timeSync, "timeSync", "t", false, "Synchronize device time with system time")
	rootCmd.Flags().BoolVar(&debugMode, "debug", false, "Enable debug logging")
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "csv", "Output format: csv, json, or both")

	// Daemon mode flags
	rootCmd.Flags().BoolVar(&daemonMode, "daemon", false, "Run as daemon, sync on interval")
	rootCmd.Flags().DurationVar(&daemonInterval, "interval", 3*time.Hour, "Sync interval for daemon mode")

	// InfluxDB flags
	rootCmd.Flags().StringVar(&influxURL, "influx-url", "", "InfluxDB URL (e.g., http://localhost:8086)")
	rootCmd.Flags().StringVar(&influxToken, "influx-token", "", "InfluxDB API token")
	rootCmd.Flags().StringVar(&influxOrg, "influx-org", "", "InfluxDB organization")
	rootCmd.Flags().StringVar(&influxBucket, "influx-bucket", "", "InfluxDB bucket")

	// Config file
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Config file path (overrides CLI flags)")
	rootCmd.Flags().StringVar(&deviceUUID, "uuid", "", "Device UUID (overrides saved UUID)")

	// Add list-devices subcommand
	listCmd := &cobra.Command{
		Use:   "list-devices",
		Short: "List all supported Omron devices",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Supported Omron devices:")
			for _, name := range device.ListDevices() {
				fmt.Printf("  - %s\n", name)
			}
		},
	}
	rootCmd.AddCommand(listCmd)

	// Add version subcommand
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("omblego v0.1.0")
			fmt.Println("Cross-platform Omron BLE blood pressure monitor reader")
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Interrupted, cleaning up...")
		cancel()
	}()

	// Load config file if provided
	if configFile != "" {
		cfg, err := config.LoadFromFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		// Apply config file settings (CLI flags override)
		if deviceName == "" && cfg.Device != "" {
			deviceName = cfg.Device
		}
		if deviceUUID == "" && cfg.UUID != "" {
			deviceUUID = cfg.UUID
		}
		if cfg.InfluxDB != nil {
			if influxURL == "" {
				influxURL = cfg.InfluxDB.URL
			}
			if influxToken == "" {
				influxToken = cfg.InfluxDB.Token
			}
			if influxOrg == "" {
				influxOrg = cfg.InfluxDB.Org
			}
			if influxBucket == "" {
				influxBucket = cfg.InfluxDB.Bucket
			}
		}
		if cfg.Daemon != nil {
			if !daemonMode && cfg.Daemon.Interval != "" {
				// If config has daemon settings but --daemon not set, hint at it
			}
			if !cmd.Flags().Changed("interval") && cfg.Daemon.Interval != "" {
				daemonInterval = cfg.GetInterval()
			}
			if !cmd.Flags().Changed("timeSync") && cfg.Daemon.TimeSync {
				timeSync = cfg.Daemon.TimeSync
			}
		}

		log.Printf("Loaded config from: %s", configFile)
	}

	// Validate device name
	if deviceName == "" {
		return fmt.Errorf("device name is required (use -d or set in config file)")
	}

	// Get device driver
	driver, err := device.GetDriver(deviceName)
	if err != nil {
		return err
	}
	log.Printf("Using device driver: %s", driver.GetConfig().Name)

	// Daemon mode - run continuously on interval
	if daemonMode {
		return runDaemonMode(ctx, driver)
	}

	// Auto mode - use energy-efficient passive reconnect (macOS only)
	if autoMode {
		return runAutoMode(ctx, driver)
	}

	// Initialize BLE client
	log.Println("Initializing Bluetooth...")
	client, err := ble.NewClient(debugMode)
	if err != nil {
		return fmt.Errorf("failed to initialize Bluetooth: %w", err)
	}
	defer client.Close()

	// Get device address
	var deviceAddr string
	if macAddress != "" {
		deviceAddr = macAddress
	} else {
		var err error
		deviceAddr, err = scanAndSelectDevice(ctx, client)
		if err != nil {
			return err
		}
	}

	// Connect to device
	log.Printf("Connecting to %s...", deviceAddr)
	if err := client.Connect(ctx, deviceAddr); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer client.Disconnect()

	// Create TX/RX handler for custom Omron protocol
	handler := ble.NewTxRxHandler(client, debugMode)

	if pairMode {
		if driver.GetConfig().Protocol == device.ProtocolStandardGATT {
			// GATT devices use standard BLE pairing
			log.Println("Pairing with GATT device...")
			log.Println("Accept any OS Bluetooth pairing dialogs that appear...")
			gattClient := ble.NewGATTClient(client, debugMode)

			// Discover services - this may trigger OS pairing dialog
			log.Println("Discovering services...")
			if err := gattClient.DiscoverServices(ctx); err != nil {
				return fmt.Errorf("failed to discover services: %w", err)
			}

			// Enable notifications - this triggers the pairing handshake
			log.Println("Enabling notifications...")
			if err := gattClient.EnableNotifications(ctx); err != nil {
				return fmt.Errorf("failed to enable notifications: %w", err)
			}

			// Wait for pairing handshake to complete (device sends time/battery notification)
			log.Println("Waiting for device pairing response (up to 30 seconds)...")
			if err := gattClient.WaitForPairing(ctx, 30*time.Second); err != nil {
				return fmt.Errorf("pairing failed: %w", err)
			}

			// IMPORTANT: Omron devices expect to disconnect themselves after pairing
			// We must wait for the device to disconnect, not disconnect ourselves
			log.Println("Waiting for device to complete pairing (device will disconnect)...")
			if err := gattClient.WaitForDeviceDisconnect(ctx, 15*time.Second); err != nil {
				if debugMode {
					log.Printf("Note: %v", err)
				}
				// Not a fatal error - device may have already disconnected
			}

			// Save UUID for auto mode
			if err := config.SaveDeviceUUID(driver.GetConfig().Name, deviceAddr, deviceAddr); err != nil {
				log.Printf("Warning: failed to save UUID: %v", err)
			} else {
				log.Printf("Saved device UUID: %s", deviceAddr)
			}

			log.Println("Device paired successfully!")
			log.Printf("You can now run: omblego -d %s --auto -t", driver.GetConfig().Name)
			return nil
		}

		// Custom Omron protocol pairing - needs initial wait for OS dialog
		log.Println("Waiting for OS Bluetooth pairing... (accept any system dialogs)")
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

		log.Println("Programming pairing key...")
		if err := handler.WriteNewUnlockKey(ctx, nil); err != nil {
			return fmt.Errorf("pairing failed: %w", err)
		}

		// Do a start/end transmission to verify
		if err := handler.StartTransmission(ctx); err != nil {
			log.Printf("Warning: start transmission after pairing failed: %v", err)
		} else {
			handler.EndTransmission(ctx)
		}

		// Save UUID for auto mode
		if err := config.SaveDeviceUUID(driver.GetConfig().Name, deviceAddr, deviceAddr); err != nil {
			log.Printf("Warning: failed to save UUID: %v", err)
		} else {
			log.Printf("Saved device UUID: %s", deviceAddr)
		}

		log.Println("Device paired successfully!")
		log.Printf("You can now run: omblego -d %s --auto -t", driver.GetConfig().Name)
		return nil
	}

	// For non-pairing mode with custom protocol, wait for OS
	if driver.GetConfig().Protocol != device.ProtocolStandardGATT {
		log.Println("Waiting for OS Bluetooth connection...")
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Read records based on device protocol type
	log.Println("Reading records from device...")
	options := device.ReadOptions{
		UseUnreadCounter: newRecOnly,
		SyncTime:         timeSync,
		Debug:            debugMode,
	}

	var records [][]device.BloodPressureRecord

	if driver.GetConfig().Protocol == device.ProtocolStandardGATT {
		// Use standard GATT protocol
		gattClient := ble.NewGATTClient(client, debugMode)
		records, err = device.GetRecordsGATT(ctx, gattClient, driver, options)
		if err != nil {
			return fmt.Errorf("failed to read records: %w", err)
		}
	} else {
		// Use custom Omron protocol
		records, err = device.GetRecords(ctx, handler, driver, options)
		if err != nil {
			return fmt.Errorf("failed to read records: %w", err)
		}
	}

	// Count total records
	totalRecords := 0
	for userIdx, userRecords := range records {
		totalRecords += len(userRecords)
		log.Printf("User %d: %d records", userIdx+1, len(userRecords))
	}

	if totalRecords == 0 {
		log.Println("No records found on device")
		return nil
	}

	// Output records
	switch strings.ToLower(outputFormat) {
	case "json":
		if err := output.WriteJSON(records); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
		log.Println("Records written to ubpm.json")
	case "both":
		if err := output.WriteCSV(records); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		if err := output.WriteJSON(records); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
		log.Println("Records written to user*.csv and ubpm.json")
	default: // csv
		if err := output.WriteCSV(records); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		log.Println("Records written to user*.csv")
	}

	return nil
}

func scanAndSelectDevice(ctx context.Context, client *ble.Client) (string, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	fmt.Println("\nTo improve your chance of a successful connection:")
	fmt.Println("  - Remove previous device pairings in your OS's Bluetooth settings")
	fmt.Println("  - Enable Bluetooth on your Omron device")
	fmt.Println("  - For pairing (-p): Hold the Bluetooth button until you see -P-")
	fmt.Println("  - For normal use: Press the Bluetooth button briefly")
	fmt.Println("  - Do not accept any pairing dialog until you select your device")
	fmt.Println()

	fmt.Println("Scanning for Bluetooth devices...")

	devices, err := client.Scan(ctx, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("scan failed: %w", err)
	}

	// Check context after scan
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if len(devices) == 0 {
		return "", fmt.Errorf("no Bluetooth devices found")
	}

	// Sort by RSSI (signal strength)
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].RSSI > devices[j].RSSI
	})

	// Display devices
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

	// Get user selection with context awareness
	inputCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		inputCh <- strings.TrimSpace(input)
	}()

	for {
		fmt.Print("\nEnter device ID (or press Enter to rescan): ")

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case err := <-errCh:
			return "", err
		case input := <-inputCh:
			if input == "" {
				// Rescan
				fmt.Println("Rescanning...")
				return scanAndSelectDevice(ctx, client)
			}

			idx, err := strconv.Atoi(input)
			if err != nil || idx < 0 || idx >= len(devices) {
				fmt.Printf("Invalid selection. Enter a number between 0 and %d\n", len(devices)-1)
				// Start new input reader
				go func() {
					reader := bufio.NewReader(os.Stdin)
					input, err := reader.ReadString('\n')
					if err != nil {
						errCh <- err
						return
					}
					inputCh <- strings.TrimSpace(input)
				}()
				continue
			}

			return devices[idx].Address, nil
		}
	}
}

// runAutoMode uses energy-efficient passive BLE reconnection (macOS only)
func runAutoMode(ctx context.Context, driver device.Driver) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("auto mode is only available on macOS")
	}

	cfg := driver.GetConfig()

	// Get UUID from --uuid flag, --mac flag, or saved config
	var targetUUID string
	if deviceUUID != "" {
		targetUUID = deviceUUID
	} else if macAddress != "" {
		targetUUID = macAddress
		// Save it for future use
		if err := config.SaveDeviceUUID(cfg.Name, macAddress, macAddress); err != nil {
			log.Printf("Warning: failed to save UUID: %v", err)
		}
	} else {
		savedUUID, err := config.GetDeviceUUID(cfg.Name)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if savedUUID == "" {
			return fmt.Errorf("no saved UUID for %s. Run with --pair first, or use --uuid to specify UUID", cfg.Name)
		}
		targetUUID = savedUUID
	}

	log.Printf("Using UUID: %s", targetUUID)

	// Create auto client
	autoClient, err := ble.NewAutoClient(debugMode)
	if err != nil {
		return fmt.Errorf("failed to create auto client: %w", err)
	}
	defer autoClient.Disconnect()

	// Wait for Bluetooth to be ready
	log.Println("Initializing Bluetooth...")
	if err := autoClient.WaitReady(ctx); err != nil {
		return fmt.Errorf("Bluetooth not ready: %w", err)
	}

	// Connect passively - waits for device to advertise
	if autoTimeout > 0 {
		log.Printf("Waiting for device (timeout: %v)...", autoTimeout)
	} else {
		log.Println("Waiting for device (press Ctrl+C to cancel)...")
	}
	log.Println("Press the Bluetooth button on your device when ready")
	if err := autoClient.ConnectByUUID(ctx, targetUUID, autoTimeout); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Discover Omron services
	log.Println("Discovering services...")
	if err := autoClient.DiscoverOmronServices(ctx); err != nil {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	// Create handler and setup notifications
	handler := ble.NewAutoTxRxHandler(autoClient, debugMode)
	if err := handler.SetupNotifications(); err != nil {
		return fmt.Errorf("failed to setup notifications: %w", err)
	}

	// Wait for connection to fully stabilize (same as normal mode)
	log.Println("Waiting for connection to stabilize...")
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Unlock device with retry
	var unlockErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			log.Printf("Retrying unlock (attempt %d/3)...", retry+1)
			time.Sleep(2 * time.Second)
		}
		if unlockErr = handler.UnlockWithKey(ctx, nil); unlockErr == nil {
			break
		}
		if debugMode {
			log.Printf("Unlock attempt %d failed: %v", retry+1, unlockErr)
		}
	}
	if unlockErr != nil {
		return fmt.Errorf("failed to unlock device: %w", unlockErr)
	}

	// Start transmission with retry
	var startErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			log.Printf("Retrying start transmission (attempt %d/3)...", retry+1)
			time.Sleep(1 * time.Second)
		}
		if startErr = handler.StartTransmission(ctx); startErr == nil {
			break
		}
		if debugMode {
			log.Printf("Start transmission attempt %d failed: %v", retry+1, startErr)
		}
	}
	if startErr != nil {
		return fmt.Errorf("failed to start transmission: %w", startErr)
	}

	// Read settings if needed
	var cachedSettings []byte
	if timeSync {
		log.Println("Reading settings...")
		settingsSize := int(cfg.SettingsWriteAddress - cfg.SettingsReadAddress)
		cachedSettings, err = handler.ReadEEPROM(ctx, cfg.SettingsReadAddress, settingsSize, cfg.TransmissionBlockSize)
		if err != nil {
			log.Printf("Warning: failed to read settings: %v", err)
		}
	}

	// Read records
	log.Println("Reading records...")
	totalSize := cfg.RecordsPerUser[0] * cfg.RecordByteSize
	recordData, err := handler.ReadEEPROM(ctx, cfg.UserStartAddresses[0], totalSize, cfg.TransmissionBlockSize)
	if err != nil {
		handler.EndTransmission(ctx)
		return fmt.Errorf("failed to read records: %w", err)
	}

	// Time sync if requested
	if timeSync && len(cachedSettings) > 0 {
		log.Println("Syncing time...")
		syncBytes, err := driver.BuildTimeSyncBytes(cachedSettings, time.Now())
		if err != nil {
			log.Printf("Warning: failed to build time sync bytes: %v", err)
		} else {
			writeAddr := cfg.SettingsWriteAddress + uint16(cfg.TimeSyncRange[0])
			if err := handler.WriteEEPROM(ctx, writeAddr, syncBytes, len(syncBytes)); err != nil {
				log.Printf("Warning: failed to write time sync: %v", err)
			} else {
				log.Println("Time synchronized successfully")
			}
		}
	}

	// End transmission
	handler.EndTransmission(ctx)

	// Parse records
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
			if debugMode {
				log.Printf("Warning: failed to parse record at offset %d: %v", offset, err)
			}
			continue
		}
		records = append(records, *record)
	}

	log.Printf("User 1: %d records", len(records))

	if len(records) == 0 {
		log.Println("No records found on device")
		return nil
	}

	// Output records
	allRecords := [][]device.BloodPressureRecord{records}
	switch strings.ToLower(outputFormat) {
	case "json":
		if err := output.WriteJSON(allRecords); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
		log.Println("Records written to ubpm.json")
	case "both":
		if err := output.WriteCSV(allRecords); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		if err := output.WriteJSON(allRecords); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
		log.Println("Records written to user*.csv and ubpm.json")
	default:
		if err := output.WriteCSV(allRecords); err != nil {
			return fmt.Errorf("failed to write CSV: %w", err)
		}
		log.Println("Records written to user1.csv")
	}

	return nil
}

// runDaemonMode runs as a daemon, syncing on interval and pushing to InfluxDB
func runDaemonMode(ctx context.Context, driver device.Driver) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("daemon mode requires auto mode which is only available on macOS")
	}

	// Validate InfluxDB config if provided
	var influxWriter *output.InfluxDBWriter
	if influxURL != "" {
		if influxToken == "" || influxOrg == "" || influxBucket == "" {
			return fmt.Errorf("InfluxDB requires --influx-url, --influx-token, --influx-org, and --influx-bucket")
		}

		influxWriter = output.NewInfluxDBWriter(output.InfluxDBConfig{
			URL:    influxURL,
			Token:  influxToken,
			Org:    influxOrg,
			Bucket: influxBucket,
		})
		defer influxWriter.Close()

		// Test connection
		if err := influxWriter.Ping(ctx); err != nil {
			return fmt.Errorf("failed to connect to InfluxDB: %w", err)
		}
		log.Println("Connected to InfluxDB")
	}

	sentTracker := output.NewSentTracker()
	cfg := driver.GetConfig()

	log.Printf("Starting daemon mode (interval: %v)", daemonInterval)
	if influxWriter != nil {
		log.Printf("Pushing to InfluxDB: %s (org=%s, bucket=%s)", influxURL, influxOrg, influxBucket)
	}

	// Run sync immediately on start
	if err := runDaemonSync(ctx, driver, cfg, influxWriter, sentTracker); err != nil {
		log.Printf("Initial sync failed: %v", err)
	}

	// Then run on interval
	ticker := time.NewTicker(daemonInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Daemon stopped")
			return nil
		case <-ticker.C:
			log.Printf("Running scheduled sync...")
			if err := runDaemonSync(ctx, driver, cfg, influxWriter, sentTracker); err != nil {
				log.Printf("Sync failed: %v", err)
				// Continue running, don't exit on error
			}
		}
	}
}

// runDaemonSync performs a single sync in daemon mode
func runDaemonSync(ctx context.Context, driver device.Driver, cfg device.Config, influxWriter *output.InfluxDBWriter, sentTracker *output.SentTracker) error {
	// Create auto client
	autoClient, err := ble.NewAutoClient(debugMode)
	if err != nil {
		return fmt.Errorf("failed to create auto client: %w", err)
	}
	defer autoClient.Disconnect()

	// Get UUID from --uuid flag or saved config
	var targetUUID string
	if deviceUUID != "" {
		targetUUID = deviceUUID
	} else {
		savedUUID, err := config.GetDeviceUUID(cfg.Name)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if savedUUID == "" {
			return fmt.Errorf("no saved UUID for %s. Run with --pair first, or use --uuid", cfg.Name)
		}
		targetUUID = savedUUID
	}

	log.Printf("Using UUID: %s", targetUUID)

	// Wait for Bluetooth to be ready
	if err := autoClient.WaitReady(ctx); err != nil {
		return fmt.Errorf("Bluetooth not ready: %w", err)
	}

	// Connect with timeout for daemon mode
	syncTimeout := 2 * time.Minute
	log.Printf("Waiting for device (timeout: %v)...", syncTimeout)
	if err := autoClient.ConnectByUUID(ctx, targetUUID, syncTimeout); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Discover services
	if err := autoClient.DiscoverOmronServices(ctx); err != nil {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	// Setup handler
	handler := ble.NewAutoTxRxHandler(autoClient, debugMode)
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
	if timeSync {
		settingsSize := int(cfg.SettingsWriteAddress - cfg.SettingsReadAddress)
		cachedSettings, err := handler.ReadEEPROM(ctx, cfg.SettingsReadAddress, settingsSize, cfg.TransmissionBlockSize)
		if err == nil {
			syncBytes, err := driver.BuildTimeSyncBytes(cachedSettings, time.Now())
			if err == nil {
				writeAddr := cfg.SettingsWriteAddress + uint16(cfg.TimeSyncRange[0])
				if err := handler.WriteEEPROM(ctx, writeAddr, syncBytes, len(syncBytes)); err != nil {
					log.Printf("Warning: time sync failed: %v", err)
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
	var records []device.BloodPressureRecord
	for offset := 0; offset < len(recordData); offset += cfg.RecordByteSize {
		if offset+cfg.RecordByteSize > len(recordData) {
			break
		}

		data := recordData[offset : offset+cfg.RecordByteSize]

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
			continue
		}
		records = append(records, *record)
	}

	log.Printf("Read %d records from device", len(records))

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
		log.Println("No new records to push")
		return nil
	}

	log.Printf("Found %d new records", newCount)

	// Push to InfluxDB
	if influxWriter != nil {
		if err := influxWriter.WriteRecords(ctx, newRecords, cfg.Name); err != nil {
			return fmt.Errorf("failed to write to InfluxDB: %w", err)
		}
		log.Printf("Pushed %d records to InfluxDB", newCount)
	}

	// Also write to CSV (append new records)
	if err := output.WriteCSV(allRecords); err != nil {
		log.Printf("Warning: failed to write CSV: %v", err)
	}

	// Update tracker
	sentTracker.UpdateLastSent(newRecords)

	return nil
}
