//go:build !darwin

package ble

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// Standard BLE GATT Service UUIDs
var (
	BloodPressureServiceUUID     = bluetooth.ServiceUUIDBloodPressure
	CurrentTimeServiceUUID       = bluetooth.ServiceUUIDCurrentTime
	BatteryServiceUUID           = bluetooth.ServiceUUIDBattery
	DeviceInformationServiceUUID = bluetooth.ServiceUUIDDeviceInformation
)

// Omron-specific Service UUID
// The Omron Option Service contains the Record Access Control Point for retrieving stored records
var OmronOptionServiceUUID = bluetooth.NewUUID([16]byte{
	0x5D, 0xF5, 0xE8, 0x17, 0xA9, 0x45, 0x4F, 0x81,
	0x89, 0xC0, 0x3D, 0x4E, 0x97, 0x59, 0xC0, 0x7C,
})

// Standard BLE GATT Characteristic UUIDs
var (
	BloodPressureMeasurementUUID    = bluetooth.CharacteristicUUIDBloodPressureMeasurement
	BloodPressureFeatureUUID        = bluetooth.CharacteristicUUIDBloodPressureFeature
	RecordAccessControlPointUUID    = bluetooth.New16BitUUID(0x2A52)
	CurrentTimeUUID                 = bluetooth.CharacteristicUUIDCurrentTime
	BatteryLevelUUID                = bluetooth.CharacteristicUUIDBatteryLevel
	ManufacturerNameUUID            = bluetooth.CharacteristicUUIDManufacturerNameString
	ModelNumberUUID                 = bluetooth.CharacteristicUUIDModelNumberString
)

// BloodPressureMeasurement represents a parsed BLE blood pressure measurement
type BloodPressureMeasurement struct {
	Systolic    float32
	Diastolic   float32
	MeanAP      float32
	Unit        string // "mmHg" or "kPa"
	Timestamp   time.Time
	PulseRate   float32
	UserID      uint8
	HasPulse    bool
	HasUserID   bool
	HasTimestamp bool
	BodyMovement bool
	IrregularPulse bool
}

// ParseBloodPressureMeasurement parses a BLE Blood Pressure Measurement characteristic value
// Format defined in Bluetooth SIG Blood Pressure Service specification
func ParseBloodPressureMeasurement(data []byte) (*BloodPressureMeasurement, error) {
	if len(data) < 7 {
		return nil, fmt.Errorf("blood pressure measurement too short: %d bytes", len(data))
	}

	m := &BloodPressureMeasurement{}
	flags := data[0]

	// Bit 0: Blood pressure units (0 = mmHg, 1 = kPa)
	if flags&0x01 != 0 {
		m.Unit = "kPa"
	} else {
		m.Unit = "mmHg"
	}

	// Parse SFLOAT values (IEEE 11073 16-bit float)
	m.Systolic = parseSFLOAT(data[1:3])
	m.Diastolic = parseSFLOAT(data[3:5])
	m.MeanAP = parseSFLOAT(data[5:7])

	offset := 7

	// Bit 1: Timestamp present
	if flags&0x02 != 0 {
		if len(data) < offset+7 {
			return nil, fmt.Errorf("timestamp expected but data too short")
		}
		m.HasTimestamp = true
		m.Timestamp = parseDateTime(data[offset : offset+7])
		offset += 7
	}

	// Bit 2: Pulse rate present
	if flags&0x04 != 0 {
		if len(data) < offset+2 {
			return nil, fmt.Errorf("pulse rate expected but data too short")
		}
		m.HasPulse = true
		m.PulseRate = parseSFLOAT(data[offset : offset+2])
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
		// Bit 1: Cuff fit detection
		m.IrregularPulse = status&0x04 != 0
		// offset += 2
	}

	return m, nil
}

// parseSFLOAT parses an IEEE 11073 16-bit SFLOAT value
func parseSFLOAT(data []byte) float32 {
	if len(data) < 2 {
		return 0
	}
	raw := binary.LittleEndian.Uint16(data)

	// Special values
	if raw == 0x07FF { // NaN
		return 0
	}
	if raw == 0x0800 { // NRes (not at this resolution)
		return 0
	}
	if raw == 0x07FE { // +INFINITY
		return 0
	}
	if raw == 0x0802 { // -INFINITY
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

// parseDateTime parses a BLE DateTime characteristic value
func parseDateTime(data []byte) time.Time {
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

// encodeDateTime encodes a time.Time to BLE DateTime format
func encodeDateTime(t time.Time) []byte {
	data := make([]byte, 7)
	binary.LittleEndian.PutUint16(data[0:2], uint16(t.Year()))
	data[2] = byte(t.Month())
	data[3] = byte(t.Day())
	data[4] = byte(t.Hour())
	data[5] = byte(t.Minute())
	data[6] = byte(t.Second())
	return data
}

// CurrentTime represents the BLE Current Time characteristic
type CurrentTime struct {
	DateTime    time.Time
	DayOfWeek   uint8
	Fractions   uint8
	AdjustReason uint8
}

// encodeCurrentTime encodes a CurrentTime to BLE format
func encodeCurrentTime(t time.Time) []byte {
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

// RACP OpCodes (Record Access Control Point)
const (
	RACPOpReportStoredRecords       = 0x01
	RACPOpDeleteStoredRecords       = 0x02
	RACPOpAbortOperation            = 0x03
	RACPOpReportNumRecords          = 0x04
	RACPOpNumRecordsResponse        = 0x05
	RACPOpResponseCode              = 0x06
	// Omron extension
	RACPOpReportSequenceNumber      = 0x11
)

// RACP Operators
const (
	RACPOperatorNull                = 0x00
	RACPOperatorAllRecords          = 0x01
	RACPOperatorLessThanOrEqual     = 0x02
	RACPOperatorGreaterThanOrEqual  = 0x03
	RACPOperatorWithinRange         = 0x04
	RACPOperatorFirstRecord         = 0x05
	RACPOperatorLastRecord          = 0x06
)

// RACP Response Codes
const (
	RACPResponseSuccess             = 0x01
	RACPResponseOpCodeNotSupported  = 0x02
	RACPResponseInvalidOperator     = 0x03
	RACPResponseOperatorNotSupported = 0x04
	RACPResponseInvalidOperand      = 0x05
	RACPResponseNoRecordsFound      = 0x06
	RACPResponseAbortUnsuccessful   = 0x07
	RACPResponseProcedureNotComplete = 0x08
	RACPResponseOperandNotSupported = 0x09
)

// GATTClient handles standard BLE GATT service communication
type GATTClient struct {
	client              *Client
	debug               bool

	// Discovered characteristics
	bpMeasurementChar   bluetooth.DeviceCharacteristic
	bpFeatureChar       bluetooth.DeviceCharacteristic
	racpChar            bluetooth.DeviceCharacteristic
	currentTimeChar     bluetooth.DeviceCharacteristic
	batteryLevelChar    bluetooth.DeviceCharacteristic

	// Notification handling
	measurements        []*BloodPressureMeasurement
	measurementsMu      sync.Mutex
	racpResponse        []byte
	racpResponseCh      chan []byte

	// Pairing signals
	pairingSignalCh     chan struct{}
	timeNotificationCh  chan []byte

	hasRACPChar         bool
}

// NewGATTClient creates a new GATT client for standard BLE services
func NewGATTClient(client *Client, debug bool) *GATTClient {
	return &GATTClient{
		client:             client,
		debug:              debug,
		racpResponseCh:     make(chan []byte, 1),
		pairingSignalCh:    make(chan struct{}, 1),
		timeNotificationCh: make(chan []byte, 1),
	}
}

// DiscoverServices discovers and caches standard BLE service characteristics
func (g *GATTClient) DiscoverServices(ctx context.Context) error {
	if !g.client.IsConnected() {
		return fmt.Errorf("not connected to device")
	}

	device := g.client.GetDevice()

	// Discover Blood Pressure Service
	services, err := device.DiscoverServices([]bluetooth.UUID{BloodPressureServiceUUID})
	if err != nil {
		return fmt.Errorf("failed to discover blood pressure service: %w", err)
	}

	if len(services) == 0 {
		return fmt.Errorf("blood pressure service not found")
	}

	bpService := services[0]
	chars, err := bpService.DiscoverCharacteristics(nil)
	if err != nil {
		return fmt.Errorf("failed to discover BP characteristics: %w", err)
	}

	for _, char := range chars {
		switch char.UUID() {
		case BloodPressureMeasurementUUID:
			g.bpMeasurementChar = char
			if g.debug {
				fmt.Println("[GATT] Found Blood Pressure Measurement characteristic")
			}
		case BloodPressureFeatureUUID:
			g.bpFeatureChar = char
			if g.debug {
				fmt.Println("[GATT] Found Blood Pressure Feature characteristic")
			}
		case RecordAccessControlPointUUID:
			g.racpChar = char
			g.hasRACPChar = true
			if g.debug {
				fmt.Println("[GATT] Found Record Access Control Point characteristic")
			}
		}
	}

	// Discover Current Time Service
	timeServices, err := device.DiscoverServices([]bluetooth.UUID{CurrentTimeServiceUUID})
	if err == nil && len(timeServices) > 0 {
		timeChars, err := timeServices[0].DiscoverCharacteristics(nil)
		if err == nil {
			for _, char := range timeChars {
				if char.UUID() == CurrentTimeUUID {
					g.currentTimeChar = char
					if g.debug {
						fmt.Println("[GATT] Found Current Time characteristic")
					}
				}
			}
		}
	}

	// Discover Battery Service
	batteryServices, err := device.DiscoverServices([]bluetooth.UUID{BatteryServiceUUID})
	if err == nil && len(batteryServices) > 0 {
		batteryChars, err := batteryServices[0].DiscoverCharacteristics(nil)
		if err == nil {
			for _, char := range batteryChars {
				if char.UUID() == BatteryLevelUUID {
					g.batteryLevelChar = char
					if g.debug {
						fmt.Println("[GATT] Found Battery Level characteristic")
					}
				}
			}
		}
	}

	// Discover Omron Option Service (contains RACP for stored records)
	// This is a custom Omron service with UUID 5DF5E817-A945-4F81-89C0-3D4E9759C07C
	omronServices, err := device.DiscoverServices([]bluetooth.UUID{OmronOptionServiceUUID})
	if err == nil && len(omronServices) > 0 {
		if g.debug {
			fmt.Println("[GATT] Found Omron Option Service")
		}
		omronChars, err := omronServices[0].DiscoverCharacteristics(nil)
		if err == nil {
			for _, char := range omronChars {
				if char.UUID() == RecordAccessControlPointUUID {
					g.racpChar = char
					g.hasRACPChar = true
					if g.debug {
						fmt.Println("[GATT] Found RACP on Omron Option Service")
					}
				}
			}
		}
	} else if g.debug {
		fmt.Println("[GATT] Omron Option Service not found (records via notifications only)")
	}

	return nil
}

// EnableNotifications enables notifications for BP measurements and RACP
func (g *GATTClient) EnableNotifications(ctx context.Context) error {
	// Enable BP measurement indications
	if g.bpMeasurementChar.UUID() != (bluetooth.UUID{}) {
		err := g.bpMeasurementChar.EnableNotifications(func(buf []byte) {
			if g.debug {
				fmt.Printf("[GATT] BP Measurement notification: %x\n", buf)
			}
			measurement, err := ParseBloodPressureMeasurement(buf)
			if err != nil {
				if g.debug {
					fmt.Printf("[GATT] Failed to parse measurement: %v\n", err)
				}
				return
			}
			g.measurementsMu.Lock()
			g.measurements = append(g.measurements, measurement)
			g.measurementsMu.Unlock()
		})
		if err != nil {
			return fmt.Errorf("failed to enable BP notifications: %w", err)
		}
		if g.debug {
			fmt.Println("[GATT] Enabled BP measurement notifications")
		}
	}

	// Enable RACP indications
	if g.hasRACPChar {
		err := g.racpChar.EnableNotifications(func(buf []byte) {
			if g.debug {
				fmt.Printf("[GATT] RACP response: %x\n", buf)
			}
			select {
			case g.racpResponseCh <- buf:
			default:
				// Channel full, drop old response
				select {
				case <-g.racpResponseCh:
				default:
				}
				g.racpResponseCh <- buf
			}
		})
		if err != nil {
			return fmt.Errorf("failed to enable RACP notifications: %w", err)
		}
		if g.debug {
			fmt.Println("[GATT] Enabled RACP notifications")
		}
	}

	// Enable Current Time notifications if available - this is key for pairing
	if g.currentTimeChar.UUID() != (bluetooth.UUID{}) {
		err := g.currentTimeChar.EnableNotifications(func(buf []byte) {
			if g.debug {
				fmt.Printf("[GATT] Current Time notification: %x\n", buf)
			}
			// Signal that we received a time notification (pairing indicator)
			select {
			case g.timeNotificationCh <- buf:
			default:
			}
		})
		if err != nil {
			if g.debug {
				fmt.Printf("[GATT] Failed to enable time notifications: %v\n", err)
			}
		} else if g.debug {
			fmt.Println("[GATT] Enabled current time notifications")
		}
	}

	// Enable Battery Level notifications - also a pairing indicator
	if g.batteryLevelChar.UUID() != (bluetooth.UUID{}) {
		err := g.batteryLevelChar.EnableNotifications(func(buf []byte) {
			if g.debug {
				fmt.Printf("[GATT] Battery Level notification: %d%%\n", buf[0])
			}
			// Signal pairing success
			select {
			case g.pairingSignalCh <- struct{}{}:
			default:
			}
		})
		if err != nil {
			if g.debug {
				fmt.Printf("[GATT] Failed to enable battery notifications: %v\n", err)
			}
		} else if g.debug {
			fmt.Println("[GATT] Enabled battery level notifications")
		}
	}

	return nil
}

// WaitForPairing waits for the device to complete the pairing handshake
// Omron devices signal successful pairing by:
// 1. Sending a currentTime notification (we must respond with time sync)
// 2. Sending a batteryLevel notification
// We ALWAYS sync time to ensure the device is properly configured
func (g *GATTClient) WaitForPairing(ctx context.Context, timeout time.Duration) error {
	if g.debug {
		fmt.Println("[GATT] Waiting for pairing handshake...")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for either time notification or battery notification
	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("pairing timeout - no response from device. Make sure device is in pairing mode (-P- on display)")
		case timeData := <-g.timeNotificationCh:
			if g.debug {
				fmt.Printf("[GATT] Received time notification: %x\n", timeData)
			}
			// Respond with current time - this completes the pairing
			if err := g.SyncTime(ctx); err != nil {
				if g.debug {
					fmt.Printf("[GATT] Time sync failed: %v\n", err)
				}
				// Continue anyway, pairing may still work
			} else {
				if g.debug {
					fmt.Println("[GATT] Time sync completed")
				}
			}
			// Wait for device to process
			time.Sleep(1 * time.Second)
			return nil
		case <-g.pairingSignalCh:
			if g.debug {
				fmt.Println("[GATT] Received battery notification")
			}
			// Device needs time sync to complete pairing properly
			if g.debug {
				fmt.Println("[GATT] Syncing time to complete pairing...")
			}
			if err := g.SyncTime(ctx); err != nil {
				if g.debug {
					fmt.Printf("[GATT] Time sync warning: %v\n", err)
				}
			} else {
				if g.debug {
					fmt.Println("[GATT] Time sync completed")
				}
			}
			// Give device a moment to process time sync
			time.Sleep(500 * time.Millisecond)
			return nil
		}
	}
}

// SyncTime synchronizes device time with system time
func (g *GATTClient) SyncTime(ctx context.Context) error {
	if g.currentTimeChar.UUID() == (bluetooth.UUID{}) {
		return fmt.Errorf("current time characteristic not available")
	}

	timeData := encodeCurrentTime(time.Now())
	_, err := g.currentTimeChar.WriteWithoutResponse(timeData)
	if err != nil {
		return fmt.Errorf("failed to write current time: %w", err)
	}

	if g.debug {
		fmt.Println("[GATT] Time synchronized")
	}
	return nil
}

// WaitForDeviceDisconnect waits for the device to disconnect itself
// Omron devices disconnect themselves after successful pairing
func (g *GATTClient) WaitForDeviceDisconnect(ctx context.Context, timeout time.Duration) error {
	if g.debug {
		fmt.Println("[GATT] Waiting for device to disconnect...")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
		case <-ticker.C:
			if !g.client.IsConnected() {
				if g.debug {
					fmt.Println("[GATT] Device disconnected")
				}
				return nil
			}
		}
	}
}

// RequestAllRecords requests all stored records via RACP
func (g *GATTClient) RequestAllRecords(ctx context.Context) error {
	if !g.hasRACPChar {
		return fmt.Errorf("RACP characteristic not available")
	}

	// Clear previous measurements
	g.measurementsMu.Lock()
	g.measurements = nil
	g.measurementsMu.Unlock()

	// Send Report All Records command
	cmd := []byte{RACPOpReportStoredRecords, RACPOperatorAllRecords}
	_, err := g.racpChar.WriteWithoutResponse(cmd)
	if err != nil {
		return fmt.Errorf("failed to send RACP command: %w", err)
	}

	if g.debug {
		fmt.Println("[GATT] Sent Report All Records command")
	}

	// Wait for response
	select {
	case resp := <-g.racpResponseCh:
		if len(resp) >= 4 && resp[0] == RACPOpResponseCode {
			responseCode := resp[3]
			if responseCode == RACPResponseSuccess {
				if g.debug {
					fmt.Println("[GATT] RACP: Success")
				}
				return nil
			} else if responseCode == RACPResponseNoRecordsFound {
				if g.debug {
					fmt.Println("[GATT] RACP: No records found")
				}
				return nil
			}
			return fmt.Errorf("RACP error: response code %d", responseCode)
		}
		return fmt.Errorf("unexpected RACP response: %x", resp)
	case <-time.After(30 * time.Second):
		return fmt.Errorf("RACP response timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetMeasurements returns collected measurements
func (g *GATTClient) GetMeasurements() []*BloodPressureMeasurement {
	g.measurementsMu.Lock()
	defer g.measurementsMu.Unlock()
	result := make([]*BloodPressureMeasurement, len(g.measurements))
	copy(result, g.measurements)
	return result
}

// GetBatteryLevel reads the battery level
func (g *GATTClient) GetBatteryLevel() (uint8, error) {
	if g.batteryLevelChar.UUID() == (bluetooth.UUID{}) {
		return 0, fmt.Errorf("battery level characteristic not available")
	}

	buf := make([]byte, 1)
	_, err := g.batteryLevelChar.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

// HasRACPSupport returns true if the device supports RACP
func (g *GATTClient) HasRACPSupport() bool {
	return g.hasRACPChar
}
