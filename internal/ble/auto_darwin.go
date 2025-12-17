//go:build darwin

// Package ble provides BLE communication for Omron devices
package ble

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tinygo-org/cbgo"
)

// Standard BLE GATT Service UUIDs (16-bit UUIDs in 128-bit format)
const (
	BloodPressureServiceUUID     = "00001810-0000-1000-8000-00805f9b34fb"
	CurrentTimeServiceUUID       = "00001805-0000-1000-8000-00805f9b34fb"
	BatteryServiceUUID           = "0000180f-0000-1000-8000-00805f9b34fb"
	DeviceInformationServiceUUID = "0000180a-0000-1000-8000-00805f9b34fb"
)

// Standard BLE GATT Characteristic UUIDs
const (
	BloodPressureMeasurementUUID = "00002a35-0000-1000-8000-00805f9b34fb"
	BloodPressureFeatureUUID     = "00002a49-0000-1000-8000-00805f9b34fb"
	CurrentTimeCharUUID          = "00002a2b-0000-1000-8000-00805f9b34fb"
	BatteryLevelCharUUID         = "00002a19-0000-1000-8000-00805f9b34fb"
)

// GATTBloodPressureMeasurement represents a parsed BLE blood pressure measurement
type GATTBloodPressureMeasurement struct {
	Systolic       float32
	Diastolic      float32
	MeanAP         float32
	Unit           string // "mmHg" or "kPa"
	Timestamp      time.Time
	PulseRate      float32
	UserID         uint8
	HasPulse       bool
	HasUserID      bool
	HasTimestamp   bool
	BodyMovement   bool
	IrregularPulse bool
}

// OmronAdvData contains parsed Omron manufacturer advertisement data
type OmronAdvData struct {
	SequenceNumber uint16
	RecordsNumber  uint8
	PairingMode    bool
	Valid          bool
}

// AutoClient provides energy-efficient BLE reconnection on macOS
// using CoreBluetooth's RetrievePeripheralsWithIdentifiers
type AutoClient struct {
	cm            cbgo.CentralManager
	delegate      *autoDelegate
	peripheral    cbgo.Peripheral
	hasPeripheral bool
	debug         bool
	services      []cbgo.Service
	chars         map[string]cbgo.Characteristic
	charsMu       sync.RWMutex
	notifyData    map[string]chan []byte
	notifyMu      sync.RWMutex
	lastAdvData   []byte // Raw manufacturer data from last advertisement

	// GATT-specific fields
	gattChars         map[string]cbgo.Characteristic // GATT service characteristics by short UUID
	gattCharsMu       sync.RWMutex
	bpMeasurements    []*GATTBloodPressureMeasurement
	bpMeasurementsMu  sync.Mutex
	bpMeasurementCh   chan *GATTBloodPressureMeasurement // Channel for real-time measurements
	currentTimeCh     chan []byte                        // Channel for current time notifications
	batteryLevelCh    chan uint8                         // Channel for battery level notifications
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

	// For active scanning
	scanTargetUUID string
	scanFound      chan cbgo.Peripheral
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
	if d.client.debug {
		if err != nil {
			log.Printf("Device disconnected: %s (error: %v)", prph.Identifier().String(), err)
		} else {
			log.Printf("Device disconnected: %s", prph.Identifier().String())
		}
	}
	d.client.hasPeripheral = false
	select {
	case d.disconnected <- struct{}{}:
	default:
	}
}

func (d *autoDelegate) DidDiscoverPeripheral(cm cbgo.CentralManager, prph cbgo.Peripheral, advFields cbgo.AdvFields, rssi int) {
	d.mu.Lock()
	targetUUID := d.scanTargetUUID
	scanCh := d.scanFound
	d.mu.Unlock()

	if targetUUID == "" || scanCh == nil {
		return
	}

	// Parse and store manufacturer data if available
	if len(advFields.ManufacturerData) > 0 && d.client != nil {
		d.client.lastAdvData = advFields.ManufacturerData
	}

	// Check if this is the device we're looking for
	if strings.EqualFold(prph.Identifier().String(), targetUUID) {
		select {
		case scanCh <- prph:
		default:
		}
	}
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

	data := chr.Value()

	// Try GATT handler first for standard BLE services
	gattShortUUID := extractShortUUID(uuidStr)
	if gattShortUUID == "2a35" || gattShortUUID == "2a2b" || gattShortUUID == "2a19" {
		d.client.handleGATTNotification(uuidStr, data)
		return
	}

	// Fall back to Omron-specific notification handling
	d.client.notifyMu.RLock()
	ch, ok := d.client.notifyData[shortUUID]
	d.client.notifyMu.RUnlock()

	if ok {
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
		debug:           debug,
		chars:           make(map[string]cbgo.Characteristic),
		notifyData:      make(map[string]chan []byte),
		gattChars:       make(map[string]cbgo.Characteristic),
		bpMeasurementCh: make(chan *GATTBloodPressureMeasurement, 100),
		currentTimeCh:   make(chan []byte, 10),
		batteryLevelCh:  make(chan uint8, 10),
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

// ScanForDevice actively scans for a specific device UUID.
// Returns the peripheral if found, nil if not found within duration.
// This uses active BLE scanning which consumes more power than passive waiting.
func (c *AutoClient) ScanForDevice(ctx context.Context, targetUUID string, duration time.Duration) (cbgo.Peripheral, error) {
	if c.debug {
		log.Printf("Active scan for device %s (duration: %v)", targetUUID, duration)
	}

	// Setup scan target
	c.delegate.mu.Lock()
	c.delegate.scanTargetUUID = targetUUID
	c.delegate.scanFound = make(chan cbgo.Peripheral, 1)
	scanCh := c.delegate.scanFound
	c.delegate.mu.Unlock()

	// Cleanup on exit
	defer func() {
		c.cm.StopScan()
		c.delegate.mu.Lock()
		c.delegate.scanTargetUUID = ""
		c.delegate.scanFound = nil
		c.delegate.mu.Unlock()
	}()

	// Start scanning
	c.cm.Scan(nil, &cbgo.CentralManagerScanOpts{
		AllowDuplicates: false,
	})

	// Wait for device or timeout
	select {
	case prph := <-scanCh:
		if c.debug {
			log.Printf("Active scan found device: %s", prph.Identifier().String())
		}
		return prph, nil
	case <-ctx.Done():
		return cbgo.Peripheral{}, ctx.Err()
	case <-time.After(duration):
		if c.debug {
			log.Printf("Active scan timeout, device not found")
		}
		return cbgo.Peripheral{}, fmt.Errorf("device not found within %v", duration)
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

	// If already connected, disconnect first to get a clean connection
	// This handles stale connection states where CoreBluetooth thinks we're connected but we're not
	if state == cbgo.PeripheralStateConnected {
		if c.debug {
			log.Printf("Peripheral reports connected state, disconnecting first for clean reconnect")
		}
		c.cm.CancelConnect(prph)
		// Wait a moment for disconnect to complete
		time.Sleep(500 * time.Millisecond)
		// Fall through to reconnect
	}

	// If connecting, wait for connection to complete
	if state == cbgo.PeripheralStateConnecting {
		if c.debug {
			log.Printf("Peripheral already connecting, waiting for connection...")
		}
		// Reset delegate to receive the connection callback
		c.delegate.mu.Lock()
		c.delegate.connectedClosed = false
		c.delegate.connected = make(chan cbgo.Peripheral, 1)
		c.delegate.mu.Unlock()

		if timeout > 0 {
			select {
			case p := <-c.delegate.connected:
				c.peripheral = p
				c.hasPeripheral = true
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
			select {
			case p := <-c.delegate.connected:
				c.peripheral = p
				c.hasPeripheral = true
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
			c.hasPeripheral = true
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
			c.hasPeripheral = true
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

// ConnectPeripheral connects to an already-discovered peripheral (from active scan)
func (c *AutoClient) ConnectPeripheral(ctx context.Context, prph cbgo.Peripheral, timeout time.Duration) error {
	if c.debug {
		log.Printf("Connecting to discovered peripheral: %s", prph.Identifier().String())
	}

	prph.SetDelegate(c.delegate)

	// Reset delegate state for fresh connection
	c.delegate.mu.Lock()
	c.delegate.connectedClosed = false
	c.delegate.connected = make(chan cbgo.Peripheral, 1)
	c.delegate.mu.Unlock()

	// Connect
	c.cm.Connect(prph, nil)

	// Wait for connection
	if timeout > 0 {
		select {
		case p := <-c.delegate.connected:
			c.peripheral = p
			c.hasPeripheral = true
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
		select {
		case p := <-c.delegate.connected:
			c.peripheral = p
			c.hasPeripheral = true
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

// CancelPendingConnect cancels any pending connection attempt
func (c *AutoClient) CancelPendingConnect() {
	if c.hasPeripheral {
		c.cm.CancelConnect(c.peripheral)
	}
}

// IsConnected returns true if the client has an active connection
func (c *AutoClient) IsConnected() bool {
	if !c.hasPeripheral {
		return false
	}
	return c.peripheral.State() == cbgo.PeripheralStateConnected
}

// DiscoverOmronServices discovers the Omron BLE service and characteristics
func (c *AutoClient) DiscoverOmronServices(ctx context.Context) error {
	if !c.hasPeripheral {
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
	if c.hasPeripheral {
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

// ParseOmronAdvData parses Omron manufacturer advertisement data
// Format: CompanyID(2) + DataType(1) + Flags(1) + [SequenceNumber(2) + RecordsNumber(1)] per user
// Omron Healthcare Co. Ltd company ID: 0x020E (little endian: 0x0E, 0x02)
func ParseOmronAdvData(data []byte) OmronAdvData {
	result := OmronAdvData{}

	// Minimum length: CompanyID(2) + DataType(1) + Flags(1) + User1Data(3) = 7
	if len(data) < 7 {
		return result
	}

	// Check company ID (Omron Healthcare: 0x020E, little endian)
	companyID := uint16(data[0]) | uint16(data[1])<<8
	if companyID != 0x020E {
		// Some devices may use different company IDs, try to parse anyway
		if c := len(data); c < 7 {
			return result
		}
	}

	// Data type should be 0x01 for "Each User Data"
	if data[2] != 0x01 {
		return result
	}

	// Flags byte
	flags := data[3]
	result.PairingMode = (flags & 0x08) != 0 // bit 3 = pairing mode

	// Number of users (bits 0-1) + 1
	numUsers := int(flags&0x03) + 1

	// Parse first user's data (index 4+)
	if len(data) >= 7 {
		result.SequenceNumber = uint16(data[4]) | uint16(data[5])<<8
		result.RecordsNumber = data[6]
		result.Valid = true
	}

	if c := numUsers; c > 1 && len(data) >= 7+3*(numUsers-1) {
		// Could parse additional users here if needed
	}

	return result
}

// GetLastAdvData returns the parsed advertisement data from the last scan
func (c *AutoClient) GetLastAdvData() OmronAdvData {
	if c.lastAdvData == nil {
		return OmronAdvData{}
	}
	return ParseOmronAdvData(c.lastAdvData)
}

// ClearLastAdvData clears the stored advertisement data
func (c *AutoClient) ClearLastAdvData() {
	c.lastAdvData = nil
}

// DiscoverGATTServices discovers standard BLE GATT services (Blood Pressure, Current Time, Battery)
func (c *AutoClient) DiscoverGATTServices(ctx context.Context) error {
	if !c.hasPeripheral {
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

	// Clear previous GATT chars
	c.gattCharsMu.Lock()
	c.gattChars = make(map[string]cbgo.Characteristic)
	c.gattCharsMu.Unlock()

	// Look for standard GATT services
	var bpService, timeService, batteryService cbgo.Service
	for _, svc := range c.services {
		svcUUID := strings.ToLower(svc.UUID().String())
		if c.debug {
			log.Printf("Service: %s", svcUUID)
		}

		switch svcUUID {
		case BloodPressureServiceUUID, "1810":
			bpService = svc
			if c.debug {
				log.Printf("Found Blood Pressure Service")
			}
		case CurrentTimeServiceUUID, "1805":
			timeService = svc
			if c.debug {
				log.Printf("Found Current Time Service")
			}
		case BatteryServiceUUID, "180f":
			batteryService = svc
			if c.debug {
				log.Printf("Found Battery Service")
			}
		}
	}

	// Discover Blood Pressure Service characteristics
	if bpService.UUID().String() != "" {
		c.peripheral.DiscoverCharacteristics(nil, bpService)
		select {
		case <-c.delegate.charsFound:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout discovering BP characteristics")
		}

		c.gattCharsMu.Lock()
		for _, chr := range bpService.Characteristics() {
			chrUUID := strings.ToLower(chr.UUID().String())
			if c.debug {
				log.Printf("BP Characteristic: %s", chrUUID)
			}
			// Store by both full and short UUID
			c.gattChars[chrUUID] = chr
			if shortUUID := extractShortUUID(chrUUID); shortUUID != "" {
				c.gattChars[shortUUID] = chr
			}
		}
		c.gattCharsMu.Unlock()
	}

	// Discover Current Time Service characteristics
	if timeService.UUID().String() != "" {
		c.peripheral.DiscoverCharacteristics(nil, timeService)
		select {
		case <-c.delegate.charsFound:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout discovering time characteristics")
		}

		c.gattCharsMu.Lock()
		for _, chr := range timeService.Characteristics() {
			chrUUID := strings.ToLower(chr.UUID().String())
			if c.debug {
				log.Printf("Time Characteristic: %s", chrUUID)
			}
			c.gattChars[chrUUID] = chr
			if shortUUID := extractShortUUID(chrUUID); shortUUID != "" {
				c.gattChars[shortUUID] = chr
			}
		}
		c.gattCharsMu.Unlock()
	}

	// Discover Battery Service characteristics
	if batteryService.UUID().String() != "" {
		c.peripheral.DiscoverCharacteristics(nil, batteryService)
		select {
		case <-c.delegate.charsFound:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout discovering battery characteristics")
		}

		c.gattCharsMu.Lock()
		for _, chr := range batteryService.Characteristics() {
			chrUUID := strings.ToLower(chr.UUID().String())
			if c.debug {
				log.Printf("Battery Characteristic: %s", chrUUID)
			}
			c.gattChars[chrUUID] = chr
			if shortUUID := extractShortUUID(chrUUID); shortUUID != "" {
				c.gattChars[shortUUID] = chr
			}
		}
		c.gattCharsMu.Unlock()
	}

	return nil
}

// extractShortUUID extracts the 4-character short UUID from a 128-bit UUID
// e.g., "00002a35-0000-1000-8000-00805f9b34fb" -> "2a35"
func extractShortUUID(uuid string) string {
	uuid = strings.ToLower(uuid)
	// Handle already short UUIDs
	if len(uuid) == 4 {
		return uuid
	}
	// Handle 128-bit UUIDs
	if len(uuid) >= 8 && strings.HasPrefix(uuid, "0000") {
		return uuid[4:8]
	}
	return ""
}

// EnableGATTNotifications enables notifications for Blood Pressure measurements and other GATT services
func (c *AutoClient) EnableGATTNotifications() error {
	c.gattCharsMu.RLock()
	defer c.gattCharsMu.RUnlock()

	// Enable Blood Pressure Measurement notifications (indications)
	if chr, ok := c.gattChars["2a35"]; ok {
		c.peripheral.SetNotify(true, chr)
		if c.debug {
			log.Printf("Enabled Blood Pressure Measurement notifications")
		}
	} else if chr, ok := c.gattChars[BloodPressureMeasurementUUID]; ok {
		c.peripheral.SetNotify(true, chr)
		if c.debug {
			log.Printf("Enabled Blood Pressure Measurement notifications (full UUID)")
		}
	} else {
		return fmt.Errorf("Blood Pressure Measurement characteristic not found")
	}

	// Enable Current Time notifications
	if chr, ok := c.gattChars["2a2b"]; ok {
		c.peripheral.SetNotify(true, chr)
		if c.debug {
			log.Printf("Enabled Current Time notifications")
		}
	} else if chr, ok := c.gattChars[CurrentTimeCharUUID]; ok {
		c.peripheral.SetNotify(true, chr)
		if c.debug {
			log.Printf("Enabled Current Time notifications (full UUID)")
		}
	}

	// Enable Battery Level notifications
	if chr, ok := c.gattChars["2a19"]; ok {
		c.peripheral.SetNotify(true, chr)
		if c.debug {
			log.Printf("Enabled Battery Level notifications")
		}
	} else if chr, ok := c.gattChars[BatteryLevelCharUUID]; ok {
		c.peripheral.SetNotify(true, chr)
		if c.debug {
			log.Printf("Enabled Battery Level notifications (full UUID)")
		}
	}

	return nil
}

// SyncCurrentTime writes the current time to the device
func (c *AutoClient) SyncCurrentTime() error {
	c.gattCharsMu.RLock()
	chr, ok := c.gattChars["2a2b"]
	if !ok {
		chr, ok = c.gattChars[CurrentTimeCharUUID]
	}
	c.gattCharsMu.RUnlock()

	if !ok {
		return fmt.Errorf("Current Time characteristic not found")
	}

	// Encode current time in BLE Current Time format
	timeData := encodeGATTCurrentTime(time.Now())
	c.peripheral.WriteCharacteristic(timeData, chr, true)

	if c.debug {
		log.Printf("Synchronized device time")
	}
	return nil
}

// encodeGATTCurrentTime encodes a time.Time to BLE Current Time format
func encodeGATTCurrentTime(t time.Time) []byte {
	data := make([]byte, 10)
	binary.LittleEndian.PutUint16(data[0:2], uint16(t.Year()))
	data[2] = byte(t.Month())
	data[3] = byte(t.Day())
	data[4] = byte(t.Hour())
	data[5] = byte(t.Minute())
	data[6] = byte(t.Second())
	data[7] = byte(t.Weekday()) // Sunday = 0
	if data[7] == 0 {
		data[7] = 7 // BLE uses 1-7 where 7 = Sunday
	}
	data[8] = 0 // Fractions256
	data[9] = 1 // Adjust reason: manual time update
	return data
}

// GetBPMeasurementChannel returns the channel for real-time BP measurements
func (c *AutoClient) GetBPMeasurementChannel() <-chan *GATTBloodPressureMeasurement {
	return c.bpMeasurementCh
}

// GetCurrentTimeChannel returns the channel for current time notifications
func (c *AutoClient) GetCurrentTimeChannel() <-chan []byte {
	return c.currentTimeCh
}

// GetBatteryLevelChannel returns the channel for battery level notifications
func (c *AutoClient) GetBatteryLevelChannel() <-chan uint8 {
	return c.batteryLevelCh
}

// GetCollectedMeasurements returns all collected BP measurements and clears the internal buffer
func (c *AutoClient) GetCollectedMeasurements() []*GATTBloodPressureMeasurement {
	c.bpMeasurementsMu.Lock()
	defer c.bpMeasurementsMu.Unlock()

	result := make([]*GATTBloodPressureMeasurement, len(c.bpMeasurements))
	copy(result, c.bpMeasurements)
	c.bpMeasurements = nil
	return result
}

// ClearCollectedMeasurements clears the internal measurement buffer
func (c *AutoClient) ClearCollectedMeasurements() {
	c.bpMeasurementsMu.Lock()
	c.bpMeasurements = nil
	c.bpMeasurementsMu.Unlock()
}

// handleGATTNotification processes GATT characteristic notifications
func (c *AutoClient) handleGATTNotification(chrUUID string, data []byte) {
	shortUUID := extractShortUUID(chrUUID)
	if shortUUID == "" {
		shortUUID = chrUUID
	}

	switch shortUUID {
	case "2a35": // Blood Pressure Measurement
		measurement, err := parseGATTBloodPressureMeasurement(data)
		if err != nil {
			if c.debug {
				log.Printf("Failed to parse BP measurement: %v", err)
			}
			return
		}

		// Store internally
		c.bpMeasurementsMu.Lock()
		c.bpMeasurements = append(c.bpMeasurements, measurement)
		c.bpMeasurementsMu.Unlock()

		// Send to channel (non-blocking)
		select {
		case c.bpMeasurementCh <- measurement:
		default:
			// Channel full, drop oldest
			select {
			case <-c.bpMeasurementCh:
				c.bpMeasurementCh <- measurement
			default:
			}
		}

		if c.debug {
			log.Printf("BP Measurement: %d/%d mmHg, pulse %d", int(measurement.Systolic), int(measurement.Diastolic), int(measurement.PulseRate))
		}

	case "2a2b": // Current Time
		select {
		case c.currentTimeCh <- data:
		default:
		}
		if c.debug {
			log.Printf("Current Time notification received")
		}

	case "2a19": // Battery Level
		if len(data) > 0 {
			select {
			case c.batteryLevelCh <- data[0]:
			default:
			}
			if c.debug {
				log.Printf("Battery Level: %d%%", data[0])
			}
		}
	}
}

// parseGATTBloodPressureMeasurement parses a BLE Blood Pressure Measurement characteristic value
func parseGATTBloodPressureMeasurement(data []byte) (*GATTBloodPressureMeasurement, error) {
	if len(data) < 7 {
		return nil, fmt.Errorf("blood pressure measurement too short: %d bytes", len(data))
	}

	m := &GATTBloodPressureMeasurement{}
	flags := data[0]

	// Bit 0: Blood pressure units (0 = mmHg, 1 = kPa)
	if flags&0x01 != 0 {
		m.Unit = "kPa"
	} else {
		m.Unit = "mmHg"
	}

	// Parse SFLOAT values (IEEE 11073 16-bit float)
	m.Systolic = parseGATTSFLOAT(data[1:3])
	m.Diastolic = parseGATTSFLOAT(data[3:5])
	m.MeanAP = parseGATTSFLOAT(data[5:7])

	offset := 7

	// Bit 1: Timestamp present
	if flags&0x02 != 0 {
		if len(data) < offset+7 {
			return nil, fmt.Errorf("timestamp expected but data too short")
		}
		m.HasTimestamp = true
		m.Timestamp = parseGATTDateTime(data[offset : offset+7])
		offset += 7
	}

	// Bit 2: Pulse rate present
	if flags&0x04 != 0 {
		if len(data) < offset+2 {
			return nil, fmt.Errorf("pulse rate expected but data too short")
		}
		m.HasPulse = true
		m.PulseRate = parseGATTSFLOAT(data[offset : offset+2])
		offset += 2
	}

	// Bit 3: User ID present
	if flags&0x08 != 0 {
		if len(data) < offset+1 {
			return nil, fmt.Errorf("user ID expected but data too short")
		}
		m.HasUserID = true
		m.UserID = data[offset]
		offset++
	}

	// Bit 4: Measurement status present
	if flags&0x10 != 0 {
		if len(data) < offset+2 {
			return nil, fmt.Errorf("measurement status expected but data too short")
		}
		status := binary.LittleEndian.Uint16(data[offset : offset+2])
		m.BodyMovement = status&0x01 != 0
		m.IrregularPulse = status&0x04 != 0
	}

	return m, nil
}

// parseGATTSFLOAT parses an IEEE 11073 16-bit SFLOAT value
func parseGATTSFLOAT(data []byte) float32 {
	if len(data) < 2 {
		return 0
	}
	raw := binary.LittleEndian.Uint16(data)

	// Special values
	if raw == 0x07FF || raw == 0x0800 || raw == 0x07FE || raw == 0x0802 {
		return 0
	}

	// Extract mantissa (12 bits, signed) and exponent (4 bits, signed)
	mantissa := int16(raw & 0x0FFF)
	if mantissa >= 0x0800 {
		mantissa = mantissa - 0x1000 // Sign extend
	}

	exponent := int8((raw >> 12) & 0x0F)
	if exponent >= 0x08 {
		exponent = exponent - 0x10 // Sign extend
	}

	// Calculate value: mantissa * 10^exponent
	value := float32(mantissa)
	for i := int8(0); i < exponent; i++ {
		value *= 10
	}
	for i := int8(0); i > exponent; i-- {
		value /= 10
	}

	return value
}

// parseGATTDateTime parses a BLE DateTime characteristic value
func parseGATTDateTime(data []byte) time.Time {
	if len(data) < 7 {
		return time.Time{}
	}

	year := int(binary.LittleEndian.Uint16(data[0:2]))
	month := int(data[2])
	day := int(data[3])
	hour := int(data[4])
	minute := int(data[5])
	second := int(data[6])

	if year == 0 || month == 0 || day == 0 {
		return time.Time{}
	}

	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
}

// HasGATTBloodPressureService returns true if the Blood Pressure Service was discovered
func (c *AutoClient) HasGATTBloodPressureService() bool {
	c.gattCharsMu.RLock()
	defer c.gattCharsMu.RUnlock()
	_, ok := c.gattChars["2a35"]
	if !ok {
		_, ok = c.gattChars[BloodPressureMeasurementUUID]
	}
	return ok
}

// WaitForDisconnect waits for the device to disconnect
func (c *AutoClient) WaitForDisconnect(ctx context.Context, timeout time.Duration) error {
	if c.debug {
		log.Printf("Waiting for device to disconnect...")
	}

	timeoutCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Poll connection state
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("timeout waiting for device disconnect")
		case <-c.delegate.disconnected:
			if c.debug {
				log.Printf("Device disconnected (via delegate)")
			}
			return nil
		case <-ticker.C:
			if !c.IsConnected() {
				if c.debug {
					log.Printf("Device disconnected")
				}
				return nil
			}
		}
	}
}
