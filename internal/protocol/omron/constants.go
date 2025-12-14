// Package omron implements the custom Omron EEPROM-based BLE protocol.
// This protocol is used by most Omron blood pressure monitors.
package omron

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

// GATT Service and Characteristic UUIDs for Omron custom protocol
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

// DefaultPairingKey is an arbitrary 16-byte key used for device authentication.
// Any 16-byte key works; the device stores the first key it receives during pairing.
var DefaultPairingKey = []byte{
	0xde, 0xad, 0xbe, 0xaf, 0x12, 0x34, 0x12, 0x34,
	0xde, 0xad, 0xbe, 0xaf, 0x12, 0x34, 0x12, 0x34,
}

// ChannelSize is the maximum bytes per BLE characteristic channel
const ChannelSize = 16

// MaxRetries is the default number of command retry attempts
const MaxRetries = 5
