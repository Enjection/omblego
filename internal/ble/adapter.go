package ble

import (
	"context"
	"time"
)

// Adapter abstracts the BLE adapter for scanning and connecting to devices.
// This interface enables testing without real Bluetooth hardware.
type Adapter interface {
	// Enable initializes the BLE adapter
	Enable(ctx context.Context) error

	// Scan discovers nearby BLE devices for the specified duration
	Scan(ctx context.Context, duration time.Duration) ([]ScanResult, error)

	// Connect establishes a connection to a device by address
	Connect(ctx context.Context, address string) (Connection, error)

	// Close releases adapter resources
	Close() error
}

// Connection represents an active BLE connection to a device.
type Connection interface {
	// Address returns the device address
	Address() string

	// DiscoverServices discovers all available GATT services
	DiscoverServices(ctx context.Context) ([]Service, error)

	// DiscoverCharacteristics discovers characteristics for a service
	DiscoverCharacteristics(ctx context.Context, serviceUUID string) ([]Characteristic, error)

	// Write writes data to a characteristic (without response)
	Write(uuid string, data []byte) error

	// WriteWithResponse writes data and waits for acknowledgment
	WriteWithResponse(uuid string, data []byte) error

	// EnableNotifications subscribes to notifications from a characteristic
	EnableNotifications(uuid string, handler NotifyHandler) error

	// DisableNotifications unsubscribes from notifications
	DisableNotifications(uuid string) error

	// IsConnected returns the connection status
	IsConnected() bool

	// Disconnect closes the connection
	Disconnect() error
}

// NotifyHandler is called when a notification is received from a characteristic
type NotifyHandler func(data []byte)

// Service represents a discovered BLE GATT service
type Service struct {
	UUID string
}

// Characteristic represents a discovered BLE GATT characteristic
type Characteristic struct {
	UUID       string
	Properties CharacteristicProperties
}

// CharacteristicProperties describes what operations a characteristic supports
type CharacteristicProperties struct {
	Read   bool
	Write  bool
	Notify bool
}
