package device

import (
	"context"
	"fmt"
	"log"
	"time"

	"omblego/internal/ble"
)

// ProtocolType identifies which BLE protocol the device uses
type ProtocolType string

const (
	// ProtocolOmronCustom uses the custom Omron EEPROM-based protocol
	ProtocolOmronCustom ProtocolType = "omron_custom"
	// ProtocolStandardGATT uses standard BLE GATT services (Blood Pressure Service)
	ProtocolStandardGATT ProtocolType = "standard_gatt"
)

// Endianness represents byte order for bit extraction
type Endianness string

const (
	BigEndian    Endianness = "big"
	LittleEndian Endianness = "little"
)

// Config holds device-specific memory layout configuration
type Config struct {
	Name                  string       // Device model name (e.g., "HEM-7322T")
	Protocol              ProtocolType // Which BLE protocol to use
	BLENamePrefix         string       // BLE local name prefix for device identification (for GATT devices)
	Endianness            Endianness   // Byte order for bit extraction
	UserStartAddresses    []uint16     // EEPROM addresses for each user's records
	RecordsPerUser        []int        // Number of record slots per user
	RecordByteSize        int          // Size of single record in bytes
	TransmissionBlockSize int          // BLE read block size
	SettingsReadAddress   uint16       // Settings region start (for reading)
	SettingsWriteAddress  uint16       // Settings region start (for writing)
	UnreadRecordsRange    [2]int       // [start, end] byte range in settings
	TimeSyncRange         [2]int       // [start, end] byte range in settings
	SupportsTimeSync      bool         // Whether time sync is implemented
	SupportsUnreadCounter bool         // Whether unread counter is implemented
}

// BloodPressureRecord represents a single measurement
type BloodPressureRecord struct {
	Timestamp time.Time
	Systolic  int  // mmHg
	Diastolic int  // mmHg
	Pulse     int  // BPM
	Movement  bool // Movement detected during measurement
	IHB       bool // Irregular heartbeat detected
}

// Driver is the interface all device drivers must implement
type Driver interface {
	// GetConfig returns the device configuration
	GetConfig() Config

	// ParseRecord parses raw bytes into a BloodPressureRecord
	ParseRecord(data []byte) (*BloodPressureRecord, error)

	// BuildTimeSyncBytes creates the bytes to write for time sync (if supported)
	BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error)
}

// ReadOptions configures how records are read
type ReadOptions struct {
	UseUnreadCounter bool // Only read new records
	SyncTime         bool // Sync device time
	Debug            bool // Enable debug output
}

// ExtractBits extracts bits from a byte array (mirrors Python _bytearrayBitsToInt)
// This matches Python's int.from_bytes() behavior:
//   - For big endian: byte 0 is MSB, bit 0 is the MSB of byte 0
//   - For little endian: byte 0 is LSB, but bit 0 is still the MSB of the resulting integer
func ExtractBits(data []byte, endianness Endianness, firstBit, lastBit int) int {
	if len(data) == 0 {
		return 0
	}

	totalBits := len(data) * 8

	// Convert bytes to integer matching Python's int.from_bytes behavior
	// We need to handle arrays larger than 8 bytes by working with the
	// relevant portion only
	var bigInt uint64

	if endianness == BigEndian {
		// Big endian: byte 0 is most significant
		// Only read up to 8 bytes to fit in uint64
		limit := len(data)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			bigInt = (bigInt << 8) | uint64(data[i])
		}
		// Adjust totalBits if we limited the bytes
		if len(data) > 8 {
			totalBits = 64
		}
	} else {
		// Little endian: byte 0 is least significant
		// Read bytes in reverse order to build the integer
		// For little endian, higher indexed bytes are more significant
		limit := len(data)
		if limit > 8 {
			// For little endian, take the LAST 8 bytes (most significant)
			// But actually for bit extraction, we need to be smarter
			// Python uses arbitrary precision, so let's handle this differently
			limit = 8
		}
		for i := limit - 1; i >= 0; i-- {
			bigInt = (bigInt << 8) | uint64(data[i])
		}
		if len(data) > 8 {
			totalBits = 64
		}
	}

	// Now extract bits using the same formula as Python:
	// shiftedBits = bigInt >> (totalBits - (lastBit + 1))
	// bitmask = (1 << numBits) - 1
	// result = shiftedBits & bitmask
	numBits := lastBit - firstBit + 1
	shift := totalBits - (lastBit + 1)
	mask := uint64((1 << numBits) - 1)

	return int((bigInt >> shift) & mask)
}

// GetRecords retrieves all records from the device
func GetRecords(ctx context.Context, handler *ble.TxRxHandler, driver Driver, options ReadOptions) ([][]BloodPressureRecord, error) {
	config := driver.GetConfig()

	// Unlock the device
	if err := handler.UnlockWithKey(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to unlock device: %w", err)
	}

	// Start transmission
	if err := handler.StartTransmission(ctx); err != nil {
		return nil, fmt.Errorf("failed to start transmission: %w", err)
	}

	// Read settings if needed for time sync or unread counter
	var cachedSettings []byte
	if options.SyncTime || options.UseUnreadCounter {
		settingsSize := int(config.SettingsWriteAddress - config.SettingsReadAddress)
		var err error
		cachedSettings, err = handler.ReadEEPROM(ctx, config.SettingsReadAddress, settingsSize, config.TransmissionBlockSize)
		if err != nil {
			handler.EndTransmission(ctx)
			return nil, fmt.Errorf("failed to read settings: %w", err)
		}
	}

	// Determine which records to read
	var allUsersReadCommands [][]readCommand
	if options.UseUnreadCounter && config.SupportsUnreadCounter {
		allUsersReadCommands = getReadCommandsOnlyNew(config, cachedSettings, options.Debug)
	} else {
		allUsersReadCommands = getReadCommandsAll(config)
	}

	// Read records for all users
	if options.Debug {
		log.Println("Starting to read records...")
	}

	allRecords := make([][]BloodPressureRecord, len(config.UserStartAddresses))

	for userIdx, userReadCommands := range allUsersReadCommands {
		var userConcatenatedBytes []byte

		for _, cmd := range userReadCommands {
			data, err := handler.ReadEEPROM(ctx, cmd.Address, cmd.Size, config.TransmissionBlockSize)
			if err != nil {
				handler.EndTransmission(ctx)
				return nil, fmt.Errorf("failed to read records for user %d: %w", userIdx+1, err)
			}
			userConcatenatedBytes = append(userConcatenatedBytes, data...)
		}

		// Parse individual records
		var userRecords []BloodPressureRecord
		for offset := 0; offset < len(userConcatenatedBytes); offset += config.RecordByteSize {
			if offset+config.RecordByteSize > len(userConcatenatedBytes) {
				break
			}

			recordData := userConcatenatedBytes[offset : offset+config.RecordByteSize]

			// Skip empty records (all 0xFF)
			isEmpty := true
			for _, b := range recordData {
				if b != 0xFF {
					isEmpty = false
					break
				}
			}
			if isEmpty {
				continue
			}

			record, err := driver.ParseRecord(recordData)
			if err != nil {
				if options.Debug {
					log.Printf("Warning: failed to parse record for user %d at offset %d: %v", userIdx+1, offset, err)
				}
				continue
			}
			userRecords = append(userRecords, *record)
		}

		allRecords[userIdx] = userRecords
	}

	// Handle time sync if requested
	if options.SyncTime && config.SupportsTimeSync {
		syncBytes, err := driver.BuildTimeSyncBytes(cachedSettings, time.Now())
		if err != nil {
			if options.Debug {
				log.Printf("Warning: failed to build time sync bytes: %v", err)
			}
		} else {
			writeAddr := config.SettingsWriteAddress + uint16(config.TimeSyncRange[0])
			if err := handler.WriteEEPROM(ctx, writeAddr, syncBytes, len(syncBytes)); err != nil {
				if options.Debug {
					log.Printf("Warning: failed to write time sync: %v", err)
				}
			} else if options.Debug {
				log.Println("Time synchronized successfully")
			}
		}
	}

	// Reset unread records counter if used
	if options.UseUnreadCounter && config.SupportsUnreadCounter {
		resetBytes := buildResetUnreadCounterBytes(config, cachedSettings)
		writeAddr := config.SettingsWriteAddress + uint16(config.UnreadRecordsRange[0])
		if err := handler.WriteEEPROM(ctx, writeAddr, resetBytes, len(resetBytes)); err != nil {
			if options.Debug {
				log.Printf("Warning: failed to reset unread counter: %v", err)
			}
		}
	}

	// End transmission
	if err := handler.EndTransmission(ctx); err != nil {
		return nil, fmt.Errorf("failed to end transmission: %w", err)
	}

	return allRecords, nil
}

// readCommand represents a single EEPROM read operation
type readCommand struct {
	Address uint16
	Size    int
}

// getReadCommandsAll returns commands to read all records for all users
func getReadCommandsAll(config Config) [][]readCommand {
	allCommands := make([][]readCommand, len(config.UserStartAddresses))

	for userIdx, startAddr := range config.UserStartAddresses {
		totalSize := config.RecordsPerUser[userIdx] * config.RecordByteSize
		allCommands[userIdx] = []readCommand{{
			Address: startAddr,
			Size:    totalSize,
		}}
	}

	return allCommands
}

// getReadCommandsOnlyNew returns commands to read only unread records (ring buffer aware)
func getReadCommandsOnlyNew(config Config, settings []byte, debug bool) [][]readCommand {
	allCommands := make([][]readCommand, len(config.UserStartAddresses))

	unreadSection := settings[config.UnreadRecordsRange[0]:config.UnreadRecordsRange[1]]

	for userIdx := range config.UserStartAddresses {
		// Extract last written slot and unread count from settings
		// Byte layout: [user1_slot_hi, user1_slot_lo, user2_slot_hi, user2_slot_lo, user1_unread_hi, user1_unread_lo, user2_unread_hi, user2_unread_lo]
		lastWrittenSlot := ExtractBits(unreadSection[2*userIdx:2*userIdx+2], config.Endianness, 8, 15)
		unreadRecords := ExtractBits(unreadSection[2*userIdx+4:2*userIdx+6], config.Endianness, 8, 15)

		if debug {
			log.Printf("User %d: last written slot=%d, unread records=%d", userIdx+1, lastWrittenSlot, unreadRecords)
		}

		if unreadRecords == 0 {
			allCommands[userIdx] = nil
			continue
		}

		allCommands[userIdx] = calcRingBufferReadLocations(config, userIdx, unreadRecords, lastWrittenSlot)
	}

	return allCommands
}

// calcRingBufferReadLocations calculates read commands for ring buffer with wraparound
func calcRingBufferReadLocations(config Config, userIdx, unreadRecords, lastWrittenSlot int) []readCommand {
	var commands []readCommand

	if lastWrittenSlot < unreadRecords {
		// Ring buffer wraparound - need two reads

		// Read from start of ring buffer to last written slot
		firstRead := readCommand{
			Address: config.UserStartAddresses[userIdx],
			Size:    config.RecordByteSize * lastWrittenSlot,
		}
		if firstRead.Size > 0 {
			commands = append(commands, firstRead)
		}

		// Read from end of ring buffer
		endOffset := config.RecordsPerUser[userIdx] + lastWrittenSlot - unreadRecords
		secondRead := readCommand{
			Address: config.UserStartAddresses[userIdx] + uint16(endOffset*config.RecordByteSize),
			Size:    config.RecordByteSize * (unreadRecords - lastWrittenSlot),
		}
		commands = append(commands, secondRead)
	} else {
		// Simple case - contiguous read
		startOffset := lastWrittenSlot - unreadRecords
		firstRead := readCommand{
			Address: config.UserStartAddresses[userIdx] + uint16(startOffset*config.RecordByteSize),
			Size:    config.RecordByteSize * unreadRecords,
		}
		commands = append(commands, firstRead)
	}

	return commands
}

// buildResetUnreadCounterBytes creates the bytes to reset unread counters
func buildResetUnreadCounterBytes(config Config, settings []byte) []byte {
	unreadSection := settings[config.UnreadRecordsRange[0]:config.UnreadRecordsRange[1]]
	result := make([]byte, len(unreadSection))
	copy(result, unreadSection)

	// Set unread counters to 0x8000 (special "no new records" value)
	resetValue := []byte{0x80, 0x00}
	if config.Endianness == LittleEndian {
		resetValue = []byte{0x00, 0x80}
	}

	// Reset for each user (at offset +4 from start)
	numUsers := len(config.UserStartAddresses)
	for userIdx := 0; userIdx < numUsers; userIdx++ {
		copy(result[2*userIdx+4:2*userIdx+6], resetValue)
	}

	return result
}

// GetRecordsGATT retrieves records from a device using standard BLE GATT protocol
func GetRecordsGATT(ctx context.Context, gattClient *ble.GATTClient, driver Driver, options ReadOptions) ([][]BloodPressureRecord, error) {
	config := driver.GetConfig()

	if options.Debug {
		log.Printf("Using standard GATT protocol for %s", config.Name)
	}

	// Discover services
	if err := gattClient.DiscoverServices(ctx); err != nil {
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}

	// Enable notifications BEFORE anything else
	// Omron devices send measurements via notifications once subscribed
	if err := gattClient.EnableNotifications(ctx); err != nil {
		return nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	// Always sync time for Omron devices - they expect this on every connection
	// The device waits for time sync before sending records
	if err := gattClient.SyncTime(ctx); err != nil {
		if options.Debug {
			log.Printf("Warning: time sync failed: %v", err)
		}
	} else if options.Debug {
		log.Println("Time synchronized")
	}

	// Request all records via RACP if the Omron Option Service is available
	if gattClient.HasRACPSupport() {
		if options.Debug {
			log.Println("Requesting stored records via RACP...")
		}
		if err := gattClient.RequestAllRecords(ctx); err != nil {
			if options.Debug {
				log.Printf("Warning: RACP request failed: %v", err)
			}
		}
	} else if options.Debug {
		log.Println("RACP not available - waiting for automatic record transfer...")
	}

	// Wait for measurements to arrive via notifications
	// Omron devices send all stored records after successful connection
	if options.Debug {
		log.Println("Waiting for measurements (10s)...")
	}
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Collect measurements
	measurements := gattClient.GetMeasurements()
	if options.Debug {
		log.Printf("Received %d measurements", len(measurements))
	}

	// Convert to BloodPressureRecords grouped by user
	userRecords := make(map[uint8][]BloodPressureRecord)
	for _, m := range measurements {
		record := BloodPressureRecord{
			Timestamp: m.Timestamp,
			Systolic:  int(m.Systolic),
			Diastolic: int(m.Diastolic),
			Pulse:     int(m.PulseRate),
			Movement:  m.BodyMovement,
			IHB:       m.IrregularPulse,
		}

		userID := m.UserID
		if !m.HasUserID {
			userID = 1 // Default to user 1
		}
		userRecords[userID] = append(userRecords[userID], record)
	}

	// Convert to slice format (user 1, user 2, etc.)
	var result [][]BloodPressureRecord
	if len(userRecords) == 0 {
		result = [][]BloodPressureRecord{{}}
	} else {
		// Find max user ID
		maxUser := uint8(0)
		for uid := range userRecords {
			if uid > maxUser {
				maxUser = uid
			}
		}
		result = make([][]BloodPressureRecord, maxUser)
		for uid, records := range userRecords {
			if uid > 0 && int(uid) <= len(result) {
				result[uid-1] = records
			}
		}
	}

	return result, nil
}
