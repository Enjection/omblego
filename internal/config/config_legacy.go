// Package config handles persistent configuration for omblego
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LegacyDeviceConfig stores saved device information (JSON format)
type LegacyDeviceConfig struct {
	UUID string `json:"uuid"` // BLE peripheral UUID
	MAC  string `json:"mac"`  // MAC address (if available)
	Name string `json:"name"` // Device name (e.g., "BP7000")
}

// LegacyInfluxDBConfig stores InfluxDB connection settings (JSON format)
type LegacyInfluxDBConfig struct {
	URL    string `json:"url"`
	Token  string `json:"token"`
	Org    string `json:"org"`
	Bucket string `json:"bucket"`
}

// LegacyDaemonConfig stores daemon mode settings (JSON format)
type LegacyDaemonConfig struct {
	Interval string `json:"interval"` // e.g., "3h", "30m"
	TimeSync bool   `json:"time_sync"`
}

// LegacyConfig is the legacy JSON configuration structure
type LegacyConfig struct {
	Device   string                          `json:"device"`            // Default device name
	UUID     string                          `json:"uuid,omitempty"`    // Explicit UUID override
	Devices  map[string]LegacyDeviceConfig   `json:"devices,omitempty"` // Saved device info (keyed by model name)
	InfluxDB *LegacyInfluxDBConfig           `json:"influxdb,omitempty"`
	Daemon   *LegacyDaemonConfig             `json:"daemon,omitempty"`
}

// GetInterval parses the daemon interval duration
func (c *LegacyConfig) GetInterval() time.Duration {
	if c.Daemon == nil || c.Daemon.Interval == "" {
		return 3 * time.Hour
	}
	d, err := time.ParseDuration(c.Daemon.Interval)
	if err != nil {
		return 3 * time.Hour
	}
	return d
}

// configPath returns the path to the config file
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omblego", "config.json"), nil
}

// LoadLegacy reads the legacy JSON configuration from the default path
func LoadLegacy() (*LegacyConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return LoadLegacyFromFile(path)
}

// LoadLegacyFromFile reads the legacy JSON configuration from a specific file
func LoadLegacyFromFile(path string) (*LegacyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return &LegacyConfig{Devices: make(map[string]LegacyDeviceConfig)}, nil
		}
		return nil, err
	}

	var cfg LegacyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Devices == nil {
		cfg.Devices = make(map[string]LegacyDeviceConfig)
	}

	return &cfg, nil
}

// SaveLegacy writes the legacy configuration to disk
func SaveLegacy(cfg *LegacyConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetDeviceUUID retrieves the saved UUID for a device model (legacy)
func GetDeviceUUID(deviceName string) (string, error) {
	cfg, err := LoadLegacy()
	if err != nil {
		return "", err
	}

	dev, ok := cfg.Devices[deviceName]
	if !ok {
		return "", nil
	}

	return dev.UUID, nil
}

// SaveDeviceUUID saves the UUID for a device model (legacy)
func SaveDeviceUUID(deviceName, uuid, mac string) error {
	cfg, err := LoadLegacy()
	if err != nil {
		return err
	}

	cfg.Devices[deviceName] = LegacyDeviceConfig{
		UUID: uuid,
		MAC:  mac,
		Name: deviceName,
	}

	return SaveLegacy(cfg)
}
