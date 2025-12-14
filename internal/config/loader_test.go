package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"omblego/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Device.UUID != "auto" {
		t.Errorf("Expected UUID 'auto', got %s", cfg.Device.UUID)
	}

	if cfg.Daemon.Interval != 3*time.Hour {
		t.Errorf("Expected interval 3h, got %v", cfg.Daemon.Interval)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Output != "stderr" {
		t.Errorf("Expected log output 'stderr', got %s", cfg.Logging.Output)
	}

	if len(cfg.Outputs) != 1 {
		t.Errorf("Expected 1 default output, got %d", len(cfg.Outputs))
	}

	if cfg.Outputs[0].Type != "csv" {
		t.Errorf("Expected CSV output, got %s", cfg.Outputs[0].Type)
	}
}

func TestLoadDefault_NoFile(t *testing.T) {
	// Should load defaults when no config file exists
	cfg, err := config.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault failed: %v", err)
	}

	// Should have default values
	if cfg.Device.UUID != "auto" {
		t.Errorf("Expected default UUID 'auto', got %s", cfg.Device.UUID)
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
device:
  model: BP7000
  uuid: test-uuid-123

daemon:
  enabled: true
  interval: 1h
  time_sync: false

logging:
  level: debug
  format: json
  output: file
  file: /var/log/omblego.log

outputs:
  - type: csv
    enabled: true
    name: local-csv
    settings:
      directory: /data
      file_pattern: "bp_{user}.csv"
  - type: influxdb
    enabled: true
    name: metrics
    settings:
      url: http://localhost:8086
      token: my-token
      org: myorg
      bucket: health
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Check device settings
	if cfg.Device.Model != "BP7000" {
		t.Errorf("Expected model BP7000, got %s", cfg.Device.Model)
	}
	if cfg.Device.UUID != "test-uuid-123" {
		t.Errorf("Expected UUID test-uuid-123, got %s", cfg.Device.UUID)
	}

	// Check daemon settings
	if !cfg.Daemon.Enabled {
		t.Error("Expected daemon enabled")
	}
	if cfg.Daemon.Interval != time.Hour {
		t.Errorf("Expected interval 1h, got %v", cfg.Daemon.Interval)
	}
	if cfg.Daemon.TimeSync {
		t.Error("Expected time_sync false")
	}

	// Check logging settings
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected format json, got %s", cfg.Logging.Format)
	}
	if cfg.Logging.Output != "file" {
		t.Errorf("Expected output file, got %s", cfg.Logging.Output)
	}
	if cfg.Logging.File != "/var/log/omblego.log" {
		t.Errorf("Expected file /var/log/omblego.log, got %s", cfg.Logging.File)
	}

	// Check outputs
	if len(cfg.Outputs) != 2 {
		t.Fatalf("Expected 2 outputs, got %d", len(cfg.Outputs))
	}

	if cfg.Outputs[0].Type != "csv" {
		t.Errorf("Expected first output type csv, got %s", cfg.Outputs[0].Type)
	}
	if cfg.Outputs[0].Name != "local-csv" {
		t.Errorf("Expected first output name local-csv, got %s", cfg.Outputs[0].Name)
	}

	if cfg.Outputs[1].Type != "influxdb" {
		t.Errorf("Expected second output type influxdb, got %s", cfg.Outputs[1].Type)
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := config.LoadFromFile("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestEnvironmentOverride(t *testing.T) {
	// Set environment variables
	os.Setenv("OMBLEGO_DEVICE_MODEL", "ENV_MODEL")
	os.Setenv("OMBLEGO_LOGGING_LEVEL", "error")
	defer os.Unsetenv("OMBLEGO_DEVICE_MODEL")
	defer os.Unsetenv("OMBLEGO_LOGGING_LEVEL")

	cfg, err := config.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault failed: %v", err)
	}

	if cfg.Device.Model != "ENV_MODEL" {
		t.Errorf("Expected model ENV_MODEL from env, got %s", cfg.Device.Model)
	}

	if cfg.Logging.Level != "error" {
		t.Errorf("Expected level error from env, got %s", cfg.Logging.Level)
	}
}

func TestLoader_Set_Get(t *testing.T) {
	loader := config.NewLoader()
	_, err := loader.Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err = loader.Set("device.model", "OVERRIDE")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	value := loader.Get("device.model")
	if value != "OVERRIDE" {
		t.Errorf("Expected OVERRIDE, got %v", value)
	}
}

func TestConfigPaths(t *testing.T) {
	paths := config.ConfigPaths()

	if len(paths) < 2 {
		t.Errorf("Expected at least 2 paths, got %d", len(paths))
	}

	// First paths should be local
	if paths[0] != "./omblego.yaml" {
		t.Errorf("Expected first path to be ./omblego.yaml, got %s", paths[0])
	}
}
