package omblego_test

import (
	"context"
	"testing"
	"time"

	// Import devices package to trigger driver registration
	_ "omblego/internal/device/devices"

	"omblego/pkg/omblego"
)

func TestNew(t *testing.T) {
	client, err := omblego.New("BP7000")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.DeviceName() != "BP7000" {
		t.Errorf("Expected device name BP7000, got %s", client.DeviceName())
	}
}

func TestNew_WithOptions(t *testing.T) {
	client, err := omblego.New("BP7000",
		omblego.WithTimeSync(true),
		omblego.WithConnectTimeout(60*time.Second),
		omblego.WithDebug(true),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.DeviceName() != "BP7000" {
		t.Errorf("Expected device name BP7000, got %s", client.DeviceName())
	}
}

func TestNew_EmptyDeviceName(t *testing.T) {
	_, err := omblego.New("")
	if err != omblego.ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestClient_Sync(t *testing.T) {
	// Skip test that requires actual BLE hardware
	t.Skip("Skipping test that requires BLE hardware")

	client, _ := omblego.New("BP7000")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if result.DeviceName != "BP7000" {
		t.Errorf("Expected device BP7000, got %s", result.DeviceName)
	}
}

func TestClient_SyncInProgress(t *testing.T) {
	// Skip test that requires actual BLE hardware
	t.Skip("Skipping test that requires BLE hardware")

	client, _ := omblego.New("BP7000")

	// Start first sync in background
	ctx1 := context.Background()
	go client.Sync(ctx1)

	// Small delay to ensure first sync starts
	time.Sleep(10 * time.Millisecond)

	// Try second sync - should fail
	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Sync(ctx2)
	if err != omblego.ErrSyncInProgress {
		// Note: This test is racy, so we accept either error or success
		t.Logf("Got error: %v (may vary due to timing)", err)
	}
}

func TestClient_Close(t *testing.T) {
	client, _ := omblego.New("BP7000")

	err := client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestListSupportedDevices(t *testing.T) {
	devices := omblego.ListSupportedDevices()

	if len(devices) == 0 {
		t.Error("Expected at least one supported device")
	}

	// Check for known devices
	found := false
	for _, d := range devices {
		if d == "BP7000" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected BP7000 in supported devices")
	}
}

func TestSyncOptions_Defaults(t *testing.T) {
	opts := omblego.DefaultSyncOptions()

	if opts.ConnectTimeout == 0 {
		t.Error("Expected non-zero connect timeout")
	}

	if opts.ReadTimeout == 0 {
		t.Error("Expected non-zero read timeout")
	}
}

func TestDaemonOptions_Defaults(t *testing.T) {
	opts := omblego.DefaultDaemonOptions()

	if opts.Interval == 0 {
		t.Error("Expected non-zero interval")
	}

	if opts.SyncOptions.ConnectTimeout == 0 {
		t.Error("Expected non-zero connect timeout in sync options")
	}
}

func TestUserRecords_TotalCount(t *testing.T) {
	records := omblego.UserRecords{
		{
			{Timestamp: time.Now(), Systolic: 120, Diastolic: 80},
			{Timestamp: time.Now(), Systolic: 118, Diastolic: 78},
		},
		{
			{Timestamp: time.Now(), Systolic: 130, Diastolic: 85},
		},
	}

	if records.TotalCount() != 3 {
		t.Errorf("Expected 3 records, got %d", records.TotalCount())
	}
}

func TestUserRecords_UserCount(t *testing.T) {
	records := omblego.UserRecords{
		{
			{Timestamp: time.Now(), Systolic: 120, Diastolic: 80},
		},
		{}, // Empty user
		{
			{Timestamp: time.Now(), Systolic: 130, Diastolic: 85},
		},
	}

	if records.UserCount() != 2 {
		t.Errorf("Expected 2 users with records, got %d", records.UserCount())
	}
}

func TestUserRecords_Latest(t *testing.T) {
	now := time.Now()

	records := omblego.UserRecords{
		{
			{Timestamp: now.Add(-2 * time.Hour), Systolic: 120, Diastolic: 80},
		},
		{
			{Timestamp: now.Add(-1 * time.Hour), Systolic: 130, Diastolic: 85},
			{Timestamp: now, Systolic: 125, Diastolic: 82}, // Latest
		},
	}

	latest := records.Latest()
	if latest == nil {
		t.Fatal("Expected latest record, got nil")
	}

	if latest.Systolic != 125 {
		t.Errorf("Expected systolic 125, got %d", latest.Systolic)
	}
}

func TestUserRecords_Latest_Empty(t *testing.T) {
	records := omblego.UserRecords{}

	latest := records.Latest()
	if latest != nil {
		t.Error("Expected nil for empty records")
	}
}
