package omron

import (
	"encoding/binary"
	"errors"
)

var (
	// ErrPacketTooShort indicates the packet is too short to parse
	ErrPacketTooShort = errors.New("packet too short")
	// ErrChecksumMismatch indicates the packet checksum is invalid
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

// RxPacket represents a received packet after reassembly from multiple channels
type RxPacket struct {
	PacketType ResponseType
	Address    uint16
	Data       []byte
}

// CalculateXORChecksum calculates XOR checksum for a packet.
// The checksum is the XOR of all bytes in the packet.
func CalculateXORChecksum(data []byte) byte {
	var xor byte
	for _, b := range data {
		xor ^= b
	}
	return xor
}

// BuildStartCommand creates a start transmission command packet.
func BuildStartCommand() []byte {
	// 0x08 0x00 0x00 0x00 0x00 0x10 0x00 0x18
	return []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x18}
}

// BuildEndCommand creates an end transmission command packet.
func BuildEndCommand() []byte {
	// 0x08 0x0f 0x00 0x00 0x00 0x00 0x00 0x07
	return []byte{0x08, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07}
}

// BuildReadCommand creates an EEPROM read command packet.
// address: EEPROM address to read from
// size: number of bytes to read
func BuildReadCommand(address uint16, size uint8) []byte {
	packet := make([]byte, 8)
	packet[0] = 0x08 // Total length
	binary.BigEndian.PutUint16(packet[1:3], uint16(CmdReadEEPROM))
	binary.BigEndian.PutUint16(packet[3:5], address)
	packet[5] = size
	packet[6] = 0x00 // Padding
	packet[7] = CalculateXORChecksum(packet[:7])
	return packet
}

// BuildWriteCommand creates an EEPROM write command packet.
// address: EEPROM address to write to
// data: bytes to write
func BuildWriteCommand(address uint16, data []byte) []byte {
	length := uint8(len(data) + 8)
	packet := make([]byte, length)
	packet[0] = length
	binary.BigEndian.PutUint16(packet[1:3], uint16(CmdWriteEEPROM))
	binary.BigEndian.PutUint16(packet[3:5], address)
	packet[5] = uint8(len(data))
	copy(packet[6:], data)
	packet[length-2] = 0x00
	packet[length-1] = CalculateXORChecksum(packet[:length-1])
	return packet
}

// ParseRxPacket parses a reassembled packet from raw bytes.
// Returns an error if the packet is malformed or the checksum is invalid.
func ParseRxPacket(raw []byte) (*RxPacket, error) {
	if len(raw) < 8 {
		return nil, ErrPacketTooShort
	}

	// Verify checksum (XOR of all bytes should be 0)
	if CalculateXORChecksum(raw) != 0 {
		return nil, ErrChecksumMismatch
	}

	packetType := ResponseType(binary.BigEndian.Uint16(raw[1:3]))
	address := binary.BigEndian.Uint16(raw[3:5])
	dataLen := raw[5]

	var data []byte
	if dataLen > 0 && int(dataLen) <= len(raw)-8 {
		// Special case for end transmission packet - error code is at byte 6
		if packetType == RespEndTransmission {
			data = raw[6:7]
		} else {
			data = raw[6 : 6+dataLen]
		}
	} else if dataLen > 0 {
		// Data length exceeds packet - fill with 0xFF
		data = make([]byte, dataLen)
		for i := range data {
			data[i] = 0xFF
		}
	}

	return &RxPacket{
		PacketType: packetType,
		Address:    address,
		Data:       data,
	}, nil
}

// IsReadResponse returns true if this is a read EEPROM response
func (p *RxPacket) IsReadResponse() bool {
	return p.PacketType == RespReadEEPROM
}

// IsWriteResponse returns true if this is a write EEPROM response
func (p *RxPacket) IsWriteResponse() bool {
	return p.PacketType == RespWriteEEPROM
}

// IsStartResponse returns true if this is a start transmission response
func (p *RxPacket) IsStartResponse() bool {
	return p.PacketType == RespStartTransmission
}

// IsEndResponse returns true if this is an end transmission response
func (p *RxPacket) IsEndResponse() bool {
	return p.PacketType == RespEndTransmission
}
