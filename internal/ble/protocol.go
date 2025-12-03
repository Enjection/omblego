package ble

import "encoding/binary"

// CommandType represents packet command types sent to device
type CommandType uint16

const (
	CmdStartTransmission CommandType = 0x0000
	CmdReadEEPROM        CommandType = 0x0100
	CmdWriteEEPROM       CommandType = 0x01C0
	CmdEndTransmission   CommandType = 0x0F00
)

// ResponseType represents packet response types from device
type ResponseType uint16

const (
	RespStartTransmission ResponseType = 0x8000
	RespReadEEPROM        ResponseType = 0x8100
	RespWriteEEPROM       ResponseType = 0x81C0
	RespEndTransmission   ResponseType = 0x8F00
)

// GATT Service and Characteristic UUIDs for Omron devices
const (
	ParentServiceUUID = "ecbe3980-c9a2-11e1-b1bd-0002a5d5c51b"
	UnlockChannelUUID = "b305b680-aee7-11e1-a730-0002a5d5c51b"
)

// RxChannelUUIDs are the 4 receive (notification) channels
var RxChannelUUIDs = []string{
	"49123040-aee8-11e1-a74d-0002a5d5c51b",
	"4d0bf320-aee8-11e1-a0d9-0002a5d5c51b",
	"5128ce60-aee8-11e1-b84b-0002a5d5c51b",
	"560f1420-aee8-11e1-8184-0002a5d5c51b",
}

// TxChannelUUIDs are the 4 transmit (write) channels
var TxChannelUUIDs = []string{
	"db5b55e0-aee7-11e1-965e-0002a5d5c51b",
	"e0b8a060-aee7-11e1-92f4-0002a5d5c51b",
	"0ae12b00-aee8-11e1-a192-0002a5d5c51b",
	"10e1ba60-aee8-11e1-89e5-0002a5d5c51b",
}

// DefaultPairingKey is an arbitrary 16-byte key used for device authentication
var DefaultPairingKey = []byte{
	0xde, 0xad, 0xbe, 0xaf, 0x12, 0x34, 0x12, 0x34,
	0xde, 0xad, 0xbe, 0xaf, 0x12, 0x34, 0x12, 0x34,
}

// CalculateXORChecksum calculates XOR checksum for a packet
func CalculateXORChecksum(data []byte) byte {
	var xor byte
	for _, b := range data {
		xor ^= b
	}
	return xor
}

// BuildStartCommand creates a start transmission command packet
func BuildStartCommand() []byte {
	// 0x08 0x00 0x00 0x00 0x00 0x10 0x00 0x18
	packet := []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x18}
	return packet
}

// BuildEndCommand creates an end transmission command packet
func BuildEndCommand() []byte {
	// 0x08 0x0f 0x00 0x00 0x00 0x00 0x00 0x07
	packet := []byte{0x08, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07}
	return packet
}

// BuildReadCommand creates an EEPROM read command packet
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

// BuildWriteCommand creates an EEPROM write command packet
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

// RxPacket represents a received packet after reassembly from multiple channels
type RxPacket struct {
	PacketType ResponseType
	Address    uint16
	Data       []byte
}

// ParseRxPacket parses a reassembled packet from raw bytes
func ParseRxPacket(raw []byte) (*RxPacket, error) {
	if len(raw) < 8 {
		return nil, ErrPacketTooShort
	}

	// Verify checksum
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
		// Data length exceeds packet - fill with 0xFF (as Python does)
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
