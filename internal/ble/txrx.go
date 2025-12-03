package ble

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	maxRetries    = 5
	defaultTimeout = 1 * time.Second
	channelSize   = 16 // Each BLE channel carries 16 bytes max
)

// TxRxHandler manages multi-channel BLE communication with Omron devices
type TxRxHandler struct {
	client          *Client
	rxBuffer        [4][]byte
	rxMutex         sync.Mutex
	rxComplete      chan *RxPacket
	unlockResponse  chan []byte
	notifyEnabled   bool
	debug           bool
}

// NewTxRxHandler creates a new TX/RX handler
func NewTxRxHandler(client *Client, debug bool) *TxRxHandler {
	return &TxRxHandler{
		client:         client,
		rxComplete:     make(chan *RxPacket, 1),
		unlockResponse: make(chan []byte, 1),
		debug:          debug,
	}
}

// EnableNotifications starts listening on all RX channels
func (h *TxRxHandler) EnableNotifications() error {
	if h.notifyEnabled {
		return nil
	}

	for i, uuid := range RxChannelUUIDs {
		channelIdx := i
		if err := h.client.EnableNotifications(uuid, func(data []byte) {
			h.handleRxData(channelIdx, data)
		}); err != nil {
			return fmt.Errorf("failed to enable notifications on channel %d: %w", i, err)
		}
	}

	h.notifyEnabled = true
	return nil
}

// handleRxData processes data from a single RX channel
func (h *TxRxHandler) handleRxData(channelIdx int, data []byte) {
	h.rxMutex.Lock()
	defer h.rxMutex.Unlock()

	if h.debug {
		log.Printf("RX ch%d < %x", channelIdx, data)
	}

	h.rxBuffer[channelIdx] = data

	// Check if channel 0 has data (contains packet size in first byte)
	if h.rxBuffer[0] == nil {
		return
	}

	packetSize := int(h.rxBuffer[0][0])
	requiredChannels := (packetSize + 15) / 16

	// Check if all required channels received
	for i := 0; i < requiredChannels; i++ {
		if h.rxBuffer[i] == nil {
			return
		}
	}

	// Reassemble packet from all channels
	combined := make([]byte, 0, packetSize)
	for i := 0; i < requiredChannels; i++ {
		combined = append(combined, h.rxBuffer[i]...)
	}
	combined = combined[:packetSize] // Trim to actual packet size

	// Parse the packet
	packet, err := ParseRxPacket(combined)
	if err != nil {
		if h.debug {
			log.Printf("Failed to parse packet: %v", err)
		}
		// Clear buffers and continue
		for i := range h.rxBuffer {
			h.rxBuffer[i] = nil
		}
		return
	}

	// Clear buffers
	for i := range h.rxBuffer {
		h.rxBuffer[i] = nil
	}

	// Send to completion channel (non-blocking)
	select {
	case h.rxComplete <- packet:
	default:
		// Channel full, discard old packet
		select {
		case <-h.rxComplete:
		default:
		}
		h.rxComplete <- packet
	}
}

// handleUnlockData processes data from the unlock channel
func (h *TxRxHandler) handleUnlockData(data []byte) {
	if h.debug {
		log.Printf("Unlock RX < %x", data)
	}

	select {
	case h.unlockResponse <- data:
	default:
		// Channel full, discard old
		select {
		case <-h.unlockResponse:
		default:
		}
		h.unlockResponse <- data
	}
}

// sendCommand sends a command across the required TX channels
func (h *TxRxHandler) sendCommand(command []byte) error {
	requiredChannels := (len(command) + channelSize - 1) / channelSize

	for i := 0; i < requiredChannels; i++ {
		start := i * channelSize
		end := start + channelSize
		if end > len(command) {
			end = len(command)
		}
		chunk := command[start:end]

		if h.debug {
			log.Printf("TX ch%d > %x", i, chunk)
		}

		if err := h.client.WriteCharacteristic(TxChannelUUIDs[i], chunk); err != nil {
			return fmt.Errorf("failed to write to channel %d: %w", i, err)
		}
	}
	return nil
}

// SendAndWait sends a command and waits for response with retries
func (h *TxRxHandler) SendAndWait(ctx context.Context, command []byte, timeout time.Duration) (*RxPacket, error) {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	// Drain any pending responses
	select {
	case <-h.rxComplete:
	default:
	}

	for retry := 0; retry < maxRetries; retry++ {
		if err := h.sendCommand(command); err != nil {
			return nil, err
		}

		select {
		case packet := <-h.rxComplete:
			return packet, nil
		case <-time.After(timeout):
			if h.debug {
				log.Printf("Timeout, retry %d/%d", retry+1, maxRetries)
			}
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, ErrTransmissionFailed
}

// StartTransmission initiates data transmission mode
func (h *TxRxHandler) StartTransmission(ctx context.Context) error {
	if err := h.EnableNotifications(); err != nil {
		return err
	}

	cmd := BuildStartCommand()
	resp, err := h.SendAndWait(ctx, cmd, defaultTimeout)
	if err != nil {
		return fmt.Errorf("start transmission failed: %w", err)
	}

	if resp.PacketType != RespStartTransmission {
		return fmt.Errorf("%w: expected 0x%04x, got 0x%04x",
			ErrUnexpectedResponse, RespStartTransmission, resp.PacketType)
	}

	return nil
}

// EndTransmission ends data transmission mode
func (h *TxRxHandler) EndTransmission(ctx context.Context) error {
	cmd := BuildEndCommand()
	resp, err := h.SendAndWait(ctx, cmd, defaultTimeout)
	if err != nil {
		return fmt.Errorf("end transmission failed: %w", err)
	}

	if resp.PacketType != RespEndTransmission {
		return fmt.Errorf("%w: expected 0x%04x, got 0x%04x",
			ErrUnexpectedResponse, RespEndTransmission, resp.PacketType)
	}

	// Check for error status in response
	if len(resp.Data) > 0 && resp.Data[0] != 0 {
		return fmt.Errorf("device reported error status: %d", resp.Data[0])
	}

	return nil
}

// ReadEEPROM reads continuous data from device EEPROM
func (h *TxRxHandler) ReadEEPROM(ctx context.Context, address uint16, size int, blockSize int) ([]byte, error) {
	if blockSize <= 0 {
		blockSize = 0x10 // Default block size
	}

	result := make([]byte, 0, size)

	for remaining := size; remaining > 0; {
		readSize := blockSize
		if remaining < blockSize {
			readSize = remaining
		}

		cmd := BuildReadCommand(address, uint8(readSize))
		resp, err := h.SendAndWait(ctx, cmd, defaultTimeout)
		if err != nil {
			return nil, fmt.Errorf("read at 0x%04x failed: %w", address, err)
		}

		if resp.PacketType != RespReadEEPROM {
			return nil, fmt.Errorf("%w for read at 0x%04x", ErrUnexpectedResponse, address)
		}

		if resp.Address != address {
			return nil, fmt.Errorf("address mismatch: expected 0x%04x, got 0x%04x", address, resp.Address)
		}

		result = append(result, resp.Data...)
		address += uint16(readSize)
		remaining -= readSize
	}

	return result, nil
}

// WriteEEPROM writes data to device EEPROM
func (h *TxRxHandler) WriteEEPROM(ctx context.Context, address uint16, data []byte, blockSize int) error {
	if blockSize <= 0 {
		blockSize = 0x08 // Default write block size
	}

	for len(data) > 0 {
		writeSize := blockSize
		if len(data) < blockSize {
			writeSize = len(data)
		}

		cmd := BuildWriteCommand(address, data[:writeSize])
		resp, err := h.SendAndWait(ctx, cmd, defaultTimeout)
		if err != nil {
			return fmt.Errorf("write at 0x%04x failed: %w", address, err)
		}

		if resp.PacketType != RespWriteEEPROM {
			return fmt.Errorf("%w for write at 0x%04x", ErrUnexpectedResponse, address)
		}

		if resp.Address != address {
			return fmt.Errorf("address mismatch: expected 0x%04x, got 0x%04x", address, resp.Address)
		}

		data = data[writeSize:]
		address += uint16(writeSize)
	}

	return nil
}

// WriteNewUnlockKey programs a new pairing key into the device
// Device must be in pairing mode (showing -P- on display)
func (h *TxRxHandler) WriteNewUnlockKey(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		key = DefaultPairingKey
	}
	if len(key) != 16 {
		return fmt.Errorf("key must be 16 bytes, got %d", len(key))
	}

	// Enable notifications on unlock channel
	if err := h.client.EnableNotifications(UnlockChannelUUID, h.handleUnlockData); err != nil {
		return fmt.Errorf("failed to enable unlock notifications: %w", err)
	}

	// Drain any pending unlock responses
	select {
	case <-h.unlockResponse:
	default:
	}

	// Enter key programming mode: 0x02 + 16 zero bytes
	enterProgramMode := make([]byte, 17)
	enterProgramMode[0] = 0x02

	if h.debug {
		log.Printf("Entering programming mode...")
	}

	if err := h.client.WriteCharacteristicWithResponse(UnlockChannelUUID, enterProgramMode); err != nil {
		return fmt.Errorf("failed to enter programming mode: %w", err)
	}

	// Wait for response
	select {
	case resp := <-h.unlockResponse:
		if len(resp) < 2 || resp[0] != 0x82 || resp[1] != 0x00 {
			return fmt.Errorf("%w: got response %x", ErrPairingModeFailed, resp)
		}
	case <-time.After(5 * time.Second):
		return ErrTimeout
	case <-ctx.Done():
		return ctx.Err()
	}

	// Program the key: 0x00 + 16-byte key
	programKey := make([]byte, 17)
	programKey[0] = 0x00
	copy(programKey[1:], key)

	if h.debug {
		log.Printf("Programming key...")
	}

	if err := h.client.WriteCharacteristicWithResponse(UnlockChannelUUID, programKey); err != nil {
		return fmt.Errorf("failed to program key: %w", err)
	}

	// Wait for response
	select {
	case resp := <-h.unlockResponse:
		if len(resp) < 2 || resp[0] != 0x80 || resp[1] != 0x00 {
			return fmt.Errorf("%w: got response %x", ErrPairingKeyFailed, resp)
		}
	case <-time.After(5 * time.Second):
		return ErrTimeout
	case <-ctx.Done():
		return ctx.Err()
	}

	if h.debug {
		log.Printf("Key programmed successfully")
	}

	return nil
}

// UnlockWithKey authenticates with the device using a previously programmed key
func (h *TxRxHandler) UnlockWithKey(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		key = DefaultPairingKey
	}
	if len(key) != 16 {
		return fmt.Errorf("key must be 16 bytes, got %d", len(key))
	}

	// Enable notifications on unlock channel
	if err := h.client.EnableNotifications(UnlockChannelUUID, h.handleUnlockData); err != nil {
		return fmt.Errorf("failed to enable unlock notifications: %w", err)
	}

	// Drain any pending unlock responses
	select {
	case <-h.unlockResponse:
	default:
	}

	// Unlock: 0x01 + 16-byte key
	unlockCmd := make([]byte, 17)
	unlockCmd[0] = 0x01
	copy(unlockCmd[1:], key)

	if h.debug {
		log.Printf("Unlocking device...")
	}

	if err := h.client.WriteCharacteristicWithResponse(UnlockChannelUUID, unlockCmd); err != nil {
		return fmt.Errorf("failed to send unlock command: %w", err)
	}

	// Wait for response
	select {
	case resp := <-h.unlockResponse:
		if len(resp) < 2 || resp[0] != 0x81 || resp[1] != 0x00 {
			return ErrUnlockFailed
		}
	case <-time.After(5 * time.Second):
		return ErrTimeout
	case <-ctx.Done():
		return ctx.Err()
	}

	if h.debug {
		log.Printf("Device unlocked successfully")
	}

	return nil
}
