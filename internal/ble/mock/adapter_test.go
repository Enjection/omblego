package mock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"omblego/internal/ble"
	"omblego/internal/ble/mock"
)

func TestMockAdapter_Enable(t *testing.T) {
	adapter := mock.NewAdapter()

	err := adapter.Enable(context.Background())
	if err != nil {
		t.Errorf("Enable() returned error: %v", err)
	}

	if !adapter.EnableCalled {
		t.Error("EnableCalled should be true")
	}
}

func TestMockAdapter_Scan(t *testing.T) {
	ctx := context.Background()
	adapter := mock.NewAdapter().WithScanResults(
		ble.ScanResult{Address: "AA:BB:CC:DD:EE:FF", Name: "BP7000", RSSI: -50},
		ble.ScanResult{Address: "11:22:33:44:55:66", Name: "HEM-7322T", RSSI: -60},
	)

	results, err := adapter.Scan(ctx, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if !adapter.ScanCalled {
		t.Error("ScanCalled should be true")
	}
}

func TestMockAdapter_Scan_WithError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("bluetooth disabled")
	adapter := mock.NewAdapter().WithScanError(expectedErr)

	_, err := adapter.Scan(ctx, 10*time.Millisecond)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestMockAdapter_Connect(t *testing.T) {
	ctx := context.Background()
	adapter := mock.NewAdapter()

	conn, err := adapter.Connect(ctx, "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("Connect() returned error: %v", err)
	}

	if conn == nil {
		t.Fatal("Connect() returned nil connection")
	}

	if conn.Address() != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Expected address AA:BB:CC:DD:EE:FF, got %s", conn.Address())
	}

	if !adapter.ConnectCalled {
		t.Error("ConnectCalled should be true")
	}
}

func TestMockConnection_Write(t *testing.T) {
	conn := mock.NewConnection("AA:BB:CC:DD:EE:FF")

	testData := []byte{0x01, 0x02, 0x03}
	err := conn.Write("test-uuid", testData)
	if err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	written := conn.GetWrittenData("test-uuid")
	if len(written) != 1 {
		t.Fatalf("Expected 1 write, got %d", len(written))
	}

	if string(written[0]) != string(testData) {
		t.Errorf("Written data mismatch: expected %v, got %v", testData, written[0])
	}
}

func TestMockConnection_Notifications(t *testing.T) {
	conn := mock.NewConnection("AA:BB:CC:DD:EE:FF")

	received := make(chan []byte, 1)
	err := conn.EnableNotifications("notify-uuid", func(data []byte) {
		received <- data
	})
	if err != nil {
		t.Fatalf("EnableNotifications() returned error: %v", err)
	}

	// Simulate notification
	testData := []byte{0xAA, 0xBB, 0xCC}
	err = conn.SimulateNotification("notify-uuid", testData)
	if err != nil {
		t.Fatalf("SimulateNotification() returned error: %v", err)
	}

	select {
	case data := <-received:
		if string(data) != string(testData) {
			t.Errorf("Received data mismatch: expected %v, got %v", testData, data)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for notification")
	}
}

func TestMockConnection_Disconnect(t *testing.T) {
	conn := mock.NewConnection("AA:BB:CC:DD:EE:FF")

	if !conn.IsConnected() {
		t.Error("Should be connected initially")
	}

	err := conn.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect() returned error: %v", err)
	}

	if conn.IsConnected() {
		t.Error("Should not be connected after disconnect")
	}

	if !conn.DisconnectCalled {
		t.Error("DisconnectCalled should be true")
	}
}

func TestMockConnection_WriteAfterDisconnect(t *testing.T) {
	conn := mock.NewConnection("AA:BB:CC:DD:EE:FF")
	conn.Disconnect()

	err := conn.Write("test-uuid", []byte{0x01})
	if err == nil {
		t.Error("Write() should return error after disconnect")
	}
}
