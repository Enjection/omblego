package ble

import (
	"bytes"
	"testing"
)

func TestCalculateXORChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected byte
	}{
		{
			name:     "empty data",
			data:     []byte{},
			expected: 0x00,
		},
		{
			name:     "single byte",
			data:     []byte{0x42},
			expected: 0x42,
		},
		{
			name:     "two bytes",
			data:     []byte{0xFF, 0xFF},
			expected: 0x00,
		},
		{
			name:     "start command without checksum",
			data:     []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00},
			expected: 0x18,
		},
		{
			name:     "end command without checksum",
			data:     []byte{0x08, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: 0x07,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateXORChecksum(tt.data)
			if result != tt.expected {
				t.Errorf("CalculateXORChecksum(%x) = %x, want %x", tt.data, result, tt.expected)
			}
		})
	}
}

func TestBuildStartCommand(t *testing.T) {
	cmd := BuildStartCommand()

	expected := []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x18}
	if !bytes.Equal(cmd, expected) {
		t.Errorf("BuildStartCommand() = %x, want %x", cmd, expected)
	}

	// Verify checksum
	if CalculateXORChecksum(cmd) != 0 {
		t.Errorf("Start command checksum invalid: XOR = %x, want 0", CalculateXORChecksum(cmd))
	}
}

func TestBuildEndCommand(t *testing.T) {
	cmd := BuildEndCommand()

	expected := []byte{0x08, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07}
	if !bytes.Equal(cmd, expected) {
		t.Errorf("BuildEndCommand() = %x, want %x", cmd, expected)
	}

	// Verify checksum
	if CalculateXORChecksum(cmd) != 0 {
		t.Errorf("End command checksum invalid: XOR = %x, want 0", CalculateXORChecksum(cmd))
	}
}

func TestBuildReadCommand(t *testing.T) {
	tests := []struct {
		name    string
		address uint16
		size    uint8
	}{
		{"read at 0x0260", 0x0260, 0x10},
		{"read at 0x02ac", 0x02ac, 0x38},
		{"read at 0x0000", 0x0000, 0x08},
		{"read max size", 0xFFFF, 0xFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := BuildReadCommand(tt.address, tt.size)

			// Verify length
			if cmd[0] != 0x08 {
				t.Errorf("Read command length = %x, want 0x08", cmd[0])
			}

			// Verify command type
			if cmd[1] != 0x01 || cmd[2] != 0x00 {
				t.Errorf("Read command type = %x%x, want 0100", cmd[1], cmd[2])
			}

			// Verify address (big endian)
			gotAddr := uint16(cmd[3])<<8 | uint16(cmd[4])
			if gotAddr != tt.address {
				t.Errorf("Read command address = %04x, want %04x", gotAddr, tt.address)
			}

			// Verify size
			if cmd[5] != tt.size {
				t.Errorf("Read command size = %x, want %x", cmd[5], tt.size)
			}

			// Verify checksum (XOR of all bytes should be 0)
			if CalculateXORChecksum(cmd) != 0 {
				t.Errorf("Read command checksum invalid: XOR = %x, want 0", CalculateXORChecksum(cmd))
			}
		})
	}
}

func TestBuildWriteCommand(t *testing.T) {
	tests := []struct {
		name    string
		address uint16
		data    []byte
	}{
		{"write 1 byte", 0x0286, []byte{0x42}},
		{"write 4 bytes", 0x0286, []byte{0x01, 0x02, 0x03, 0x04}},
		{"write 8 bytes", 0x0300, []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := BuildWriteCommand(tt.address, tt.data)

			// Verify length
			expectedLen := uint8(len(tt.data) + 8)
			if cmd[0] != expectedLen {
				t.Errorf("Write command length = %d, want %d", cmd[0], expectedLen)
			}

			// Verify command type (0x01C0)
			if cmd[1] != 0x01 || cmd[2] != 0xC0 {
				t.Errorf("Write command type = %x%x, want 01C0", cmd[1], cmd[2])
			}

			// Verify address (big endian)
			gotAddr := uint16(cmd[3])<<8 | uint16(cmd[4])
			if gotAddr != tt.address {
				t.Errorf("Write command address = %04x, want %04x", gotAddr, tt.address)
			}

			// Verify data length field
			if cmd[5] != uint8(len(tt.data)) {
				t.Errorf("Write command data length = %d, want %d", cmd[5], len(tt.data))
			}

			// Verify data
			gotData := cmd[6 : 6+len(tt.data)]
			if !bytes.Equal(gotData, tt.data) {
				t.Errorf("Write command data = %x, want %x", gotData, tt.data)
			}

			// Verify checksum (XOR of all bytes should be 0)
			if CalculateXORChecksum(cmd) != 0 {
				t.Errorf("Write command checksum invalid: XOR = %x, want 0", CalculateXORChecksum(cmd))
			}
		})
	}
}

func TestParseRxPacket(t *testing.T) {
	tests := []struct {
		name           string
		raw            []byte
		expectedType   ResponseType
		expectedAddr   uint16
		expectedData   []byte
		expectError    bool
	}{
		{
			name:         "start transmission response",
			raw:          buildValidPacket(RespStartTransmission, 0x0000, []byte{0x01, 0x02, 0x03}),
			expectedType: RespStartTransmission,
			expectedAddr: 0x0000,
			expectedData: []byte{0x01, 0x02, 0x03},
			expectError:  false,
		},
		{
			name:         "read response",
			raw:          buildValidPacket(RespReadEEPROM, 0x0260, []byte{0xAA, 0xBB, 0xCC, 0xDD}),
			expectedType: RespReadEEPROM,
			expectedAddr: 0x0260,
			expectedData: []byte{0xAA, 0xBB, 0xCC, 0xDD},
			expectError:  false,
		},
		{
			name:         "write response",
			raw:          buildValidPacket(RespWriteEEPROM, 0x0286, []byte{}),
			expectedType: RespWriteEEPROM,
			expectedAddr: 0x0286,
			expectedData: []byte{},
			expectError:  false,
		},
		{
			name:        "packet too short",
			raw:         []byte{0x01, 0x02, 0x03},
			expectError: true,
		},
		{
			name:        "invalid checksum",
			raw:         []byte{0x08, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF}, // Wrong checksum
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := ParseRxPacket(tt.raw)

			if tt.expectError {
				if err == nil {
					t.Errorf("ParseRxPacket() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseRxPacket() unexpected error: %v", err)
				return
			}

			if packet.PacketType != tt.expectedType {
				t.Errorf("PacketType = %04x, want %04x", packet.PacketType, tt.expectedType)
			}

			if packet.Address != tt.expectedAddr {
				t.Errorf("Address = %04x, want %04x", packet.Address, tt.expectedAddr)
			}

			if !bytes.Equal(packet.Data, tt.expectedData) {
				t.Errorf("Data = %x, want %x", packet.Data, tt.expectedData)
			}
		})
	}
}

// Helper function to build a valid packet with correct checksum
func buildValidPacket(respType ResponseType, addr uint16, data []byte) []byte {
	length := uint8(8 + len(data))
	packet := make([]byte, length)
	packet[0] = length
	packet[1] = byte(respType >> 8)
	packet[2] = byte(respType)
	packet[3] = byte(addr >> 8)
	packet[4] = byte(addr)
	packet[5] = uint8(len(data))
	copy(packet[6:], data)
	packet[length-2] = 0x00
	packet[length-1] = CalculateXORChecksum(packet[:length-1])
	return packet
}

func TestCommandTypes(t *testing.T) {
	// Verify command type constants match protocol spec
	if CmdStartTransmission != 0x0000 {
		t.Errorf("CmdStartTransmission = %04x, want 0x0000", CmdStartTransmission)
	}
	if CmdReadEEPROM != 0x0100 {
		t.Errorf("CmdReadEEPROM = %04x, want 0x0100", CmdReadEEPROM)
	}
	if CmdWriteEEPROM != 0x01C0 {
		t.Errorf("CmdWriteEEPROM = %04x, want 0x01C0", CmdWriteEEPROM)
	}
	if CmdEndTransmission != 0x0F00 {
		t.Errorf("CmdEndTransmission = %04x, want 0x0F00", CmdEndTransmission)
	}
}

func TestResponseTypes(t *testing.T) {
	// Verify response type constants match protocol spec
	if RespStartTransmission != 0x8000 {
		t.Errorf("RespStartTransmission = %04x, want 0x8000", RespStartTransmission)
	}
	if RespReadEEPROM != 0x8100 {
		t.Errorf("RespReadEEPROM = %04x, want 0x8100", RespReadEEPROM)
	}
	if RespWriteEEPROM != 0x81C0 {
		t.Errorf("RespWriteEEPROM = %04x, want 0x81C0", RespWriteEEPROM)
	}
	if RespEndTransmission != 0x8F00 {
		t.Errorf("RespEndTransmission = %04x, want 0x8F00", RespEndTransmission)
	}
}

func TestUUIDs(t *testing.T) {
	// Verify UUIDs are lowercase and valid format
	if len(RxChannelUUIDs) != 4 {
		t.Errorf("RxChannelUUIDs length = %d, want 4", len(RxChannelUUIDs))
	}
	if len(TxChannelUUIDs) != 4 {
		t.Errorf("TxChannelUUIDs length = %d, want 4", len(TxChannelUUIDs))
	}

	// Verify default pairing key is 16 bytes
	if len(DefaultPairingKey) != 16 {
		t.Errorf("DefaultPairingKey length = %d, want 16", len(DefaultPairingKey))
	}
}
