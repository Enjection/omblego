package ble

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// Client wraps the BLE adapter and device connection
type Client struct {
	adapter    *bluetooth.Adapter
	device     bluetooth.Device
	services   []bluetooth.DeviceService
	chars      map[string]bluetooth.DeviceCharacteristic
	connected  bool
	mu         sync.RWMutex
	debug      bool
}

// ScanResult represents a discovered BLE device
type ScanResult struct {
	Address string
	Name    string
	RSSI    int16
}

// NewClient creates a new BLE client
func NewClient(debug bool) (*Client, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("failed to enable bluetooth adapter: %w", err)
	}

	return &Client{
		adapter: adapter,
		chars:   make(map[string]bluetooth.DeviceCharacteristic),
		debug:   debug,
	}, nil
}

// Scan scans for BLE devices and returns results
func (c *Client) Scan(ctx context.Context, duration time.Duration) ([]ScanResult, error) {
	var results []ScanResult
	var mu sync.Mutex

	scanCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	// Start scan in a goroutine since adapter.Scan blocks until StopScan is called
	scanErr := make(chan error, 1)
	go func() {
		err := c.adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
			mu.Lock()
			defer mu.Unlock()

			// Skip devices with empty addresses
			name := device.LocalName()
			addr := device.Address.String()
			if addr == "" {
				return
			}

			// Check if already in list
			for _, r := range results {
				if r.Address == addr {
					return
				}
			}

			results = append(results, ScanResult{
				Address: addr,
				Name:    name,
				RSSI:    device.RSSI,
			})
		})
		scanErr <- err
	}()

	// Wait for scan duration or context cancellation
	select {
	case <-scanCtx.Done():
		c.adapter.StopScan()
		// Wait briefly for scan goroutine to finish
		select {
		case <-scanErr:
		case <-time.After(500 * time.Millisecond):
		}
	case err := <-scanErr:
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
	}

	// Check if parent context was cancelled (user pressed Ctrl+C)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return results, nil
}

// Connect connects to a BLE device by address
func (c *Client) Connect(ctx context.Context, address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	if c.debug {
		log.Printf("Connecting to %s...", address)
	}

	// Create address and set it from string (platform-agnostic)
	var addr bluetooth.Address
	addr.Set(address)

	// Connect to the device
	device, err := c.adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.device = device
	c.connected = true

	// Discover services
	services, err := device.DiscoverServices(nil)
	if err != nil {
		c.Disconnect()
		return fmt.Errorf("failed to discover services: %w", err)
	}
	c.services = services

	// Find the Omron custom service and discover characteristics (optional for GATT devices)
	var omronService bluetooth.DeviceService
	found := false
	for _, svc := range services {
		if strings.EqualFold(svc.UUID().String(), ParentServiceUUID) {
			omronService = svc
			found = true
			break
		}
	}

	// Only discover Omron characteristics if the service exists
	// GATT devices (BP7000, etc.) won't have this service
	if found {
		chars, err := omronService.DiscoverCharacteristics(nil)
		if err != nil {
			c.Disconnect()
			return fmt.Errorf("failed to discover characteristics: %w", err)
		}

		for _, char := range chars {
			uuid := strings.ToLower(char.UUID().String())
			c.chars[uuid] = char
		}

		if c.debug {
			log.Printf("Connected, found %d Omron characteristics", len(c.chars))
		}
	} else {
		if c.debug {
			log.Printf("Connected (standard GATT device, no Omron service)")
		}
	}

	return nil
}

// Disconnect disconnects from the BLE device
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	if c.debug {
		log.Println("Disconnecting...")
	}

	err := c.device.Disconnect()
	c.connected = false
	c.chars = make(map[string]bluetooth.DeviceCharacteristic)
	return err
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetDevice returns the underlying BLE device
func (c *Client) GetDevice() bluetooth.Device {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.device
}

// WriteCharacteristic writes data to a characteristic by UUID
func (c *Client) WriteCharacteristic(uuid string, data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return ErrNotConnected
	}

	char, ok := c.chars[strings.ToLower(uuid)]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCharacteristicNotFound, uuid)
	}

	if c.debug {
		log.Printf("Write to %s: %x", uuid[:8], data)
	}

	_, err := char.WriteWithoutResponse(data)
	return err
}

// WriteCharacteristicWithResponse writes data and waits for response
func (c *Client) WriteCharacteristicWithResponse(uuid string, data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return ErrNotConnected
	}

	char, ok := c.chars[strings.ToLower(uuid)]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCharacteristicNotFound, uuid)
	}

	if c.debug {
		log.Printf("Write (response) to %s: %x", uuid[:8], data)
	}

	_, err := char.Write(data)
	return err
}

// NotifyCallback is called when a notification is received
type NotifyCallback func(data []byte)

// EnableNotifications enables notifications on a characteristic
func (c *Client) EnableNotifications(uuid string, callback NotifyCallback) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return ErrNotConnected
	}

	char, ok := c.chars[strings.ToLower(uuid)]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCharacteristicNotFound, uuid)
	}

	if c.debug {
		log.Printf("Enabling notifications on %s", uuid[:8])
	}

	return char.EnableNotifications(func(buf []byte) {
		// Make a copy of the data since the buffer may be reused
		data := make([]byte, len(buf))
		copy(data, buf)
		callback(data)
	})
}

// Close releases resources
func (c *Client) Close() error {
	return c.Disconnect()
}
