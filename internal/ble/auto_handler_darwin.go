//go:build darwin

package ble

import (
	"context"
	"fmt"
	"log"
	"time"
)

// AutoTxRxHandler handles Omron protocol communication using AutoClient
type AutoTxRxHandler struct {
	client *AutoClient
	debug  bool
}

// NewAutoTxRxHandler creates a new handler for AutoClient
func NewAutoTxRxHandler(client *AutoClient, debug bool) *AutoTxRxHandler {
	return &AutoTxRxHandler{
		client: client,
		debug:  debug,
	}
}

// UnlockWithKey authenticates with the device using the pairing key
func (h *AutoTxRxHandler) UnlockWithKey(ctx context.Context, key []byte) error {
	if key == nil {
		key = DefaultPairingKey
	}

	// Enable notifications on unlock channel first
	if err := h.client.EnableNotifications("b305b680"); err != nil {
		return fmt.Errorf("failed to enable unlock notifications: %w", err)
	}

	if h.debug {
		log.Println("Unlocking device...")
	}

	// Write key with response
	cmd := append([]byte{0x01}, key...)
	if err := h.client.WriteCharacteristic("b305b680", cmd, true); err != nil {
		return fmt.Errorf("failed to write unlock key: %w", err)
	}

	// Wait for unlock response
	resp, err := h.client.WaitNotification("b305b680", 5*time.Second)
	if err != nil {
		return fmt.Errorf("unlock response timeout: %w", err)
	}

	if h.debug {
		log.Printf("Unlock response: %x", resp)
	}

	// Check response - should start with 0x81 for success
	if len(resp) > 0 && resp[0] != 0x81 {
		return fmt.Errorf("unlock failed with response: %x", resp)
	}

	if h.debug {
		log.Println("Device unlocked successfully")
	}

	return nil
}

// SetupNotifications enables notifications on all RX channels
func (h *AutoTxRxHandler) SetupNotifications() error {
	rxChannels := []string{"49123040", "4d0bf320", "5128ce60", "560f1420"}
	for _, ch := range rxChannels {
		if err := h.client.EnableNotifications(ch); err != nil {
			return fmt.Errorf("failed to enable notifications on %s: %w", ch, err)
		}
	}
	return nil
}

// drainNotifications clears any stale notifications from all RX channels
func (h *AutoTxRxHandler) drainNotifications() {
	rxChannels := []string{"49123040", "4d0bf320", "5128ce60", "560f1420"}
	for _, ch := range rxChannels {
		notifyCh := h.client.getNotifyChannel(ch)
		if notifyCh == nil {
			continue
		}
		// Drain all pending notifications
		for {
			select {
			case <-notifyCh:
			default:
				goto nextChannel
			}
		}
	nextChannel:
	}
}

// StartTransmission initiates the data transfer session
func (h *AutoTxRxHandler) StartTransmission(ctx context.Context) error {
	// Drain any stale notifications before sending command
	h.drainNotifications()

	// Small delay after unlock for device to be ready
	time.Sleep(500 * time.Millisecond)

	cmd := BuildStartCommand()
	if h.debug {
		log.Printf("TX > %x", cmd)
	}

	if err := h.client.WriteCharacteristic("db5b55e0", cmd, false); err != nil {
		return err
	}

	// Wait for response on first RX channel
	resp, err := h.client.WaitNotification("49123040", 5*time.Second)
	if err != nil {
		return fmt.Errorf("start transmission response timeout: %w", err)
	}

	if h.debug {
		log.Printf("RX < %x", resp)
	}

	return nil
}

// EndTransmission closes the data transfer session
func (h *AutoTxRxHandler) EndTransmission(ctx context.Context) error {
	cmd := BuildEndCommand()
	if h.debug {
		log.Printf("TX > %x", cmd)
	}

	if err := h.client.WriteCharacteristic("db5b55e0", cmd, false); err != nil {
		return err
	}

	// Wait for response
	resp, err := h.client.WaitNotification("49123040", 5*time.Second)
	if err != nil {
		if h.debug {
			log.Printf("End transmission response timeout (non-fatal)")
		}
		return nil
	}

	if h.debug {
		log.Printf("RX < %x", resp)
	}

	return nil
}

// collectResponse collects response data from all RX channels with proper buffering
func (h *AutoTxRxHandler) collectResponse(timeout time.Duration) ([]byte, error) {
	rxChannels := []string{"49123040", "4d0bf320", "5128ce60", "560f1420"}
	rxBuffer := make([][]byte, 4)

	// First, drain any stale notifications from all channels
	for i, ch := range rxChannels {
		for {
			select {
			case data := <-h.client.getNotifyChannel(ch):
				// Store in buffer by channel index
				rxBuffer[i] = data
			default:
				goto nextChannel
			}
		}
	nextChannel:
	}

	// Clear buffers for fresh collection
	for i := range rxBuffer {
		rxBuffer[i] = nil
	}

	// Now collect fresh notifications with timeout
	deadline := time.Now().Add(timeout)
	gotChannel0 := false

	for time.Now().Before(deadline) {
		for i, ch := range rxChannels {
			select {
			case data := <-h.client.getNotifyChannel(ch):
				rxBuffer[i] = data
				if h.debug {
					log.Printf("RX ch%d < %x", i, data)
				}
				if i == 0 {
					gotChannel0 = true
				}
			default:
				continue
			}
		}

		// If we have channel 0, check if we have all required channels
		if gotChannel0 && rxBuffer[0] != nil {
			packetSize := int(rxBuffer[0][0])
			requiredChannels := (packetSize + 15) / 16

			allPresent := true
			for i := 0; i < requiredChannels; i++ {
				if rxBuffer[i] == nil {
					allPresent = false
					break
				}
			}

			if allPresent {
				// Assemble packet
				combined := make([]byte, 0, packetSize)
				for i := 0; i < requiredChannels; i++ {
					combined = append(combined, rxBuffer[i]...)
				}
				if len(combined) > packetSize {
					combined = combined[:packetSize]
				}
				return combined, nil
			}
		}

		// Small sleep to avoid busy-waiting
		time.Sleep(10 * time.Millisecond)
	}

	// Timeout - return whatever we have
	if rxBuffer[0] == nil {
		return nil, fmt.Errorf("no response received")
	}

	packetSize := int(rxBuffer[0][0])
	requiredChannels := (packetSize + 15) / 16
	combined := make([]byte, 0, packetSize)
	for i := 0; i < requiredChannels; i++ {
		if rxBuffer[i] != nil {
			combined = append(combined, rxBuffer[i]...)
		}
	}
	if len(combined) > packetSize {
		combined = combined[:packetSize]
	}

	return combined, nil
}

// ReadEEPROM reads data from the device's EEPROM
func (h *AutoTxRxHandler) ReadEEPROM(ctx context.Context, address uint16, size int, blockSize int) ([]byte, error) {
	var result []byte

	for offset := 0; offset < size; offset += blockSize {
		readSize := blockSize
		if offset+blockSize > size {
			readSize = size - offset
		}

		addr := address + uint16(offset)
		cmd := BuildReadCommand(addr, uint8(readSize))

		if h.debug {
			log.Printf("TX > %x", cmd)
		}

		if err := h.client.WriteCharacteristic("db5b55e0", cmd, false); err != nil {
			return nil, err
		}

		// Collect response with proper buffering
		rxData, err := h.collectResponse(3 * time.Second)
		if err != nil {
			return nil, fmt.Errorf("no response at address 0x%04x: %w", addr, err)
		}

		if len(rxData) < 8 {
			return nil, fmt.Errorf("incomplete response at address 0x%04x", addr)
		}

		// Parse response packet
		packet, err := ParseRxPacket(rxData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		result = append(result, packet.Data...)
	}

	return result, nil
}

// sendCommand sends a command, splitting across multiple TX channels if needed
func (h *AutoTxRxHandler) sendCommand(command []byte) error {
	txChannels := []string{"db5b55e0", "e0b8a060", "0ae12b00", "10e1ba60"}
	channelSize := 16

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

		if err := h.client.WriteCharacteristic(txChannels[i], chunk, false); err != nil {
			return fmt.Errorf("failed to write to channel %d: %w", i, err)
		}
	}
	return nil
}

// WriteEEPROM writes data to the device's EEPROM
func (h *AutoTxRxHandler) WriteEEPROM(ctx context.Context, address uint16, data []byte, blockSize int) error {
	for offset := 0; offset < len(data); offset += blockSize {
		writeSize := blockSize
		if offset+blockSize > len(data) {
			writeSize = len(data) - offset
		}

		addr := address + uint16(offset)
		chunk := data[offset : offset+writeSize]
		cmd := BuildWriteCommand(addr, chunk)

		if h.debug {
			log.Printf("TX > %x", cmd)
		}

		// Split command across TX channels if needed
		if err := h.sendCommand(cmd); err != nil {
			return err
		}

		// Wait for acknowledgment
		resp, err := h.client.WaitNotification("49123040", 2*time.Second)
		if err != nil {
			return fmt.Errorf("write response timeout at address 0x%04x: %w", addr, err)
		}

		if h.debug {
			log.Printf("RX < %x", resp)
		}
	}

	return nil
}
