package ble

import "errors"

var (
	ErrPacketTooShort       = errors.New("packet too short")
	ErrChecksumMismatch     = errors.New("checksum mismatch")
	ErrTransmissionFailed   = errors.New("transmission failed after max retries")
	ErrUnexpectedResponse   = errors.New("unexpected response type")
	ErrServiceNotFound      = errors.New("omron service not found on device")
	ErrCharacteristicNotFound = errors.New("characteristic not found")
	ErrPairingModeFailed    = errors.New("could not enter pairing mode - is device in -P- mode?")
	ErrPairingKeyFailed     = errors.New("failed to program pairing key")
	ErrUnlockFailed         = errors.New("pairing key does not match stored key")
	ErrNotConnected         = errors.New("not connected to device")
	ErrTimeout              = errors.New("operation timed out")
)
