// Package drivers provides the device driver interface and registry for omblego.
// Device drivers implement device-specific logic for parsing records and building
// time sync commands.
package drivers

import (
	"time"
)

// ProtocolType identifies which BLE protocol a device uses
type ProtocolType string

const (
	// ProtocolOmronCustom uses the custom Omron EEPROM-based protocol
	ProtocolOmronCustom ProtocolType = "omron_custom"
	// ProtocolStandardGATT uses standard BLE GATT services (Blood Pressure Service)
	ProtocolStandardGATT ProtocolType = "standard_gatt"
)

// Endianness represents byte order for bit extraction
type Endianness string

const (
	BigEndian    Endianness = "big"
	LittleEndian Endianness = "little"
)

// BloodPressureRecord represents a single measurement
type BloodPressureRecord struct {
	Timestamp time.Time
	Systolic  int  // mmHg
	Diastolic int  // mmHg
	Pulse     int  // BPM
	Movement  bool // Movement detected during measurement
	IHB       bool // Irregular heartbeat detected
}

// DeviceConfig holds device-specific memory layout configuration
type DeviceConfig struct {
	Name                  string       // Device model name (e.g., "HEM-7322T")
	Protocol              ProtocolType // Which BLE protocol to use
	BLENamePrefix         string       // BLE local name prefix for device identification
	Endianness            Endianness   // Byte order for bit extraction
	UserStartAddresses    []uint16     // EEPROM addresses for each user's records
	RecordsPerUser        []int        // Number of record slots per user
	RecordByteSize        int          // Size of single record in bytes
	TransmissionBlockSize int          // BLE read block size
	SettingsReadAddress   uint16       // Settings region start (for reading)
	SettingsWriteAddress  uint16       // Settings region start (for writing)
	UnreadRecordsRange    [2]int       // [start, end] byte range in settings
	TimeSyncRange         [2]int       // [start, end] byte range in settings
	SupportsTimeSync      bool         // Whether time sync is implemented
	SupportsUnreadCounter bool         // Whether unread counter is implemented
}

// Driver is the interface all device drivers must implement
type Driver interface {
	// Name returns the device model name
	Name() string

	// Config returns the device configuration
	Config() DeviceConfig

	// ParseRecord parses raw bytes into a BloodPressureRecord
	ParseRecord(data []byte) (*BloodPressureRecord, error)

	// BuildTimeSyncBytes creates the bytes to write for time sync (if supported)
	BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error)
}
