package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"omblego/internal/ble"
)

var (
	scanDuration time.Duration
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for nearby Bluetooth devices",
	Long: `Scans for nearby Bluetooth Low Energy devices.

This helps identify Omron devices and their addresses for pairing.
Look for devices with names starting with "BLEsmart_" or similar.

Examples:
  # Quick scan (5 seconds)
  omblego scan

  # Longer scan
  omblego scan --duration 15s`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().DurationVar(&scanDuration, "duration", 5*time.Second, "Scan duration")

	AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		slog.Info("interrupted, stopping scan...")
		cancel()
	}()

	slog.Info("initializing Bluetooth...")
	client, err := ble.NewClient(IsDebugMode())
	if err != nil {
		return fmt.Errorf("failed to initialize Bluetooth: %w", err)
	}
	defer client.Close()

	fmt.Printf("Scanning for %v...\n\n", scanDuration)

	devices, err := client.Scan(ctx, scanDuration)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No Bluetooth devices found.")
		fmt.Println("\nTips:")
		fmt.Println("  - Make sure the device is powered on")
		fmt.Println("  - Press the Bluetooth button on the device")
		fmt.Println("  - For pairing, hold the button until -P- appears")
		return nil
	}

	// Sort by RSSI (signal strength)
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].RSSI > devices[j].RSSI
	})

	fmt.Printf("Found %d device(s):\n\n", len(devices))
	fmt.Println("ID  | MAC Address        | Name                           | RSSI")
	fmt.Println("----+--------------------+--------------------------------+------")

	for i, dev := range devices {
		name := dev.Name
		if name == "" {
			name = "(unknown)"
		}
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		fmt.Printf("%-3d | %-18s | %-30s | %d dBm\n", i, dev.Address, name, dev.RSSI)
	}

	fmt.Println("\nTo pair with a device, run:")
	fmt.Println("  omblego pair -d <DEVICE_MODEL> --mac <MAC_ADDRESS>")
	fmt.Println("\nExample:")
	if len(devices) > 0 {
		fmt.Printf("  omblego pair -d BP7000 --mac %s\n", devices[0].Address)
	}

	return nil
}
