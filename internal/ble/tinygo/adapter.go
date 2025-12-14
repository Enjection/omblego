// Package tinygo provides BLE adapter implementation using tinygo.org/x/bluetooth.
package tinygo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"

	"omblego/internal/ble"
)

// TinyGoAdapter implements ble.Adapter using TinyGo's bluetooth library.
type TinyGoAdapter struct {
	adapter *bluetooth.Adapter
	enabled bool
	debug   bool
	logger  *slog.Logger
	mu      sync.Mutex
}

// NewAdapter creates a new TinyGo BLE adapter.
func NewAdapter(debug bool) *TinyGoAdapter {
	return &TinyGoAdapter{
		adapter: bluetooth.DefaultAdapter,
		debug:   debug,
		logger:  slog.Default(),
	}
}

// NewAdapterWithLogger creates a new TinyGo BLE adapter with a custom logger.
func NewAdapterWithLogger(debug bool, logger *slog.Logger) *TinyGoAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &TinyGoAdapter{
		adapter: bluetooth.DefaultAdapter,
		debug:   debug,
		logger:  logger,
	}
}

// Enable initializes the BLE adapter.
func (a *TinyGoAdapter) Enable(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.enabled {
		return nil
	}

	if err := a.adapter.Enable(); err != nil {
		return fmt.Errorf("failed to enable bluetooth adapter: %w", err)
	}

	a.enabled = true
	a.logger.Debug("BLE adapter enabled")
	return nil
}

// Scan discovers nearby BLE devices.
func (a *TinyGoAdapter) Scan(ctx context.Context, duration time.Duration) ([]ble.ScanResult, error) {
	if !a.enabled {
		if err := a.Enable(ctx); err != nil {
			return nil, err
		}
	}

	var results []ble.ScanResult
	var mu sync.Mutex
	seen := make(map[string]bool)

	scanCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	scanErr := make(chan error, 1)
	go func() {
		err := a.adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
			mu.Lock()
			defer mu.Unlock()

			addr := device.Address.String()
			if addr == "" || seen[addr] {
				return
			}

			seen[addr] = true
			results = append(results, ble.ScanResult{
				Address: addr,
				Name:    device.LocalName(),
				RSSI:    device.RSSI,
			})

			if a.debug {
				a.logger.Debug("discovered device", "address", addr, "name", device.LocalName(), "rssi", device.RSSI)
			}
		})
		scanErr <- err
	}()

	select {
	case <-scanCtx.Done():
		a.adapter.StopScan()
		select {
		case <-scanErr:
		case <-time.After(500 * time.Millisecond):
		}
	case err := <-scanErr:
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return results, nil
}

// Connect establishes a connection to a device.
func (a *TinyGoAdapter) Connect(ctx context.Context, address string) (ble.Connection, error) {
	if !a.enabled {
		if err := a.Enable(ctx); err != nil {
			return nil, err
		}
	}

	a.logger.Debug("connecting to device", "address", address)

	var addr bluetooth.Address
	addr.Set(address)

	device, err := a.adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	conn := &TinyGoConnection{
		device:  device,
		address: address,
		chars:   make(map[string]bluetooth.DeviceCharacteristic),
		debug:   a.debug,
		logger:  a.logger,
	}

	// Discover all services
	services, err := device.DiscoverServices(nil)
	if err != nil {
		device.Disconnect()
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}
	conn.services = services

	a.logger.Debug("connected", "address", address, "services", len(services))

	return conn, nil
}

// Close releases adapter resources.
func (a *TinyGoAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.enabled = false
	return nil
}

// TinyGoConnection implements ble.Connection using TinyGo's bluetooth library.
type TinyGoConnection struct {
	device    bluetooth.Device
	address   string
	services  []bluetooth.DeviceService
	chars     map[string]bluetooth.DeviceCharacteristic
	connected bool
	debug     bool
	logger    *slog.Logger
	mu        sync.RWMutex
}

// Address returns the device address.
func (c *TinyGoConnection) Address() string {
	return c.address
}

// DiscoverServices discovers all available GATT services.
func (c *TinyGoConnection) DiscoverServices(ctx context.Context) ([]ble.Service, error) {
	var result []ble.Service
	for _, svc := range c.services {
		result = append(result, ble.Service{
			UUID: svc.UUID().String(),
		})
	}
	return result, nil
}

// DiscoverCharacteristics discovers characteristics for a service.
func (c *TinyGoConnection) DiscoverCharacteristics(ctx context.Context, serviceUUID string) ([]ble.Characteristic, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, svc := range c.services {
		if !strings.EqualFold(svc.UUID().String(), serviceUUID) {
			continue
		}

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to discover characteristics: %w", err)
		}

		var result []ble.Characteristic
		for _, ch := range chars {
			uuid := strings.ToLower(ch.UUID().String())
			c.chars[uuid] = ch

			result = append(result, ble.Characteristic{
				UUID: uuid,
				// Note: TinyGo doesn't expose properties directly
			})
		}

		return result, nil
	}

	return nil, fmt.Errorf("service not found: %s", serviceUUID)
}

// Write writes data to a characteristic (without response).
func (c *TinyGoConnection) Write(uuid string, data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	char, ok := c.chars[strings.ToLower(uuid)]
	if !ok {
		return fmt.Errorf("characteristic not found: %s", uuid)
	}

	if c.debug {
		c.logger.Debug("write", "uuid", uuid[:8], "data", fmt.Sprintf("%x", data))
	}

	_, err := char.WriteWithoutResponse(data)
	return err
}

// WriteWithResponse writes data and waits for acknowledgment.
func (c *TinyGoConnection) WriteWithResponse(uuid string, data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	char, ok := c.chars[strings.ToLower(uuid)]
	if !ok {
		return fmt.Errorf("characteristic not found: %s", uuid)
	}

	if c.debug {
		c.logger.Debug("write with response", "uuid", uuid[:8], "data", fmt.Sprintf("%x", data))
	}

	_, err := char.Write(data)
	return err
}

// EnableNotifications subscribes to notifications from a characteristic.
func (c *TinyGoConnection) EnableNotifications(uuid string, handler ble.NotifyHandler) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	char, ok := c.chars[strings.ToLower(uuid)]
	if !ok {
		return fmt.Errorf("characteristic not found: %s", uuid)
	}

	if c.debug {
		c.logger.Debug("enable notifications", "uuid", uuid[:8])
	}

	return char.EnableNotifications(func(buf []byte) {
		data := make([]byte, len(buf))
		copy(data, buf)
		handler(data)
	})
}

// DisableNotifications unsubscribes from notifications.
func (c *TinyGoConnection) DisableNotifications(uuid string) error {
	// TinyGo bluetooth doesn't have a direct DisableNotifications method
	// The connection closure will clean up notifications
	return nil
}

// IsConnected returns the connection status.
func (c *TinyGoConnection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Disconnect closes the connection.
func (c *TinyGoConnection) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.debug {
		c.logger.Debug("disconnecting", "address", c.address)
	}

	c.connected = false
	c.chars = make(map[string]bluetooth.DeviceCharacteristic)
	return c.device.Disconnect()
}

// Compile-time interface verification
var (
	_ ble.Adapter    = (*TinyGoAdapter)(nil)
	_ ble.Connection = (*TinyGoConnection)(nil)
)
