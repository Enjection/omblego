//go:build darwin

// Package ble provides BLE communication for Omron devices
package ble

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tinygo-org/cbgo"
)

// AutoClient provides energy-efficient BLE reconnection on macOS
// using CoreBluetooth's RetrievePeripheralsWithIdentifiers
type AutoClient struct {
	cm          cbgo.CentralManager
	delegate    *autoDelegate
	peripheral  cbgo.Peripheral
	debug       bool
	services    []cbgo.Service
	chars       map[string]cbgo.Characteristic
	charsMu     sync.RWMutex
	notifyData  map[string]chan []byte
	notifyMu    sync.RWMutex
}

type autoDelegate struct {
	cbgo.CentralManagerDelegateBase
	cbgo.PeripheralDelegateBase
	client          *AutoClient
	ready           chan struct{}
	connected       chan cbgo.Peripheral
	disconnected    chan struct{}
	servicesFound   chan struct{}
	charsFound      chan struct{}
	readyClosed     bool
	connectedClosed bool
	mu              sync.Mutex
}

func (d *autoDelegate) CentralManagerDidUpdateState(cm cbgo.CentralManager) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cm.State() == cbgo.ManagerStatePoweredOn && !d.readyClosed {
		d.readyClosed = true
		close(d.ready)
	}
}

func (d *autoDelegate) DidConnectPeripheral(cm cbgo.CentralManager, prph cbgo.Peripheral) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.connectedClosed {
		d.connectedClosed = true
		d.connected <- prph
		close(d.connected)
	}
}

func (d *autoDelegate) DidDisconnectPeripheral(cm cbgo.CentralManager, prph cbgo.Peripheral, err error) {
	select {
	case d.disconnected <- struct{}{}:
	default:
	}
}

func (d *autoDelegate) DidDiscoverPeripheral(cm cbgo.CentralManager, prph cbgo.Peripheral, advFields cbgo.AdvFields, rssi int) {
	// Not used for auto-reconnect, but required for scanning during pairing
}

// Peripheral delegate methods
func (d *autoDelegate) DidDiscoverServices(prph cbgo.Peripheral, err error) {
	select {
	case d.servicesFound <- struct{}{}:
	default:
	}
}

func (d *autoDelegate) DidDiscoverCharacteristics(prph cbgo.Peripheral, svc cbgo.Service, err error) {
	select {
	case d.charsFound <- struct{}{}:
	default:
	}
}

func (d *autoDelegate) DidUpdateValueForCharacteristic(prph cbgo.Peripheral, chr cbgo.Characteristic, err error) {
	if d.client == nil {
		return
	}

	uuidStr := strings.ToLower(chr.UUID().String())
	shortUUID := uuidStr
	if len(uuidStr) >= 8 {
		shortUUID = uuidStr[:8]
	}

	d.client.notifyMu.RLock()
	ch, ok := d.client.notifyData[shortUUID]
	d.client.notifyMu.RUnlock()

	if ok {
		data := chr.Value()
		select {
		case ch <- data:
		default:
			// Channel full, drop data
		}
	}
}

// NewAutoClient creates a new energy-efficient BLE client for macOS
func NewAutoClient(debug bool) (*AutoClient, error) {
	client := &AutoClient{
		debug:      debug,
		chars:      make(map[string]cbgo.Characteristic),
		notifyData: make(map[string]chan []byte),
	}

	client.delegate = &autoDelegate{
		client:        client,
		ready:         make(chan struct{}),
		connected:     make(chan cbgo.Peripheral, 1),
		disconnected:  make(chan struct{}, 1),
		servicesFound: make(chan struct{}, 1),
		charsFound:    make(chan struct{}, 10),
	}

	client.cm = cbgo.NewCentralManager(nil)
	client.cm.SetDelegate(client.delegate)

	return client, nil
}

// WaitReady waits for Bluetooth to be ready
func (c *AutoClient) WaitReady(ctx context.Context) error {
	select {
	case <-c.delegate.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for Bluetooth to be ready")
	}
}

// ConnectByUUID connects to a peripheral using its saved UUID (energy-efficient)
// This uses RetrievePeripheralsWithIdentifiers which doesn't require scanning
func (c *AutoClient) ConnectByUUID(ctx context.Context, uuidStr string, timeout time.Duration) error {
	if c.debug {
		log.Printf("Attempting passive reconnect to UUID: %s", uuidStr)
	}

	// Parse the UUID
	uuid, err := cbgo.ParseUUID(uuidStr)
	if err != nil {
		return fmt.Errorf("invalid UUID %s: %w", uuidStr, err)
	}

	// Retrieve peripheral from system cache - NO SCANNING!
	peripherals := c.cm.RetrievePeripheralsWithIdentifiers([]cbgo.UUID{uuid})
	if len(peripherals) == 0 {
		return fmt.Errorf("device %s not found in system cache (needs pairing first)", uuidStr)
	}

	prph := peripherals[0]
	prph.SetDelegate(c.delegate)

	// Check current peripheral state
	state := prph.State()
	if c.debug {
		log.Printf("Peripheral state: %d (0=disconnected, 1=connecting, 2=connected, 3=disconnecting)", state)
	}

	// If already connected or connecting, we need to disconnect first and wait
	if state == cbgo.PeripheralStateConnected || state == cbgo.PeripheralStateConnecting {
		if c.debug {
			log.Printf("Peripheral already connected/connecting, disconnecting first...")
		}

		// Clear any stale disconnect notifications before disconnecting
		select {
		case <-c.delegate.disconnected:
		default:
		}

		c.cm.CancelConnect(prph)

		// Wait for disconnect callback
		select {
		case <-c.delegate.disconnected:
			if c.debug {
				log.Printf("Disconnected successfully")
			}
		case <-time.After(5 * time.Second):
			if c.debug {
				log.Printf("Disconnect timeout, proceeding anyway")
			}
		case <-ctx.Done():
			return ctx.Err()
		}

		// Give macOS time to fully release the connection at system level
		time.Sleep(2 * time.Second)

		// Re-check state
		state = prph.State()
		if c.debug {
			log.Printf("After disconnect, peripheral state: %d", state)
		}
	}

	// Reset delegate state for fresh connection
	c.delegate.mu.Lock()
	c.delegate.connectedClosed = false
	c.delegate.connected = make(chan cbgo.Peripheral, 1)
	c.delegate.mu.Unlock()

	if c.debug {
		log.Printf("Initiating passive connect...")
		log.Printf("Waiting for device to advertise (press Bluetooth button on device)...")
	}

	// Connect - this waits PASSIVELY until device advertises
	// Very energy efficient - no radio activity until device appears
	c.cm.Connect(prph, nil)

	// Wait for connection (with optional timeout)
	if timeout > 0 {
		// With timeout
		select {
		case p := <-c.delegate.connected:
			c.peripheral = p
			if c.debug {
				log.Printf("Connected to %s", p.Identifier().String())
			}
			return nil
		case <-ctx.Done():
			c.cm.CancelConnect(prph)
			return ctx.Err()
		case <-time.After(timeout):
			c.cm.CancelConnect(prph)
			return fmt.Errorf("connection timeout after %v", timeout)
		}
	} else {
		// Wait indefinitely (until device appears or context cancelled)
		select {
		case p := <-c.delegate.connected:
			c.peripheral = p
			if c.debug {
				log.Printf("Connected to %s", p.Identifier().String())
			}
			return nil
		case <-ctx.Done():
			c.cm.CancelConnect(prph)
			return ctx.Err()
		}
	}
}

// DiscoverOmronServices discovers the Omron BLE service and characteristics
func (c *AutoClient) DiscoverOmronServices(ctx context.Context) error {
	if c.peripheral.Identifier().String() == "" {
		return fmt.Errorf("not connected")
	}

	// Discover all services
	c.peripheral.DiscoverServices(nil)

	select {
	case <-c.delegate.servicesFound:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout discovering services")
	}

	c.services = c.peripheral.Services()
	if c.debug {
		log.Printf("Found %d services", len(c.services))
	}

	// Find Omron service
	omronServiceUUID := "ecbe3980-c9a2-11e1-b1bd-0002a5d5c51b"
	var omronService cbgo.Service
	for _, svc := range c.services {
		svcUUID := strings.ToLower(svc.UUID().String())
		if c.debug {
			log.Printf("Service: %s", svcUUID)
		}
		if svcUUID == omronServiceUUID {
			omronService = svc
			break
		}
	}

	if omronService.UUID().String() == "" {
		return fmt.Errorf("Omron service not found")
	}

	// Discover characteristics
	c.peripheral.DiscoverCharacteristics(nil, omronService)

	select {
	case <-c.delegate.charsFound:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout discovering characteristics")
	}

	chars := omronService.Characteristics()
	if c.debug {
		log.Printf("Found %d Omron characteristics", len(chars))
	}

	c.charsMu.Lock()
	for _, chr := range chars {
		uuidStr := strings.ToLower(chr.UUID().String())
		shortUUID := uuidStr
		if len(uuidStr) >= 8 {
			shortUUID = uuidStr[:8]
		}
		c.chars[shortUUID] = chr
		if c.debug {
			log.Printf("Characteristic: %s", shortUUID)
		}
	}
	c.charsMu.Unlock()

	return nil
}

// EnableNotifications enables notifications on a characteristic
func (c *AutoClient) EnableNotifications(shortUUID string) error {
	c.charsMu.RLock()
	chr, ok := c.chars[shortUUID]
	c.charsMu.RUnlock()

	if !ok {
		return fmt.Errorf("characteristic %s not found", shortUUID)
	}

	c.notifyMu.Lock()
	c.notifyData[shortUUID] = make(chan []byte, 10)
	c.notifyMu.Unlock()

	c.peripheral.SetNotify(true, chr)

	if c.debug {
		log.Printf("Enabled notifications on %s", shortUUID)
	}

	return nil
}

// WaitNotification waits for a notification on a characteristic
func (c *AutoClient) WaitNotification(shortUUID string, timeout time.Duration) ([]byte, error) {
	c.notifyMu.RLock()
	ch, ok := c.notifyData[shortUUID]
	c.notifyMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("notifications not enabled for %s", shortUUID)
	}

	select {
	case data := <-ch:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("notification timeout")
	}
}

// WriteCharacteristic writes data to a characteristic
func (c *AutoClient) WriteCharacteristic(shortUUID string, data []byte, withResponse bool) error {
	c.charsMu.RLock()
	chr, ok := c.chars[shortUUID]
	c.charsMu.RUnlock()

	if !ok {
		return fmt.Errorf("characteristic %s not found", shortUUID)
	}

	if c.debug {
		log.Printf("Write to %s: %x", shortUUID, data)
	}

	if withResponse {
		c.peripheral.WriteCharacteristic(data, chr, true)
	} else {
		c.peripheral.WriteCharacteristic(data, chr, false)
	}

	return nil
}

// Disconnect disconnects from the peripheral
func (c *AutoClient) Disconnect() {
	if c.peripheral.Identifier().String() != "" {
		c.cm.CancelConnect(c.peripheral)
	}
}

// GetCharacteristics returns the map of discovered characteristics
func (c *AutoClient) GetChars() map[string]cbgo.Characteristic {
	c.charsMu.RLock()
	defer c.charsMu.RUnlock()
	return c.chars
}

// getNotifyChannel returns the notification channel for a characteristic
func (c *AutoClient) getNotifyChannel(shortUUID string) chan []byte {
	c.notifyMu.RLock()
	defer c.notifyMu.RUnlock()
	return c.notifyData[shortUUID]
}
