package devices

import (
	"testing"
	"time"

	"omblego/internal/device"
)

func TestDeviceRegistration(t *testing.T) {
	// Test that all devices are registered
	expectedDevices := []string{
		"hem-7322t",
		"hem-7600t",
		"hem-6232t",
		"hem-7530t",
		"hem-7155t",
		"hem-7150t",
		"hem-7342t",
		"hem-7361t",
	}

	for _, name := range expectedDevices {
		t.Run(name, func(t *testing.T) {
			driver, err := device.GetDriver(name)
			if err != nil {
				t.Errorf("GetDriver(%s) error: %v", name, err)
				return
			}
			if driver == nil {
				t.Errorf("GetDriver(%s) returned nil", name)
			}
		})
	}
}

func TestHEM7322T_Config(t *testing.T) {
	driver := NewHEM7322T()
	config := driver.GetConfig()

	if config.Name != "HEM-7322T" {
		t.Errorf("Name = %s, want HEM-7322T", config.Name)
	}
	if config.Endianness != device.BigEndian {
		t.Errorf("Endianness = %s, want big", config.Endianness)
	}
	if len(config.UserStartAddresses) != 2 {
		t.Errorf("UserStartAddresses count = %d, want 2", len(config.UserStartAddresses))
	}
	if config.RecordByteSize != 0x0e {
		t.Errorf("RecordByteSize = %d, want 14", config.RecordByteSize)
	}
	if !config.SupportsTimeSync {
		t.Error("SupportsTimeSync = false, want true")
	}
	if !config.SupportsUnreadCounter {
		t.Error("SupportsUnreadCounter = false, want true")
	}
}

func TestHEM7322T_ParseRecord(t *testing.T) {
	driver := NewHEM7322T()

	// HEM-7322T format (big endian, 14 bytes):
	// Bits 0-7: DIA
	// Bits 8-15: SYS-25
	// Bits 16-23: Year-2000
	// Bits 24-31: BPM
	// Bit 32: MOV, Bit 33: IHB
	// Bits 34-37: Month (4 bits)
	// Bits 38-42: Day (5 bits)
	// Bits 43-47: Hour (5 bits)
	// Bits 52-57: Minute (6 bits)
	// Bits 58-63: Second (6 bits)

	// Create a record with known values:
	// DIA=80, SYS=120 (95+25), Year=2024 (24+2000), BPM=72
	// MOV=0, IHB=0, Month=6, Day=15, Hour=14, Minute=30, Second=45
	//
	// Byte 4: MOV(0) IHB(0) Month[3:0](0110) Day[4:3](01) = 0b00_0110_01 = 0x19
	// Byte 5: Day[2:0](111) Hour[4:0](01110) = 0b111_01110 = 0xEE
	// Byte 6: unused[3:0](0000) Minute[5:2](0111) = 0b0000_0111 = 0x07
	// Byte 7: Minute[1:0](10) Second[5:0](101101) = 0b10_101101 = 0xAD
	record := []byte{
		0x50, // byte 0: DIA = 80
		0x5F, // byte 1: SYS-25 = 95 (SYS = 120)
		0x18, // byte 2: Year-2000 = 24 (Year = 2024)
		0x48, // byte 3: BPM = 72
		0x19, // byte 4: MOV=0, IHB=0, Month=6, Day upper bits
		0xEE, // byte 5: Day lower bits, Hour=14
		0x07, // byte 6: Minute upper bits
		0xAD, // byte 7: Minute lower bits, Second=45
		0x00, // byte 8
		0x00, // byte 9
		0x00, // byte 10
		0x00, // byte 11
		0x00, // byte 12
		0x00, // byte 13
	}

	parsed, err := driver.ParseRecord(record)
	if err != nil {
		t.Fatalf("ParseRecord error: %v", err)
	}

	// Verify the basic values we can directly set in bytes
	if parsed.Diastolic != 80 {
		t.Errorf("Diastolic = %d, want 80", parsed.Diastolic)
	}
	if parsed.Systolic != 120 {
		t.Errorf("Systolic = %d, want 120 (95 + 25)", parsed.Systolic)
	}
	if parsed.Pulse != 72 {
		t.Errorf("Pulse = %d, want 72", parsed.Pulse)
	}
	if parsed.Timestamp.Year() != 2024 {
		t.Errorf("Year = %d, want 2024", parsed.Timestamp.Year())
	}
}

func TestHEM7155T_Config(t *testing.T) {
	driver := NewHEM7155T()
	config := driver.GetConfig()

	if config.Name != "HEM-7155T" {
		t.Errorf("Name = %s, want HEM-7155T", config.Name)
	}
	if config.Endianness != device.LittleEndian {
		t.Errorf("Endianness = %s, want little", config.Endianness)
	}
	if config.RecordByteSize != 0x10 {
		t.Errorf("RecordByteSize = %d, want 16", config.RecordByteSize)
	}
}

func TestHEM7600T_SingleUser(t *testing.T) {
	driver := NewHEM7600T()
	config := driver.GetConfig()

	if len(config.UserStartAddresses) != 1 {
		t.Errorf("UserStartAddresses count = %d, want 1 (single user)", len(config.UserStartAddresses))
	}
	if len(config.RecordsPerUser) != 1 {
		t.Errorf("RecordsPerUser count = %d, want 1", len(config.RecordsPerUser))
	}
}

func TestHEM7530T_NoTimeSync(t *testing.T) {
	driver := NewHEM7530T()
	config := driver.GetConfig()

	if config.SupportsTimeSync {
		t.Error("SupportsTimeSync = true, want false for HEM-7530T")
	}
	if config.SupportsUnreadCounter {
		t.Error("SupportsUnreadCounter = true, want false for HEM-7530T")
	}

	// Try to build time sync bytes - should fail
	_, err := driver.BuildTimeSyncBytes(make([]byte, 100), time.Now())
	if err == nil {
		t.Error("BuildTimeSyncBytes should return error for unsupported device")
	}
}

func TestHEM6232T_NoTimeSync(t *testing.T) {
	driver := NewHEM6232T()

	_, err := driver.BuildTimeSyncBytes(make([]byte, 100), time.Now())
	if err == nil {
		t.Error("BuildTimeSyncBytes should return error for HEM-6232T")
	}
}

func TestTimeSyncBytes_BigEndian(t *testing.T) {
	driver := NewHEM7322T()
	config := driver.GetConfig()

	// Create mock cached settings
	settingsSize := config.TimeSyncRange[1]
	cachedSettings := make([]byte, settingsSize+10) // Extra padding

	// Set some initial values in the time sync range
	for i := config.TimeSyncRange[0]; i < config.TimeSyncRange[1]; i++ {
		cachedSettings[i] = byte(i)
	}

	testTime := time.Date(2024, 6, 15, 14, 30, 45, 0, time.Local)

	result, err := driver.BuildTimeSyncBytes(cachedSettings, testTime)
	if err != nil {
		t.Fatalf("BuildTimeSyncBytes error: %v", err)
	}

	// Verify the time sync bytes structure (for big-endian devices)
	// Format: [original 0:2], month, year-2000, hour, day, second, minute, 0x00, checksum
	if result[2] != byte(testTime.Month()) {
		t.Errorf("Month = %d, want %d", result[2], testTime.Month())
	}
	if result[3] != byte(testTime.Year()-2000) {
		t.Errorf("Year-2000 = %d, want %d", result[3], testTime.Year()-2000)
	}
	if result[4] != byte(testTime.Hour()) {
		t.Errorf("Hour = %d, want %d", result[4], testTime.Hour())
	}
	if result[5] != byte(testTime.Day()) {
		t.Errorf("Day = %d, want %d", result[5], testTime.Day())
	}
	if result[6] != byte(testTime.Second()) {
		t.Errorf("Second = %d, want %d", result[6], testTime.Second())
	}
	if result[7] != byte(testTime.Minute()) {
		t.Errorf("Minute = %d, want %d", result[7], testTime.Minute())
	}
}

func TestTimeSyncBytes_LittleEndian(t *testing.T) {
	driver := NewHEM7155T()
	config := driver.GetConfig()

	// Create mock cached settings
	settingsSize := config.TimeSyncRange[1]
	cachedSettings := make([]byte, settingsSize+10)

	testTime := time.Date(2024, 6, 15, 14, 30, 45, 0, time.Local)

	result, err := driver.BuildTimeSyncBytes(cachedSettings, testTime)
	if err != nil {
		t.Fatalf("BuildTimeSyncBytes error: %v", err)
	}

	// Verify the time sync bytes structure (for little-endian devices)
	// Format: [original 0:8], year-2000, month, day, hour, minute, second, checksum, 0x00
	if result[8] != byte(testTime.Year()-2000) {
		t.Errorf("Year-2000 = %d, want %d", result[8], testTime.Year()-2000)
	}
	if result[9] != byte(testTime.Month()) {
		t.Errorf("Month = %d, want %d", result[9], testTime.Month())
	}
	if result[10] != byte(testTime.Day()) {
		t.Errorf("Day = %d, want %d", result[10], testTime.Day())
	}
	if result[11] != byte(testTime.Hour()) {
		t.Errorf("Hour = %d, want %d", result[11], testTime.Hour())
	}
	if result[12] != byte(testTime.Minute()) {
		t.Errorf("Minute = %d, want %d", result[12], testTime.Minute())
	}
	if result[13] != byte(testTime.Second()) {
		t.Errorf("Second = %d, want %d", result[13], testTime.Second())
	}
}

func TestAllDevicesHaveValidConfigs(t *testing.T) {
	devices := []device.Driver{
		NewHEM7322T(),
		NewHEM7600T(),
		NewHEM6232T(),
		NewHEM7530T(),
		NewHEM7155T(),
		NewHEM7150T(),
		NewHEM7342T(),
		NewHEM7361T(),
	}

	for _, driver := range devices {
		config := driver.GetConfig()
		t.Run(config.Name, func(t *testing.T) {
			// All devices should have a name
			if config.Name == "" {
				t.Error("Device name is empty")
			}

			// Should have at least one user
			if len(config.UserStartAddresses) == 0 {
				t.Error("No user start addresses defined")
			}

			// User addresses and counts should match
			if len(config.UserStartAddresses) != len(config.RecordsPerUser) {
				t.Errorf("UserStartAddresses (%d) and RecordsPerUser (%d) length mismatch",
					len(config.UserStartAddresses), len(config.RecordsPerUser))
			}

			// Record size should be reasonable
			if config.RecordByteSize < 8 || config.RecordByteSize > 64 {
				t.Errorf("RecordByteSize = %d, seems unreasonable", config.RecordByteSize)
			}

			// Block size should be > 0
			if config.TransmissionBlockSize <= 0 {
				t.Errorf("TransmissionBlockSize = %d, should be > 0", config.TransmissionBlockSize)
			}

			// Endianness should be valid
			if config.Endianness != device.BigEndian && config.Endianness != device.LittleEndian {
				t.Errorf("Invalid Endianness: %s", config.Endianness)
			}
		})
	}
}

func TestParseRecord_TooShort(t *testing.T) {
	driver := NewHEM7322T()

	_, err := driver.ParseRecord([]byte{0x01, 0x02, 0x03}) // Too short
	if err == nil {
		t.Error("ParseRecord should error on too-short data")
	}
}
