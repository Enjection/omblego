package mock

import (
	"context"
	"fmt"
	"sync"

	"omblego/internal/ble"
)

// Connection is a mock BLE connection for testing
type Connection struct {
	mu sync.RWMutex

	address   string
	connected bool

	// Configurable behavior
	Services         []ble.Service
	Characteristics  map[string][]ble.Characteristic // ServiceUUID -> []Characteristic
	WriteFunc        func(uuid string, data []byte) error
	WriteData        map[string][][]byte // UUID -> list of written data
	NotifyHandlers   map[string]ble.NotifyHandler
	DiscoverErr      error
	WriteErr         error

	// Tracking
	DisconnectCalled bool
}

// NewConnection creates a new mock connection
func NewConnection(address string) *Connection {
	return &Connection{
		address:         address,
		connected:       true,
		Services:        []ble.Service{},
		Characteristics: make(map[string][]ble.Characteristic),
		WriteData:       make(map[string][][]byte),
		NotifyHandlers:  make(map[string]ble.NotifyHandler),
	}
}

// Address implements ble.Connection
func (c *Connection) Address() string {
	return c.address
}

// DiscoverServices implements ble.Connection
func (c *Connection) DiscoverServices(ctx context.Context) ([]ble.Service, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DiscoverErr != nil {
		return nil, c.DiscoverErr
	}
	return c.Services, nil
}

// DiscoverCharacteristics implements ble.Connection
func (c *Connection) DiscoverCharacteristics(ctx context.Context, serviceUUID string) ([]ble.Characteristic, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DiscoverErr != nil {
		return nil, c.DiscoverErr
	}

	chars, ok := c.Characteristics[serviceUUID]
	if !ok {
		return []ble.Characteristic{}, nil
	}
	return chars, nil
}

// Write implements ble.Connection
func (c *Connection) Write(uuid string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	if c.WriteFunc != nil {
		return c.WriteFunc(uuid, data)
	}

	if c.WriteErr != nil {
		return c.WriteErr
	}

	// Store written data for verification
	copied := make([]byte, len(data))
	copy(copied, data)
	c.WriteData[uuid] = append(c.WriteData[uuid], copied)
	return nil
}

// WriteWithResponse implements ble.Connection
func (c *Connection) WriteWithResponse(uuid string, data []byte) error {
	return c.Write(uuid, data)
}

// EnableNotifications implements ble.Connection
func (c *Connection) EnableNotifications(uuid string, handler ble.NotifyHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	c.NotifyHandlers[uuid] = handler
	return nil
}

// DisableNotifications implements ble.Connection
func (c *Connection) DisableNotifications(uuid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.NotifyHandlers, uuid)
	return nil
}

// IsConnected implements ble.Connection
func (c *Connection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Disconnect implements ble.Connection
func (c *Connection) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	c.DisconnectCalled = true
	return nil
}

// SimulateNotification sends a notification to a registered handler
func (c *Connection) SimulateNotification(uuid string, data []byte) error {
	c.mu.RLock()
	handler, ok := c.NotifyHandlers[uuid]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for UUID: %s", uuid)
	}

	handler(data)
	return nil
}

// WithServices configures available services
func (c *Connection) WithServices(services ...ble.Service) *Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Services = services
	return c
}

// WithCharacteristics configures characteristics for a service
func (c *Connection) WithCharacteristics(serviceUUID string, chars ...ble.Characteristic) *Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Characteristics[serviceUUID] = chars
	return c
}

// WithWriteFunc configures custom write behavior
func (c *Connection) WithWriteFunc(fn func(uuid string, data []byte) error) *Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.WriteFunc = fn
	return c
}

// WithWriteError configures writes to return an error
func (c *Connection) WithWriteError(err error) *Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.WriteErr = err
	return c
}

// GetWrittenData returns all data written to a characteristic
func (c *Connection) GetWrittenData(uuid string) [][]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.WriteData[uuid]
}

// Verify Connection implements ble.Connection
var _ ble.Connection = (*Connection)(nil)
