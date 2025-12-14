package output_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"omblego/internal/output"
)

func TestScriptBackend_Write(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	// Create temp directory for output
	tmpDir, err := os.MkdirTemp("", "script_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.txt")

	// Create a simple script that writes stdin to a file
	scriptPath := filepath.Join(tmpDir, "test.sh")
	scriptContent := `#!/bin/sh
cat > "` + outputFile + `"
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create script: %v", err)
	}

	backend, err := output.NewScriptBackend("test-script", output.ScriptBackendConfig{
		Command: scriptPath,
		Format:  "json",
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	records := [][]output.Record{
		{
			{
				Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				Systolic:  120,
				Diastolic: 80,
				Pulse:     72,
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

	// Verify output file was created
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	if len(content) == 0 {
		t.Error("Output file is empty")
	}
}

func TestScriptBackend_CSVFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	tmpDir, _ := os.MkdirTemp("", "script_test")
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.csv")
	scriptPath := filepath.Join(tmpDir, "test.sh")
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
cat > "`+outputFile+`"
`), 0755)

	backend, _ := output.NewScriptBackend("test", output.ScriptBackendConfig{
		Command: scriptPath,
		Format:  "csv",
	})

	records := [][]output.Record{
		{
			{
				Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				Systolic:  120,
				Diastolic: 80,
				Pulse:     72,
			},
		},
	}

	backend.Write(context.Background(), records, output.Metadata{})

	content, _ := os.ReadFile(outputFile)
	if len(content) == 0 {
		t.Error("CSV output is empty")
	}
}

func TestScriptBackend_Type(t *testing.T) {
	backend, _ := output.NewScriptBackend("test", output.ScriptBackendConfig{
		Command: "echo",
	})

	if backend.Type() != "script" {
		t.Errorf("Expected type 'script', got '%s'", backend.Type())
	}
}

func TestScriptBackend_RequiresCommand(t *testing.T) {
	_, err := output.NewScriptBackend("test", output.ScriptBackendConfig{})
	if err == nil {
		t.Error("Expected error when command is missing")
	}
}

func TestScriptBackend_InvalidFormat(t *testing.T) {
	_, err := output.NewScriptBackend("test", output.ScriptBackendConfig{
		Command: "echo",
		Format:  "invalid",
	})
	if err == nil {
		t.Error("Expected error for invalid format")
	}
}

func TestScriptBackend_Health(t *testing.T) {
	backend, _ := output.NewScriptBackend("test", output.ScriptBackendConfig{
		Command: "echo",
	})

	err := backend.Health(context.Background())
	if err != nil {
		t.Errorf("Health check failed for 'echo': %v", err)
	}
}

func TestScriptBackend_HealthNonexistent(t *testing.T) {
	backend, _ := output.NewScriptBackend("test", output.ScriptBackendConfig{
		Command: "nonexistent_command_xyz",
	})

	err := backend.Health(context.Background())
	if err == nil {
		t.Error("Expected error for nonexistent command")
	}
}
