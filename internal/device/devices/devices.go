// Package devices contains all Omron device driver implementations
package devices

import (
	"errors"
	"time"

	"omblego/internal/device"
)

func init() {
	// Register all device drivers
	device.Register("hem-7322t", NewHEM7322T)
	device.Register("hem-7600t", NewHEM7600T)
	device.Register("hem-6232t", NewHEM6232T)
	device.Register("hem-7530t", NewHEM7530T)
	device.Register("hem-7155t", NewHEM7155T)
	device.Register("hem-7150t", NewHEM7150T)
	device.Register("hem-7342t", NewHEM7342T)
	device.Register("hem-7361t", NewHEM7361T)

	// Standard GATT protocol devices
	device.Register("bp7000", NewBP7000)
	device.Register("bp5250", NewBP5250)
	device.Register("evolv", NewEvolv)
}

// ============================================================================
// Big Endian Devices - Format A (HEM-7322T style)
// Record: DIA(0-7), SYS(8-15)+25, Year(16-23)+2000, BPM(24-31), MOV(32), IHB(33),
//         Month(34-37), Day(38-42), Hour(43-47), Minute(52-57), Second(58-63)
// ============================================================================

// HEM7322T implements the driver for Omron HEM-7322T (M700 Intelli IT)
type HEM7322T struct {
	config device.Config
}

func NewHEM7322T() device.Driver {
	return &HEM7322T{
		config: device.Config{
			Name:                  "HEM-7322T",
			Endianness:            device.BigEndian,
			UserStartAddresses:    []uint16{0x02ac, 0x0824},
			RecordsPerUser:        []int{100, 100},
			RecordByteSize:        0x0e,
			TransmissionBlockSize: 0x38,
			SettingsReadAddress:   0x0260,
			SettingsWriteAddress:  0x0286,
			UnreadRecordsRange:    [2]int{0x00, 0x08},
			TimeSyncRange:         [2]int{0x14, 0x1e},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM7322T) GetConfig() device.Config { return d.config }

func (d *HEM7322T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseBigEndianFormatA(data, d.config.Endianness)
}

func (d *HEM7322T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncBigEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// HEM7600T implements the driver for Omron HEM-7600T (Omron Evolv) - single user
type HEM7600T struct {
	config device.Config
}

func NewHEM7600T() device.Driver {
	return &HEM7600T{
		config: device.Config{
			Name:                  "HEM-7600T",
			Endianness:            device.BigEndian,
			UserStartAddresses:    []uint16{0x02ac},
			RecordsPerUser:        []int{100},
			RecordByteSize:        0x0e,
			TransmissionBlockSize: 0x38,
			SettingsReadAddress:   0x0260,
			SettingsWriteAddress:  0x0286,
			UnreadRecordsRange:    [2]int{0x00, 0x08},
			TimeSyncRange:         [2]int{0x14, 0x1e},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM7600T) GetConfig() device.Config { return d.config }

func (d *HEM7600T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseBigEndianFormatA(data, d.config.Endianness)
}

func (d *HEM7600T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncBigEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// ============================================================================
// Big Endian Devices - Format B (HEM-6232T style)
// Record: DIA(0-7), SYS(8-15)+25, Year(18-23)+2000, BPM(24-31), IHB(32), MOV(33),
//         Month(34-37), Day(38-42), Hour(43-47), Minute(52-57), Second(58-63)
// Note: Year starts at bit 18 (not 16), and IHB/MOV are swapped
// ============================================================================

// HEM6232T implements the driver for Omron HEM-6232T (wrist monitor)
type HEM6232T struct {
	config device.Config
}

func NewHEM6232T() device.Driver {
	return &HEM6232T{
		config: device.Config{
			Name:                  "HEM-6232T",
			Endianness:            device.BigEndian,
			UserStartAddresses:    []uint16{0x2e8, 0x860},
			RecordsPerUser:        []int{100, 100},
			RecordByteSize:        0x0e,
			TransmissionBlockSize: 0x38,
			SettingsReadAddress:   0x0260,
			SettingsWriteAddress:  0x02A4,
			UnreadRecordsRange:    [2]int{0x00, 0x08},
			TimeSyncRange:         [2]int{0x14, 0x1e},
			SupportsTimeSync:      false, // Not yet verified
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM6232T) GetConfig() device.Config { return d.config }

func (d *HEM6232T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseBigEndianFormatB(data, d.config.Endianness)
}

func (d *HEM6232T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return nil, errors.New("time sync not supported for HEM-6232T")
}

// HEM7530T implements the driver for Omron HEM-7530T (Omron Complete) - no EKG support
type HEM7530T struct {
	config device.Config
}

func NewHEM7530T() device.Driver {
	return &HEM7530T{
		config: device.Config{
			Name:                  "HEM-7530T",
			Endianness:            device.BigEndian,
			UserStartAddresses:    []uint16{0x2e8},
			RecordsPerUser:        []int{90},
			RecordByteSize:        0x0e,
			TransmissionBlockSize: 0x10,
			SettingsReadAddress:   0x0260,
			SettingsWriteAddress:  0x02a4,
			UnreadRecordsRange:    [2]int{0x00, 0x08},
			TimeSyncRange:         [2]int{0x14, 0x1e},
			SupportsTimeSync:      false,
			SupportsUnreadCounter: false,
		},
	}
}

func (d *HEM7530T) GetConfig() device.Config { return d.config }

func (d *HEM7530T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseBigEndianFormatB(data, d.config.Endianness)
}

func (d *HEM7530T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return nil, errors.New("time sync not supported for HEM-7530T")
}

// ============================================================================
// Little Endian Devices
// Record: Minute(68-73), Second(74-79), MOV(80), IHB(81), Month(82-85),
//         Day(86-90), Hour(91-95), Year(98-103)+2000, BPM(104-111),
//         DIA(112-119), SYS(120-127)+25
// ============================================================================

// HEM7155T implements the driver for Omron HEM-7155T (M400/M4/X4 smart)
type HEM7155T struct {
	config device.Config
}

func NewHEM7155T() device.Driver {
	return &HEM7155T{
		config: device.Config{
			Name:                  "HEM-7155T",
			Endianness:            device.LittleEndian,
			UserStartAddresses:    []uint16{0x0098, 0x0458},
			RecordsPerUser:        []int{60, 60},
			RecordByteSize:        0x10,
			TransmissionBlockSize: 0x10,
			SettingsReadAddress:   0x0010,
			SettingsWriteAddress:  0x0054,
			UnreadRecordsRange:    [2]int{0x00, 0x10},
			TimeSyncRange:         [2]int{0x2C, 0x3C},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM7155T) GetConfig() device.Config { return d.config }

func (d *HEM7155T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseLittleEndianFormat(data, d.config.Endianness)
}

func (d *HEM7155T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncLittleEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// HEM7150T implements the driver for Omron HEM-7150T (BP7250) - single user
type HEM7150T struct {
	config device.Config
}

func NewHEM7150T() device.Driver {
	return &HEM7150T{
		config: device.Config{
			Name:                  "HEM-7150T",
			Endianness:            device.LittleEndian,
			UserStartAddresses:    []uint16{0x0098},
			RecordsPerUser:        []int{60},
			RecordByteSize:        0x10,
			TransmissionBlockSize: 0x10,
			SettingsReadAddress:   0x0010,
			SettingsWriteAddress:  0x0054,
			UnreadRecordsRange:    [2]int{0x00, 0x10},
			TimeSyncRange:         [2]int{0x2C, 0x3C},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM7150T) GetConfig() device.Config { return d.config }

func (d *HEM7150T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseLittleEndianFormat(data, d.config.Endianness)
}

func (d *HEM7150T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncLittleEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// HEM7342T implements the driver for Omron HEM-7342T (BP7450)
type HEM7342T struct {
	config device.Config
}

func NewHEM7342T() device.Driver {
	return &HEM7342T{
		config: device.Config{
			Name:                  "HEM-7342T",
			Endianness:            device.LittleEndian,
			UserStartAddresses:    []uint16{0x0098, 0x06D8},
			RecordsPerUser:        []int{100, 100},
			RecordByteSize:        0x10,
			TransmissionBlockSize: 0x10,
			SettingsReadAddress:   0x0010,
			SettingsWriteAddress:  0x0054,
			UnreadRecordsRange:    [2]int{0x00, 0x10},
			TimeSyncRange:         [2]int{0x2C, 0x3C},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM7342T) GetConfig() device.Config { return d.config }

func (d *HEM7342T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseLittleEndianFormat(data, d.config.Endianness)
}

func (d *HEM7342T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncLittleEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// HEM7361T implements the driver for Omron HEM-7361T (M500/M7 Intelli IT)
type HEM7361T struct {
	config device.Config
}

func NewHEM7361T() device.Driver {
	return &HEM7361T{
		config: device.Config{
			Name:                  "HEM-7361T",
			Endianness:            device.LittleEndian,
			UserStartAddresses:    []uint16{0x0098, 0x06D8},
			RecordsPerUser:        []int{100, 100},
			RecordByteSize:        0x10,
			TransmissionBlockSize: 0x10,
			SettingsReadAddress:   0x0010,
			SettingsWriteAddress:  0x0054,
			UnreadRecordsRange:    [2]int{0x00, 0x10},
			TimeSyncRange:         [2]int{0x2C, 0x3C},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *HEM7361T) GetConfig() device.Config { return d.config }

func (d *HEM7361T) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseLittleEndianFormat(data, d.config.Endianness)
}

func (d *HEM7361T) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncLittleEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// ============================================================================
// Shared parsing functions
// ============================================================================

// parseBigEndianFormatA parses record format for HEM-7322T, HEM-7600T
func parseBigEndianFormatA(data []byte, endianness device.Endianness) (*device.BloodPressureRecord, error) {
	if len(data) < 14 {
		return nil, errors.New("record too short")
	}

	dia := device.ExtractBits(data, endianness, 0, 7)
	sys := device.ExtractBits(data, endianness, 8, 15) + 25
	year := device.ExtractBits(data, endianness, 16, 23) + 2000
	bpm := device.ExtractBits(data, endianness, 24, 31)
	mov := device.ExtractBits(data, endianness, 32, 32)
	ihb := device.ExtractBits(data, endianness, 33, 33)
	month := device.ExtractBits(data, endianness, 34, 37)
	day := device.ExtractBits(data, endianness, 38, 42)
	hour := device.ExtractBits(data, endianness, 43, 47)
	minute := device.ExtractBits(data, endianness, 52, 57)
	second := device.ExtractBits(data, endianness, 58, 63)

	// Clamp seconds to valid range
	if second > 59 {
		second = 59
	}

	return &device.BloodPressureRecord{
		Timestamp: time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local),
		Systolic:  sys,
		Diastolic: dia,
		Pulse:     bpm,
		Movement:  mov == 1,
		IHB:       ihb == 1,
	}, nil
}

// parseBigEndianFormatB parses record format for HEM-6232T, HEM-7530T
// Differences from format A: Year at bits 18-23, IHB/MOV swapped
func parseBigEndianFormatB(data []byte, endianness device.Endianness) (*device.BloodPressureRecord, error) {
	if len(data) < 14 {
		return nil, errors.New("record too short")
	}

	dia := device.ExtractBits(data, endianness, 0, 7)
	sys := device.ExtractBits(data, endianness, 8, 15) + 25
	year := device.ExtractBits(data, endianness, 18, 23) + 2000 // Different from format A
	bpm := device.ExtractBits(data, endianness, 24, 31)
	ihb := device.ExtractBits(data, endianness, 32, 32) // Swapped
	mov := device.ExtractBits(data, endianness, 33, 33) // Swapped
	month := device.ExtractBits(data, endianness, 34, 37)
	day := device.ExtractBits(data, endianness, 38, 42)
	hour := device.ExtractBits(data, endianness, 43, 47)
	minute := device.ExtractBits(data, endianness, 52, 57)
	second := device.ExtractBits(data, endianness, 58, 63)

	if second > 59 {
		second = 59
	}

	return &device.BloodPressureRecord{
		Timestamp: time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local),
		Systolic:  sys,
		Diastolic: dia,
		Pulse:     bpm,
		Movement:  mov == 1,
		IHB:       ihb == 1,
	}, nil
}

// parseLittleEndianFormat parses record format for little-endian devices
func parseLittleEndianFormat(data []byte, endianness device.Endianness) (*device.BloodPressureRecord, error) {
	if len(data) < 16 {
		return nil, errors.New("record too short")
	}

	minute := device.ExtractBits(data, endianness, 68, 73)
	second := device.ExtractBits(data, endianness, 74, 79)
	mov := device.ExtractBits(data, endianness, 80, 80)
	ihb := device.ExtractBits(data, endianness, 81, 81)
	month := device.ExtractBits(data, endianness, 82, 85)
	day := device.ExtractBits(data, endianness, 86, 90)
	hour := device.ExtractBits(data, endianness, 91, 95)
	year := device.ExtractBits(data, endianness, 98, 103) + 2000
	bpm := device.ExtractBits(data, endianness, 104, 111)
	dia := device.ExtractBits(data, endianness, 112, 119)
	sys := device.ExtractBits(data, endianness, 120, 127) + 25

	if second > 59 {
		second = 59
	}

	return &device.BloodPressureRecord{
		Timestamp: time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local),
		Systolic:  sys,
		Diastolic: dia,
		Pulse:     bpm,
		Movement:  mov == 1,
		IHB:       ihb == 1,
	}, nil
}

// ============================================================================
// Time sync functions
// ============================================================================

// buildTimeSyncBigEndian creates time sync bytes for big-endian devices
func buildTimeSyncBigEndian(cachedSettings []byte, syncRange [2]int, newTime time.Time) ([]byte, error) {
	if len(cachedSettings) < syncRange[1] {
		return nil, errors.New("settings too short for time sync")
	}

	original := cachedSettings[syncRange[0]:syncRange[1]]
	result := make([]byte, len(original))

	// Keep first 2 bytes unchanged
	copy(result, original[:2])

	// Write time: month, year-2000, hour, day, second, minute
	result[2] = byte(newTime.Month())
	result[3] = byte(newTime.Year() - 2000)
	result[4] = byte(newTime.Hour())
	result[5] = byte(newTime.Day())
	result[6] = byte(newTime.Second())
	result[7] = byte(newTime.Minute())
	result[8] = 0x00

	// Calculate checksum (sum of bytes & 0xFF)
	var sum byte
	for i := 0; i < 9; i++ {
		sum += result[i]
	}
	result[9] = sum

	return result, nil
}

// buildTimeSyncLittleEndian creates time sync bytes for little-endian devices
func buildTimeSyncLittleEndian(cachedSettings []byte, syncRange [2]int, newTime time.Time) ([]byte, error) {
	if len(cachedSettings) < syncRange[1] {
		return nil, errors.New("settings too short for time sync")
	}

	original := cachedSettings[syncRange[0]:syncRange[1]]
	result := make([]byte, len(original))

	// Keep first 8 bytes unchanged
	copy(result, original[:8])

	// Write time: year-2000, month, day, hour, minute, second
	result[8] = byte(newTime.Year() - 2000)
	result[9] = byte(newTime.Month())
	result[10] = byte(newTime.Day())
	result[11] = byte(newTime.Hour())
	result[12] = byte(newTime.Minute())
	result[13] = byte(newTime.Second())

	// Calculate checksum (sum of first 14 bytes & 0xFF)
	var sum byte
	for i := 0; i < 14; i++ {
		sum += result[i]
	}
	result[14] = sum
	result[15] = 0x00

	return result, nil
}

// ============================================================================
// US Market Devices (BP7000, BP5250, EVOLV)
// These use the same custom Omron protocol as HEM-7600T/HEM-7150T
// ============================================================================

// BP7000 implements the driver for Omron BP7000 (EVOLV)
// Uses big endian memory layout like HEM-7600T but may have different record format
type BP7000 struct {
	config device.Config
}

func NewBP7000() device.Driver {
	return &BP7000{
		config: device.Config{
			Name:                  "BP7000",
			Protocol:              device.ProtocolOmronCustom,
			BLENamePrefix:         "BLEsmart_0000011F", // Pairing mode prefix
			Endianness:            device.BigEndian,
			UserStartAddresses:    []uint16{0x02ac},
			RecordsPerUser:        []int{100},
			RecordByteSize:        0x0e,
			TransmissionBlockSize: 0x38,
			SettingsReadAddress:   0x0260,
			SettingsWriteAddress:  0x0286,
			UnreadRecordsRange:    [2]int{0x00, 0x08},
			TimeSyncRange:         [2]int{0x14, 0x1e},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *BP7000) GetConfig() device.Config { return d.config }

func (d *BP7000) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseBP7000Format(data)
}

func (d *BP7000) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncBigEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}

// parseBP7000Format parses record format for BP7000
// BP7000 uses same bit-packed format as HEM-7600T for timestamps.
// If timestamps are all identical, it means device clock was never synced.
// Use -t flag to sync time, then new measurements will have correct timestamps.
func parseBP7000Format(data []byte) (*device.BloodPressureRecord, error) {
	if len(data) < 14 {
		return nil, errors.New("record too short")
	}

	// BP7000 record format (same as HEM-7600T):
	// Bits 0-7:   diastolic
	// Bits 8-15:  systolic - 25
	// Bits 16-23: year - 2000
	// Bits 24-31: pulse (BPM)
	// Bit 32:     movement flag
	// Bit 33:     IHB flag
	// Bits 34-37: month
	// Bits 38-42: day
	// Bits 43-47: hour
	// Bits 52-57: minute
	// Bits 58-63: second

	dia := device.ExtractBits(data, device.BigEndian, 0, 7)
	sys := device.ExtractBits(data, device.BigEndian, 8, 15) + 25
	year := device.ExtractBits(data, device.BigEndian, 16, 23) + 2000
	bpm := device.ExtractBits(data, device.BigEndian, 24, 31)
	mov := device.ExtractBits(data, device.BigEndian, 32, 32)
	ihb := device.ExtractBits(data, device.BigEndian, 33, 33)
	month := device.ExtractBits(data, device.BigEndian, 34, 37)
	day := device.ExtractBits(data, device.BigEndian, 38, 42)
	hour := device.ExtractBits(data, device.BigEndian, 43, 47)
	minute := device.ExtractBits(data, device.BigEndian, 52, 57)
	second := device.ExtractBits(data, device.BigEndian, 58, 63)

	// Clamp values to valid ranges
	if second > 59 {
		second = 59
	}
	if month < 1 || month > 12 {
		month = 1
	}
	if day < 1 || day > 31 {
		day = 1
	}

	return &device.BloodPressureRecord{
		Timestamp: time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local),
		Systolic:  sys,
		Diastolic: dia,
		Pulse:     bpm,
		Movement:  mov == 1,
		IHB:       ihb == 1,
	}, nil
}

// BP5250 implements the driver for Omron BP5250
// Uses standard BLE Blood Pressure Service
type BP5250 struct {
	config device.Config
}

func NewBP5250() device.Driver {
	return &BP5250{
		config: device.Config{
			Name:             "BP5250",
			Protocol:         device.ProtocolStandardGATT,
			BLENamePrefix:    "BLEsmart_00000160", // Pairing mode prefix
			SupportsTimeSync: true,
		},
	}
}

func (d *BP5250) GetConfig() device.Config { return d.config }

func (d *BP5250) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return nil, errors.New("BP5250 uses GATT protocol, not EEPROM parsing")
}

func (d *BP5250) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return nil, errors.New("BP5250 uses GATT protocol for time sync")
}

// Evolv implements the driver for Omron EVOLV
// Uses same format as BP7000
type Evolv struct {
	config device.Config
}

func NewEvolv() device.Driver {
	return &Evolv{
		config: device.Config{
			Name:                  "EVOLV",
			Protocol:              device.ProtocolOmronCustom,
			BLENamePrefix:         "BLEsmart_0000021F", // Pairing mode prefix
			Endianness:            device.BigEndian,
			UserStartAddresses:    []uint16{0x02ac},
			RecordsPerUser:        []int{100},
			RecordByteSize:        0x0e,
			TransmissionBlockSize: 0x38,
			SettingsReadAddress:   0x0260,
			SettingsWriteAddress:  0x0286,
			UnreadRecordsRange:    [2]int{0x00, 0x08},
			TimeSyncRange:         [2]int{0x14, 0x1e},
			SupportsTimeSync:      true,
			SupportsUnreadCounter: true,
		},
	}
}

func (d *Evolv) GetConfig() device.Config { return d.config }

func (d *Evolv) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
	return parseBP7000Format(data)
}

func (d *Evolv) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return buildTimeSyncBigEndian(cachedSettings, d.config.TimeSyncRange, newTime)
}
