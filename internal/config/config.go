// Package config handles persistent configuration for omblego
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DeviceConfig stores saved device information
type DeviceConfig struct {
	UUID string `json:"uuid"` // BLE peripheral UUID
	MAC  string `json:"mac"`  // MAC address (if available)
	Name string `json:"name"` // Device name (e.g., "BP7000")
}

// InfluxDBConfig stores InfluxDB connection settings
type InfluxDBConfig struct {
	URL    string `json:"url"`
	Token  string `json:"token"`
	Org    string `json:"org"`
	Bucket string `json:"bucket"`
}

// DaemonConfig stores daemon mode settings
type DaemonConfig struct {
	Interval string `json:"interval"` // e.g., "3h", "30m"
	TimeSync bool   `json:"time_sync"`
}

// Config is the main configuration structure
type Config struct {
	Device   string                   `json:"device"`            // Default device name
	UUID     string                   `json:"uuid,omitempty"`    // Explicit UUID override
	Devices  map[string]DeviceConfig  `json:"devices,omitempty"` // Saved device info (keyed by model name)
	InfluxDB *InfluxDBConfig          `json:"influxdb,omitempty"`
	Daemon   *DaemonConfig            `json:"daemon,omitempty"`
}

// GetInterval parses the daemon interval duration
func (c *Config) GetInterval() time.Duration {
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

// Load reads the configuration from the default path
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return LoadFromFile(path)
}

// LoadFromFile reads the configuration from a specific file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return &Config{Devices: make(map[string]DeviceConfig)}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Devices == nil {
		cfg.Devices = make(map[string]DeviceConfig)
	}

	return &cfg, nil
}

// Save writes the configuration to disk
func Save(cfg *Config) error {
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

// GetDeviceUUID retrieves the saved UUID for a device model
func GetDeviceUUID(deviceName string) (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}

	dev, ok := cfg.Devices[deviceName]
	if !ok {
		return "", nil
	}

	return dev.UUID, nil
}

// SaveDeviceUUID saves the UUID for a device model
func SaveDeviceUUID(deviceName, uuid, mac string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.Devices[deviceName] = DeviceConfig{
		UUID: uuid,
		MAC:  mac,
		Name: deviceName,
	}

	return Save(cfg)
}
