package output_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"omblego/internal/output"
)

func TestJSONBackend_Write(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "json_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	backend, err := output.NewJSONBackend("test-json", output.JSONBackendConfig{
		Directory: tmpDir,
		Filename:  "test.json",
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	records := [][]output.Record{
		{
			{
				Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.Local),
				Systolic:  120,
				Diastolic: 80,
				Pulse:     72,
			},
		},
		{
			{
				Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.Local),
				Systolic:  118,
				Diastolic: 78,
				Pulse:     68,
			},
		},
	}

	metadata := output.Metadata{
		DeviceName:    "BP7000",
		SyncTimestamp: time.Now(),
	}

	err = backend.Write(context.Background(), records, metadata)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists
	jsonPath := filepath.Join(tmpDir, "test.json")
	content, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	// Verify it's valid JSON
	var data output.UBPMOutput
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Verify structure
	if len(data.UBPM) != 2 {
		t.Errorf("Expected 2 users, got %d", len(data.UBPM))
	}

	if len(data.UBPM["U1"]) != 1 {
		t.Errorf("Expected 1 record for U1, got %d", len(data.UBPM["U1"]))
	}
}

func TestJSONBackend_Type(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "json_test")
	defer os.RemoveAll(tmpDir)

	backend, _ := output.NewJSONBackend("test", output.JSONBackendConfig{
		Directory: tmpDir,
	})

	if backend.Type() != "json" {
		t.Errorf("Expected type 'json', got '%s'", backend.Type())
	}
}
