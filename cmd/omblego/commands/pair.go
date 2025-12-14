package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"omblego/internal/ble"
	"omblego/internal/config"
	"omblego/internal/device"
)

var (
	pairDeviceName string
	pairDeviceMAC  string
	pairInteractive bool
)

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair with a new Omron device",
	Long: `Initiates pairing with an Omron BLE blood pressure monitor.

The device must be in pairing mode (showing -P- on display).
To enter pairing mode, hold the Bluetooth button until -P- appears.

After pairing, you can use 'omblego sync' to read records.

Examples:
  # Interactive pairing (scan for devices)
  omblego pair -d BP7000 --interactive

  # Pair with known MAC address
  omblego pair -d BP7000 --mac 00:11:22:33:44:55`,
	RunE: runPair,
}

func init() {
	pairCmd.Flags().StringVarP(&pairDeviceName, "device", "d", "", "Device model name (e.g., BP7000)")
	pairCmd.Flags().StringVarP(&pairDeviceMAC, "mac", "m", "", "Device MAC address")
	pairCmd.Flags().BoolVarP(&pairInteractive, "interactive", "i", true, "Interactive mode: scan and select device")

	pairCmd.MarkFlagRequired("device")

	AddCommand(pairCmd)
}

func runPair(cmd *cobra.Command, args []string) error {
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

	// Get device driver
	driver, err := device.GetDriver(pairDeviceName)
	if err != nil {
		return fmt.Errorf("unsupported device: %w", err)
	}
	slog.Info("using device driver", "device", driver.GetConfig().Name)

	// Initialize Bluetooth
	slog.Info("initializing Bluetooth...")
	client, err := ble.NewClient(IsDebugMode())
	if err != nil {
		return fmt.Errorf("failed to initialize Bluetooth: %w", err)
	}
	defer client.Close()

	// Get device address
	var deviceAddr string
	if pairDeviceMAC != "" {
		deviceAddr = pairDeviceMAC
	} else if pairInteractive {
		deviceAddr, err = interactivePairScan(ctx, client)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("specify --mac or use --interactive for device selection")
	}

	// Connect
	slog.Info("connecting to device...", "address", deviceAddr)
	if err := client.Connect(ctx, deviceAddr); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer client.Disconnect()

	cfg := driver.GetConfig()

	// Handle pairing based on protocol type
	if cfg.Protocol == device.ProtocolStandardGATT {
		return pairGATTDevice(ctx, client, driver, deviceAddr)
	}
	return pairCustomProtocolDevice(ctx, client, driver, deviceAddr)
}

func pairGATTDevice(ctx context.Context, client *ble.Client, driver device.Driver, deviceAddr string) error {
	slog.Info("pairing with GATT device...")
	slog.Info("accept any OS Bluetooth pairing dialogs that appear...")

	gattClient := ble.NewGATTClient(client, IsDebugMode())

	// Discover services
	slog.Info("discovering services...")
	if err := gattClient.DiscoverServices(ctx); err != nil {
		return fmt.Errorf("failed to discover services: %w", err)
	}

	// Enable notifications
	slog.Info("enabling notifications...")
	if err := gattClient.EnableNotifications(ctx); err != nil {
		return fmt.Errorf("failed to enable notifications: %w", err)
	}

	// Wait for pairing
	slog.Info("waiting for device pairing response (up to 30 seconds)...")
	if err := gattClient.WaitForPairing(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("pairing failed: %w", err)
	}

	// Wait for device to disconnect
	slog.Info("waiting for device to complete pairing (device will disconnect)...")
	if err := gattClient.WaitForDeviceDisconnect(ctx, 15*time.Second); err != nil {
		slog.Debug("device disconnect note", "error", err)
	}

	// Save UUID
	if err := config.SaveDeviceUUID(driver.GetConfig().Name, deviceAddr, deviceAddr); err != nil {
		slog.Warn("failed to save UUID", "error", err)
	} else {
		slog.Info("saved device UUID", "uuid", deviceAddr)
	}

	slog.Info("device paired successfully!")
	fmt.Printf("You can now run: omblego sync -d %s --auto\n", driver.GetConfig().Name)
	return nil
}

func pairCustomProtocolDevice(ctx context.Context, client *ble.Client, driver device.Driver, deviceAddr string) error {
	slog.Info("waiting for OS Bluetooth pairing... (accept any system dialogs)")
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	handler := ble.NewTxRxHandler(client, IsDebugMode())

	slog.Info("programming pairing key...")
	if err := handler.WriteNewUnlockKey(ctx, nil); err != nil {
		return fmt.Errorf("pairing failed: %w", err)
	}

	// Verify pairing
	if err := handler.StartTransmission(ctx); err != nil {
		slog.Warn("verification note", "error", err)
	} else {
		handler.EndTransmission(ctx)
	}

	// Save UUID
	if err := config.SaveDeviceUUID(driver.GetConfig().Name, deviceAddr, deviceAddr); err != nil {
		slog.Warn("failed to save UUID", "error", err)
	} else {
		slog.Info("saved device UUID", "uuid", deviceAddr)
	}

	slog.Info("device paired successfully!")
	fmt.Printf("You can now run: omblego sync -d %s --auto\n", driver.GetConfig().Name)
	return nil
}

func interactivePairScan(ctx context.Context, client *ble.Client) (string, error) {
	fmt.Println("\nTo pair your Omron device:")
	fmt.Println("  1. Remove any existing pairings in your OS's Bluetooth settings")
	fmt.Println("  2. On the device, hold the Bluetooth button until -P- appears")
	fmt.Println("  3. Wait for the device to appear in the scan below")
	fmt.Println()

	fmt.Println("Scanning for Bluetooth devices...")

	devices, err := client.Scan(ctx, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("scan failed: %w", err)
	}

	if len(devices) == 0 {
		return "", fmt.Errorf("no Bluetooth devices found. Make sure the device is in pairing mode (-P-)")
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

	fmt.Print("\nEnter device ID (the one showing -P- on display): ")
	var idx int
	if _, err := fmt.Scanf("%d", &idx); err != nil || idx < 0 || idx >= len(devices) {
		return "", fmt.Errorf("invalid selection")
	}

	return devices[idx].Address, nil
}
