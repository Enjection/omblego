package device

import (
	"testing"
)

func TestExtractBits_BigEndian(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		firstBit int
		lastBit  int
		expected int
	}{
		{
			name:     "single byte, all bits",
			data:     []byte{0xFF},
			firstBit: 0,
			lastBit:  7,
			expected: 255,
		},
		{
			name:     "single byte, lower nibble",
			data:     []byte{0xAB},
			firstBit: 4,
			lastBit:  7,
			expected: 0x0B,
		},
		{
			name:     "single byte, upper nibble",
			data:     []byte{0xAB},
			firstBit: 0,
			lastBit:  3,
			expected: 0x0A,
		},
		{
			name:     "two bytes, first byte",
			data:     []byte{0x12, 0x34},
			firstBit: 0,
			lastBit:  7,
			expected: 0x12,
		},
		{
			name:     "two bytes, second byte",
			data:     []byte{0x12, 0x34},
			firstBit: 8,
			lastBit:  15,
			expected: 0x34,
		},
		{
			name:     "two bytes, cross boundary",
			data:     []byte{0xAB, 0xCD},
			firstBit: 4,
			lastBit:  11,
			expected: 0xBC,
		},
		{
			name:     "single bit set",
			data:     []byte{0x80},
			firstBit: 0,
			lastBit:  0,
			expected: 1,
		},
		{
			name:     "single bit unset",
			data:     []byte{0x7F},
			firstBit: 0,
			lastBit:  0,
			expected: 0,
		},
		// Real-world examples from HEM-7322T record format
		{
			name:     "DIA bits 0-7 from record",
			data:     []byte{0x50, 0x5F}, // DIA=80, SYS-25=95
			firstBit: 0,
			lastBit:  7,
			expected: 0x50, // 80
		},
		{
			name:     "SYS bits 8-15 from record",
			data:     []byte{0x50, 0x5F}, // DIA=80, SYS-25=95
			firstBit: 8,
			lastBit:  15,
			expected: 0x5F, // 95 (actual SYS = 95+25 = 120)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractBits(tt.data, BigEndian, tt.firstBit, tt.lastBit)
			if result != tt.expected {
				t.Errorf("ExtractBits(%x, BigEndian, %d, %d) = %d (0x%x), want %d (0x%x)",
					tt.data, tt.firstBit, tt.lastBit, result, result, tt.expected, tt.expected)
			}
		})
	}
}

func TestExtractBits_LittleEndian(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		firstBit int
		lastBit  int
		expected int
	}{
		{
			name:     "two bytes LE, full value",
			data:     []byte{0x34, 0x12}, // 0x1234 in little-endian
			firstBit: 0,
			lastBit:  15,
			expected: 0x1234,
		},
		{
			name:     "two bytes LE, lower byte",
			data:     []byte{0x34, 0x12},
			firstBit: 8,
			lastBit:  15,
			expected: 0x34,
		},
		{
			name:     "two bytes LE, upper byte",
			data:     []byte{0x34, 0x12},
			firstBit: 0,
			lastBit:  7,
			expected: 0x12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractBits(tt.data, LittleEndian, tt.firstBit, tt.lastBit)
			if result != tt.expected {
				t.Errorf("ExtractBits(%x, LittleEndian, %d, %d) = %d (0x%x), want %d (0x%x)",
					tt.data, tt.firstBit, tt.lastBit, result, result, tt.expected, tt.expected)
			}
		})
	}
}

func TestExtractBits_SingleBits(t *testing.T) {
	// Test extracting MOV and IHB flags (single bit extractions)
	data := []byte{0x00, 0x00, 0x00, 0x00, 0x03} // bits 32 and 33 are set

	// In big endian, bit 32 is the MSB of byte index 4
	mov := ExtractBits(data, BigEndian, 32, 32)
	ihb := ExtractBits(data, BigEndian, 33, 33)

	if mov != 0 {
		t.Errorf("MOV bit extraction = %d, want 0", mov)
	}
	if ihb != 0 {
		t.Errorf("IHB bit extraction = %d, want 0", ihb)
	}

	// Set both flags
	data2 := []byte{0x00, 0x00, 0x00, 0x00, 0xC0} // bits 32 and 33 set (0b11000000)
	mov2 := ExtractBits(data2, BigEndian, 32, 32)
	ihb2 := ExtractBits(data2, BigEndian, 33, 33)

	if mov2 != 1 {
		t.Errorf("MOV bit extraction with flag set = %d, want 1", mov2)
	}
	if ihb2 != 1 {
		t.Errorf("IHB bit extraction with flag set = %d, want 1", ihb2)
	}
}

func TestConfig_Validation(t *testing.T) {
	config := Config{
		Name:                  "TEST-DEVICE",
		Endianness:            BigEndian,
		UserStartAddresses:    []uint16{0x0100, 0x0200},
		RecordsPerUser:        []int{50, 50},
		RecordByteSize:        14,
		TransmissionBlockSize: 56,
		SettingsReadAddress:   0x0050,
		SettingsWriteAddress:  0x0080,
		UnreadRecordsRange:    [2]int{0, 8},
		TimeSyncRange:         [2]int{20, 30},
		SupportsTimeSync:      true,
		SupportsUnreadCounter: true,
	}

	// Verify configuration
	if config.Name != "TEST-DEVICE" {
		t.Errorf("Config.Name = %s, want TEST-DEVICE", config.Name)
	}

	if len(config.UserStartAddresses) != len(config.RecordsPerUser) {
		t.Errorf("UserStartAddresses and RecordsPerUser length mismatch: %d vs %d",
			len(config.UserStartAddresses), len(config.RecordsPerUser))
	}

	if config.SettingsWriteAddress <= config.SettingsReadAddress {
		t.Errorf("SettingsWriteAddress (%x) should be > SettingsReadAddress (%x)",
			config.SettingsWriteAddress, config.SettingsReadAddress)
	}
}

func TestBloodPressureRecord(t *testing.T) {
	record := BloodPressureRecord{
		Systolic:  120,
		Diastolic: 80,
		Pulse:     72,
		Movement:  false,
		IHB:       true,
	}

	if record.Systolic != 120 {
		t.Errorf("Systolic = %d, want 120", record.Systolic)
	}
	if record.Diastolic != 80 {
		t.Errorf("Diastolic = %d, want 80", record.Diastolic)
	}
	if record.Pulse != 72 {
		t.Errorf("Pulse = %d, want 72", record.Pulse)
	}
	if record.Movement != false {
		t.Errorf("Movement = %v, want false", record.Movement)
	}
	if record.IHB != true {
		t.Errorf("IHB = %v, want true", record.IHB)
	}
}

func TestEndianness(t *testing.T) {
	if BigEndian != "big" {
		t.Errorf("BigEndian = %s, want 'big'", BigEndian)
	}
	if LittleEndian != "little" {
		t.Errorf("LittleEndian = %s, want 'little'", LittleEndian)
	}
}
