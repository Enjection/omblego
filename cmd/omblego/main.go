package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"omblego/internal/ble"
	"omblego/internal/device"
	"omblego/internal/output"

	// Import device drivers to register them
	_ "omblego/internal/device/devices"
)

var (
	deviceName   string
	macAddress   string
	pairMode     bool
	newRecOnly   bool
	timeSync     bool
	debugMode    bool
	outputFormat string
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
  omblego -d HEM-7322T -p

Normal usage after pairing:
  omblego -d HEM-7322T`,
		RunE: run,
	}

	rootCmd.Flags().StringVarP(&deviceName, "device", "d", "", "Device model name (e.g., HEM-7322T)")
	rootCmd.Flags().StringVarP(&macAddress, "mac", "m", "", "Bluetooth MAC address (skip device scan)")
	rootCmd.Flags().BoolVarP(&pairMode, "pair", "p", false, "Enable pairing mode (first-time setup)")
	rootCmd.Flags().BoolVarP(&newRecOnly, "newRecOnly", "n", false, "Only read new/unread records")
	rootCmd.Flags().BoolVarP(&timeSync, "timeSync", "t", false, "Synchronize device time with system time")
	rootCmd.Flags().BoolVar(&debugMode, "debug", false, "Enable debug logging")
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "csv", "Output format: csv, json, or both")

	rootCmd.MarkFlagRequired("device")

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

	// Get device driver
	driver, err := device.GetDriver(deviceName)
	if err != nil {
		return err
	}
	log.Printf("Using device driver: %s", driver.GetConfig().Name)

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

			log.Println("Device paired successfully!")
			log.Println("You can now run omblego without the -p flag.")
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

		log.Println("Device paired successfully!")
		log.Println("You can now run omblego without the -p flag.")
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
