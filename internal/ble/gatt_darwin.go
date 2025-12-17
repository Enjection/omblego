//go:build darwin

package ble

import (
	"context"
	"fmt"
	"time"
)

// GATTClient handles standard BLE GATT service communication on Darwin
// This wraps AutoClient to provide a compatible interface
type GATTClient struct {
	client *AutoClient
	debug  bool
}

// NewGATTClient creates a new GATT client for standard BLE services
func NewGATTClient(client *Client, debug bool) *GATTClient {
	// Note: On Darwin, we don't use the Client type, we use AutoClient
	// This constructor exists for API compatibility but won't work properly
	// Use NewGATTClientFromAutoClient instead
	return &GATTClient{
		client: nil,
		debug:  debug,
	}
}

// NewGATTClientFromAutoClient creates a GATT client from an AutoClient
func NewGATTClientFromAutoClient(autoClient *AutoClient, debug bool) *GATTClient {
	return &GATTClient{
		client: autoClient,
		debug:  debug,
	}
}

// DiscoverServices discovers and caches standard BLE service characteristics
func (g *GATTClient) DiscoverServices(ctx context.Context) error {
	if g.client == nil {
		return fmt.Errorf("no AutoClient available")
	}
	return g.client.DiscoverGATTServices(ctx)
}

// EnableNotifications enables notifications for BP measurements and RACP
func (g *GATTClient) EnableNotifications(ctx context.Context) error {
	if g.client == nil {
		return fmt.Errorf("no AutoClient available")
	}
	return g.client.EnableGATTNotifications()
}

// SyncTime synchronizes device time with system time
func (g *GATTClient) SyncTime(ctx context.Context) error {
	if g.client == nil {
		return fmt.Errorf("no AutoClient available")
	}
	return g.client.SyncCurrentTime()
}

// WaitForPairing waits for the device to complete the pairing handshake
func (g *GATTClient) WaitForPairing(ctx context.Context, timeout time.Duration) error {
	if g.client == nil {
		return fmt.Errorf("no AutoClient available")
	}

	if g.debug {
		fmt.Println("[GATT] Waiting for pairing handshake...")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for either time notification or battery notification
	timeCh := g.client.GetCurrentTimeChannel()
	batteryCh := g.client.GetBatteryLevelChannel()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("pairing timeout - no response from device")
		case <-timeCh:
			if g.debug {
				fmt.Println("[GATT] Received time notification")
			}
			// Respond with current time
			if err := g.SyncTime(ctx); err != nil {
				if g.debug {
					fmt.Printf("[GATT] Time sync failed: %v\n", err)
				}
			}
			time.Sleep(1 * time.Second)
			return nil
		case <-batteryCh:
			if g.debug {
				fmt.Println("[GATT] Received battery notification")
			}
			// Sync time to complete pairing
			if err := g.SyncTime(ctx); err != nil {
				if g.debug {
					fmt.Printf("[GATT] Time sync warning: %v\n", err)
				}
			}
			time.Sleep(500 * time.Millisecond)
			return nil
		}
	}
}

// WaitForDeviceDisconnect waits for the device to disconnect itself
func (g *GATTClient) WaitForDeviceDisconnect(ctx context.Context, timeout time.Duration) error {
	if g.client == nil {
		return fmt.Errorf("no AutoClient available")
	}
	return g.client.WaitForDisconnect(ctx, timeout)
}

// RequestAllRecords is not supported on Darwin GATT (records come via notifications)
func (g *GATTClient) RequestAllRecords(ctx context.Context) error {
	// On Darwin with standard GATT, records are sent automatically via notifications
	// after time sync, so this is essentially a no-op
	if g.debug {
		fmt.Println("[GATT] RequestAllRecords: records are sent via notifications on Darwin")
	}
	return nil
}

// GetMeasurements returns collected measurements
func (g *GATTClient) GetMeasurements() []*BloodPressureMeasurement {
	if g.client == nil {
		return nil
	}

	// Convert GATTBloodPressureMeasurement to BloodPressureMeasurement
	gattMeasurements := g.client.GetCollectedMeasurements()
	result := make([]*BloodPressureMeasurement, len(gattMeasurements))
	for i, m := range gattMeasurements {
		result[i] = &BloodPressureMeasurement{
			Systolic:       m.Systolic,
			Diastolic:      m.Diastolic,
			MeanAP:         m.MeanAP,
			Unit:           m.Unit,
			Timestamp:      m.Timestamp,
			PulseRate:      m.PulseRate,
			UserID:         m.UserID,
			HasPulse:       m.HasPulse,
			HasUserID:      m.HasUserID,
			HasTimestamp:   m.HasTimestamp,
			BodyMovement:   m.BodyMovement,
			IrregularPulse: m.IrregularPulse,
		}
	}
	return result
}

// GetBatteryLevel reads the battery level (not implemented for Darwin GATT)
func (g *GATTClient) GetBatteryLevel() (uint8, error) {
	// Battery level is received via notifications on Darwin
	return 0, fmt.Errorf("use battery notification channel instead")
}

// HasRACPSupport returns false on Darwin (we use notifications instead)
func (g *GATTClient) HasRACPSupport() bool {
	return false
}

// BloodPressureMeasurement represents a parsed BLE blood pressure measurement
// This is needed for API compatibility with the non-Darwin code
type BloodPressureMeasurement struct {
	Systolic       float32
	Diastolic      float32
	MeanAP         float32
	Unit           string
	Timestamp      time.Time
	PulseRate      float32
	UserID         uint8
	HasPulse       bool
	HasUserID      bool
	HasTimestamp   bool
	BodyMovement   bool
	IrregularPulse bool
}
