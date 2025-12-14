package output_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"omblego/internal/output"
)

func TestCSVBackend_Write(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "csv_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	backend, err := output.NewCSVBackend("test-csv", output.CSVBackendConfig{
		Directory:   tmpDir,
		FilePattern: "user{user}.csv",
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
				Movement:  false,
				IHB:       false,
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
	csvPath := filepath.Join(tmpDir, "user1.csv")
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		t.Error("CSV file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV: %v", err)
	}

	if len(content) == 0 {
		t.Error("CSV file is empty")
	}
}

func TestCSVBackend_Name(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "csv_test")
	defer os.RemoveAll(tmpDir)

	backend, _ := output.NewCSVBackend("my-csv", output.CSVBackendConfig{
		Directory: tmpDir,
	})

	if backend.Name() != "my-csv" {
		t.Errorf("Expected name 'my-csv', got '%s'", backend.Name())
	}
}

func TestCSVBackend_Type(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "csv_test")
	defer os.RemoveAll(tmpDir)

	backend, _ := output.NewCSVBackend("test", output.CSVBackendConfig{
		Directory: tmpDir,
	})

	if backend.Type() != "csv" {
		t.Errorf("Expected type 'csv', got '%s'", backend.Type())
	}
}

func TestCSVBackend_Health(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "csv_test")
	defer os.RemoveAll(tmpDir)

	backend, _ := output.NewCSVBackend("test", output.CSVBackendConfig{
		Directory: tmpDir,
	})

	err := backend.Health(context.Background())
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}
