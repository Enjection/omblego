package output_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"omblego/internal/device"
	"omblego/internal/output"
	"omblego/internal/output/mock"
)

func TestManager_Write_SingleBackend(t *testing.T) {
	backend := mock.NewBackend("test")
	mgr := output.NewManagerWithBackends([]output.Backend{backend})
	defer mgr.Close()

	records := [][]output.Record{
		{
			{Timestamp: time.Now(), Systolic: 120, Diastolic: 80, Pulse: 70},
			{Timestamp: time.Now(), Systolic: 118, Diastolic: 78, Pulse: 68},
		},
	}
	metadata := output.Metadata{
		DeviceName:    "BP7000",
		SyncTimestamp: time.Now(),
	}

	results := mgr.Write(context.Background(), records, metadata)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0].Error != nil {
		t.Errorf("Write failed: %v", results[0].Error)
	}

	if results[0].RecordsWritten != 2 {
		t.Errorf("Expected 2 records written, got %d", results[0].RecordsWritten)
	}

	if backend.GetWriteCalls() != 1 {
		t.Errorf("Expected 1 write call, got %d", backend.GetWriteCalls())
	}
}

func TestManager_Write_MultipleBackends(t *testing.T) {
	backend1 := mock.NewBackend("backend1")
	backend2 := mock.NewBackend("backend2")
	mgr := output.NewManagerWithBackends([]output.Backend{backend1, backend2})
	defer mgr.Close()

	records := [][]output.Record{
		{{Timestamp: time.Now(), Systolic: 120, Diastolic: 80, Pulse: 70}},
	}
	metadata := output.Metadata{DeviceName: "BP7000"}

	results := mgr.Write(context.Background(), records, metadata)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Errorf("Backend %s failed: %v", r.Backend, r.Error)
		}
	}

	if backend1.GetWriteCalls() != 1 || backend2.GetWriteCalls() != 1 {
		t.Errorf("Each backend should be called once")
	}
}

func TestManager_Write_PartialFailure(t *testing.T) {
	backend1 := mock.NewBackend("backend1")
	backend2 := mock.NewBackend("backend2").WithWriteError(errors.New("write failed"))
	mgr := output.NewManagerWithBackends([]output.Backend{backend1, backend2})
	defer mgr.Close()

	records := [][]output.Record{
		{{Timestamp: time.Now(), Systolic: 120, Diastolic: 80, Pulse: 70}},
	}
	metadata := output.Metadata{DeviceName: "BP7000"}

	results := mgr.Write(context.Background(), records, metadata)

	// Both backends should be attempted
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Find results by backend name
	var backend1Result, backend2Result *output.WriteResult
	for i := range results {
		if results[i].Backend == "backend1" {
			backend1Result = &results[i]
		} else if results[i].Backend == "backend2" {
			backend2Result = &results[i]
		}
	}

	if backend1Result == nil || backend1Result.Error != nil {
		t.Error("backend1 should succeed")
	}

	if backend2Result == nil || backend2Result.Error == nil {
		t.Error("backend2 should fail")
	}
}

func TestManager_HealthCheck(t *testing.T) {
	backend1 := mock.NewBackend("healthy")
	backend2 := mock.NewBackend("unhealthy").WithHealthError(errors.New("connection refused"))
	mgr := output.NewManagerWithBackends([]output.Backend{backend1, backend2})
	defer mgr.Close()

	results := mgr.HealthCheck(context.Background())

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results["healthy"] != nil {
		t.Errorf("healthy backend should return nil error")
	}

	if results["unhealthy"] == nil {
		t.Error("unhealthy backend should return error")
	}
}

func TestManager_Close(t *testing.T) {
	backend1 := mock.NewBackend("backend1")
	backend2 := mock.NewBackend("backend2")
	mgr := output.NewManagerWithBackends([]output.Backend{backend1, backend2})

	err := mgr.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !backend1.CloseCalled {
		t.Error("backend1.Close should be called")
	}

	if !backend2.CloseCalled {
		t.Error("backend2.Close should be called")
	}
}

func TestManager_NewManagerFromConfig(t *testing.T) {
	// Register mock backend
	// (already registered via init() in mock package)

	configs := []output.BackendConfig{
		{Type: "mock", Name: "mock1", Enabled: true},
		{Type: "mock", Name: "mock2", Enabled: true},
		{Type: "mock", Name: "disabled", Enabled: false},
	}

	mgr, err := output.NewManager(configs)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Close()

	backends := mgr.Backends()
	if len(backends) != 2 {
		t.Errorf("Expected 2 backends (disabled excluded), got %d", len(backends))
	}
}

func TestFactory_Register_And_Create(t *testing.T) {
	// Create using registered mock backend
	cfg := output.BackendConfig{
		Type:    "mock",
		Name:    "test-backend",
		Enabled: true,
	}

	backend, err := output.Create(cfg)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if backend.Name() != "test-backend" {
		t.Errorf("Expected name 'test-backend', got '%s'", backend.Name())
	}

	if backend.Type() != "mock" {
		t.Errorf("Expected type 'mock', got '%s'", backend.Type())
	}
}

func TestFactory_UnknownType(t *testing.T) {
	cfg := output.BackendConfig{
		Type:    "unknown-type",
		Enabled: true,
	}

	_, err := output.Create(cfg)
	if err == nil {
		t.Error("Expected error for unknown type")
	}
}

func TestFactory_ListTypes(t *testing.T) {
	types := output.ListTypes()

	// Should at least contain "mock"
	found := false
	for _, typ := range types {
		if typ == "mock" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected 'mock' in registered types: %v", types)
	}
}

func TestMetadata(t *testing.T) {
	backend := mock.NewBackend("test")
	mgr := output.NewManagerWithBackends([]output.Backend{backend})
	defer mgr.Close()

	expectedMeta := output.Metadata{
		DeviceName:    "BP7000",
		SyncTimestamp: time.Now(),
		IsFullSync:    true,
		SessionID:     "session-123",
	}

	records := [][]device.BloodPressureRecord{
		{{Timestamp: time.Now(), Systolic: 120, Diastolic: 80, Pulse: 70}},
	}

	mgr.Write(context.Background(), records, expectedMeta)

	writtenMeta := backend.GetWrittenMetadata()
	if len(writtenMeta) != 1 {
		t.Fatalf("Expected 1 metadata entry, got %d", len(writtenMeta))
	}

	if writtenMeta[0].DeviceName != expectedMeta.DeviceName {
		t.Errorf("DeviceName mismatch")
	}
	if writtenMeta[0].IsFullSync != expectedMeta.IsFullSync {
		t.Errorf("IsFullSync mismatch")
	}
	if writtenMeta[0].SessionID != expectedMeta.SessionID {
		t.Errorf("SessionID mismatch")
	}
}
