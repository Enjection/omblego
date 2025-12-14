package sync

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"omblego/internal/ble"
	"omblego/internal/config"
	"omblego/internal/device"
	"omblego/internal/output"
)

// Run performs a sync operation with the given options.
// It connects to the BLE device, reads records, optionally syncs time,
// and writes to configured outputs.
func (s *Syncer) Run(ctx context.Context, opts Options) (*Result, error) {
	// Get device driver
	driver, err := device.GetDriver(opts.DeviceName)
	if err != nil {
		return nil, fmt.Errorf("unsupported device %s: %w", opts.DeviceName, err)
	}

	cfg := driver.GetConfig()
	startTime := time.Now()

	// Get device UUID
	targetUUID := opts.DeviceUUID
	if targetUUID == "" && opts.DeviceMAC != "" {
		targetUUID = opts.DeviceMAC
	}
	if targetUUID == "" {
		savedUUID, err := config.GetDeviceUUID(cfg.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load device config: %w", err)
		}
		if savedUUID == "" {
			return nil, fmt.Errorf("no saved UUID for %s - run pair first", cfg.Name)
		}
		targetUUID = savedUUID
	}

	s.logger.Debug("starting sync", "device", opts.DeviceName, "uuid", targetUUID)

	// Perform the BLE sync
	var records [][]device.BloodPressureRecord
	if runtime.GOOS == "darwin" {
		records, err = s.runAutoSync(ctx, driver, cfg, targetUUID, opts)
	} else {
		records, err = s.runManualSync(ctx, driver, cfg, targetUUID, opts)
	}

	if err != nil {
		return nil, err
	}

	// Count records
	totalCount := 0
	for _, userRecords := range records {
		totalCount += len(userRecords)
	}

	result := &Result{
		Records:       records,
		DeviceName:    cfg.Name,
		DeviceUUID:    targetUUID,
		SyncTimestamp: startTime,
		RecordCount:   totalCount,
		IsFullSync:    !opts.NewRecOnly,
	}

	// Write to outputs if configured
	if s.outputs != nil && totalCount > 0 {
		metadata := output.Metadata{
			DeviceName:    cfg.Name,
			SyncTimestamp: startTime,
			IsFullSync:    !opts.NewRecOnly,
		}
		outputRecords := make([][]output.Record, len(records))
		for i, userRecs := range records {
			outputRecords[i] = userRecs
		}
		results := s.outputs.Write(ctx, outputRecords, metadata)
		for _, r := range results {
			if r.Error != nil {
				s.logger.Warn("output write failed", "backend", r.Backend, "error", r.Error)
			}
		}
	}

	s.logger.Info("sync completed", "records", totalCount, "duration", time.Since(startTime))
	return result, nil
}

// runAutoSync uses macOS auto-reconnect for energy-efficient sync
func (s *Syncer) runAutoSync(ctx context.Context, driver device.Driver, cfg device.Config, targetUUID string, opts Options) ([][]device.BloodPressureRecord, error) {
	autoClient, err := ble.NewAutoClient(opts.Debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create auto client: %w", err)
	}
	defer autoClient.Disconnect()

	// Wait for Bluetooth
	if err := autoClient.WaitReady(ctx); err != nil {
		return nil, fmt.Errorf("bluetooth not ready: %w", err)
	}

	// Connect with timeout
	connectTimeout := opts.ConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = 2 * time.Minute
	}
	s.logger.Debug("waiting for device", "timeout", connectTimeout)

	if err := autoClient.ConnectByUUID(ctx, targetUUID, connectTimeout); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	// Discover services
	if err := autoClient.DiscoverOmronServices(ctx); err != nil {
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	// Setup handler
	handler := ble.NewAutoTxRxHandler(autoClient, opts.Debug)
	if err := handler.SetupNotifications(); err != nil {
		return nil, fmt.Errorf("failed to setup notifications: %w", err)
	}

	// Wait for connection to stabilize
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Unlock device with retry
	var unlockErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(2 * time.Second)
		}
		if unlockErr = handler.UnlockWithKey(ctx, nil); unlockErr == nil {
			break
		}
	}
	if unlockErr != nil {
		return nil, fmt.Errorf("failed to unlock device: %w", unlockErr)
	}

	// Start transmission with retry
	var startErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(1 * time.Second)
		}
		if startErr = handler.StartTransmission(ctx); startErr == nil {
			break
		}
	}
	if startErr != nil {
		return nil, fmt.Errorf("failed to start transmission: %w", startErr)
	}

	// Time sync if requested
	if opts.TimeSync {
		s.syncDeviceTime(ctx, handler, driver, cfg)
	}

	// Read records
	records, err := s.readRecords(ctx, handler, driver, cfg)
	handler.EndTransmission(ctx)

	if err != nil {
		return nil, err
	}

	return [][]device.BloodPressureRecord{records}, nil
}

// runManualSync uses standard BLE connection (non-macOS)
func (s *Syncer) runManualSync(ctx context.Context, driver device.Driver, cfg device.Config, targetUUID string, opts Options) ([][]device.BloodPressureRecord, error) {
	client, err := ble.NewClient(opts.Debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create BLE client: %w", err)
	}
	defer client.Close()

	// Connect
	if err := client.Connect(ctx, targetUUID); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer client.Disconnect()

	// Wait for OS connection to stabilize
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Read records based on protocol
	readOpts := device.ReadOptions{
		UseUnreadCounter: opts.NewRecOnly,
		SyncTime:         opts.TimeSync,
		Debug:            opts.Debug,
	}

	var records [][]device.BloodPressureRecord
	if cfg.Protocol == device.ProtocolStandardGATT {
		gattClient := ble.NewGATTClient(client, opts.Debug)
		records, err = device.GetRecordsGATT(ctx, gattClient, driver, readOpts)
	} else {
		handler := ble.NewTxRxHandler(client, opts.Debug)
		records, err = device.GetRecords(ctx, handler, driver, readOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	return records, nil
}

// syncDeviceTime syncs the device clock with system time
func (s *Syncer) syncDeviceTime(ctx context.Context, handler *ble.AutoTxRxHandler, driver device.Driver, cfg device.Config) {
	settingsSize := int(cfg.SettingsWriteAddress - cfg.SettingsReadAddress)
	cachedSettings, err := handler.ReadEEPROM(ctx, cfg.SettingsReadAddress, settingsSize, cfg.TransmissionBlockSize)
	if err != nil {
		s.logger.Warn("failed to read settings for time sync", "error", err)
		return
	}

	syncBytes, err := driver.BuildTimeSyncBytes(cachedSettings, time.Now())
	if err != nil {
		s.logger.Warn("failed to build time sync bytes", "error", err)
		return
	}

	writeAddr := cfg.SettingsWriteAddress + uint16(cfg.TimeSyncRange[0])
	if err := handler.WriteEEPROM(ctx, writeAddr, syncBytes, len(syncBytes)); err != nil {
		s.logger.Warn("time sync write failed", "error", err)
		return
	}

	s.logger.Debug("device time synchronized")
}

// readRecords reads blood pressure records from the device
func (s *Syncer) readRecords(ctx context.Context, handler *ble.AutoTxRxHandler, driver device.Driver, cfg device.Config) ([]device.BloodPressureRecord, error) {
	totalSize := cfg.RecordsPerUser[0] * cfg.RecordByteSize
	recordData, err := handler.ReadEEPROM(ctx, cfg.UserStartAddresses[0], totalSize, cfg.TransmissionBlockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	var records []device.BloodPressureRecord
	for offset := 0; offset < len(recordData); offset += cfg.RecordByteSize {
		if offset+cfg.RecordByteSize > len(recordData) {
			break
		}

		data := recordData[offset : offset+cfg.RecordByteSize]

		// Skip empty records (all 0xFF)
		isEmpty := true
		for _, b := range data {
			if b != 0xFF {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			continue
		}

		record, err := driver.ParseRecord(data)
		if err != nil {
			s.logger.Debug("failed to parse record", "offset", offset, "error", err)
			continue
		}
		records = append(records, *record)
	}

	return records, nil
}
