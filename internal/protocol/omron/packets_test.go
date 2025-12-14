package omron_test

import (
	"testing"

	"omblego/internal/protocol/omron"
)

func TestCalculateXORChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected byte
	}{
		{
			name:     "empty",
			data:     []byte{},
			expected: 0x00,
		},
		{
			name:     "single byte",
			data:     []byte{0x42},
			expected: 0x42,
		},
		{
			name:     "two identical bytes",
			data:     []byte{0xFF, 0xFF},
			expected: 0x00,
		},
		{
			name:     "start command",
			data:     []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00},
			expected: 0x18,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := omron.CalculateXORChecksum(tt.data)
			if result != tt.expected {
				t.Errorf("Expected 0x%02x, got 0x%02x", tt.expected, result)
			}
		})
	}
}

func TestBuildStartCommand(t *testing.T) {
	cmd := omron.BuildStartCommand()

	expected := []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x18}
	if len(cmd) != len(expected) {
		t.Fatalf("Expected length %d, got %d", len(expected), len(cmd))
	}

	for i, b := range expected {
		if cmd[i] != b {
			t.Errorf("Byte %d: expected 0x%02x, got 0x%02x", i, b, cmd[i])
		}
	}

	// Verify checksum
	if omron.CalculateXORChecksum(cmd) != 0 {
		t.Error("Invalid checksum in start command")
	}
}

func TestBuildEndCommand(t *testing.T) {
	cmd := omron.BuildEndCommand()

	expected := []byte{0x08, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07}
	if len(cmd) != len(expected) {
		t.Fatalf("Expected length %d, got %d", len(expected), len(cmd))
	}

	for i, b := range expected {
		if cmd[i] != b {
			t.Errorf("Byte %d: expected 0x%02x, got 0x%02x", i, b, cmd[i])
		}
	}

	// Verify checksum
	if omron.CalculateXORChecksum(cmd) != 0 {
		t.Error("Invalid checksum in end command")
	}
}

func TestBuildReadCommand(t *testing.T) {
	tests := []struct {
		address  uint16
		size     uint8
		expected []byte
	}{
		{
			address:  0x0260,
			size:     0x38,
			expected: []byte{0x08, 0x01, 0x00, 0x02, 0x60, 0x38, 0x00, 0x53},
		},
		{
			address:  0x02ac,
			size:     0x38,
			expected: []byte{0x08, 0x01, 0x00, 0x02, 0xac, 0x38, 0x00, 0x1f},
		},
	}

	for _, tt := range tests {
		cmd := omron.BuildReadCommand(tt.address, tt.size)

		if len(cmd) != len(tt.expected) {
			t.Fatalf("Address 0x%04x: expected length %d, got %d", tt.address, len(tt.expected), len(cmd))
		}

		// Verify checksum
		if omron.CalculateXORChecksum(cmd) != 0 {
			t.Errorf("Address 0x%04x: invalid checksum", tt.address)
		}
	}
}

func TestBuildWriteCommand(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	cmd := omron.BuildWriteCommand(0x0286, data)

	// Expected structure: length(1) + cmd(2) + addr(2) + size(1) + data(4) + padding(1) + checksum(1)
	expectedLen := 12
	if len(cmd) != expectedLen {
		t.Fatalf("Expected length %d, got %d", expectedLen, len(cmd))
	}

	// Verify length byte
	if cmd[0] != byte(expectedLen) {
		t.Errorf("Length byte: expected %d, got %d", expectedLen, cmd[0])
	}

	// Verify checksum
	if omron.CalculateXORChecksum(cmd) != 0 {
		t.Error("Invalid checksum in write command")
	}
}

func TestParseRxPacket(t *testing.T) {
	tests := []struct {
		name        string
		raw         []byte
		expectedErr error
		checkType   omron.ResponseType
	}{
		{
			name:        "too short",
			raw:         []byte{0x01, 0x02, 0x03},
			expectedErr: omron.ErrPacketTooShort,
		},
		{
			name:        "bad checksum",
			raw:         []byte{0x08, 0x81, 0x00, 0x02, 0x60, 0x00, 0x00, 0x00},
			expectedErr: omron.ErrChecksumMismatch,
		},
		{
			name: "valid read response",
			// Checksum: 0x08 ^ 0x81 ^ 0x00 ^ 0x02 ^ 0x60 ^ 0x00 ^ 0x00 = 0xeb
			raw:         []byte{0x08, 0x81, 0x00, 0x02, 0x60, 0x00, 0x00, 0xeb},
			expectedErr: nil,
			checkType:   omron.RespReadEEPROM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt, err := omron.ParseRxPacket(tt.raw)

			if tt.expectedErr != nil {
				if err != tt.expectedErr {
					t.Errorf("Expected error %v, got %v", tt.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if pkt.PacketType != tt.checkType {
				t.Errorf("Expected type 0x%04x, got 0x%04x", tt.checkType, pkt.PacketType)
			}
		})
	}
}

func TestRxPacketHelpers(t *testing.T) {
	readPkt := &omron.RxPacket{PacketType: omron.RespReadEEPROM}
	writePkt := &omron.RxPacket{PacketType: omron.RespWriteEEPROM}
	startPkt := &omron.RxPacket{PacketType: omron.RespStartTransmission}
	endPkt := &omron.RxPacket{PacketType: omron.RespEndTransmission}

	if !readPkt.IsReadResponse() {
		t.Error("IsReadResponse should return true")
	}
	if !writePkt.IsWriteResponse() {
		t.Error("IsWriteResponse should return true")
	}
	if !startPkt.IsStartResponse() {
		t.Error("IsStartResponse should return true")
	}
	if !endPkt.IsEndResponse() {
		t.Error("IsEndResponse should return true")
	}
}

func TestConstants(t *testing.T) {
	// Verify UUIDs are properly defined
	if len(omron.RxChannelUUIDs) != 4 {
		t.Errorf("Expected 4 RX channels, got %d", len(omron.RxChannelUUIDs))
	}
	if len(omron.TxChannelUUIDs) != 4 {
		t.Errorf("Expected 4 TX channels, got %d", len(omron.TxChannelUUIDs))
	}

	// Verify pairing key length
	if len(omron.DefaultPairingKey) != 16 {
		t.Errorf("Expected 16-byte pairing key, got %d bytes", len(omron.DefaultPairingKey))
	}

	// Verify channel size constant
	if omron.ChannelSize != 16 {
		t.Errorf("Expected channel size 16, got %d", omron.ChannelSize)
	}
}
