// Package config provides configuration loading and management for omblego.
// Configuration can be loaded from YAML files, environment variables, and CLI flags.
package config

import (
	"time"
)

// Config is the root configuration structure
type Config struct {
	Device  DeviceSettings  `koanf:"device"`
	Outputs []OutputConfig  `koanf:"outputs"`
	Daemon  DaemonConfig    `koanf:"daemon"`
	Logging LoggingConfig   `koanf:"logging"`
}

// DeviceSettings configures the target BLE device
type DeviceSettings struct {
	Model string `koanf:"model"` // Device model name (e.g., "BP7000", "HEM-7322T")
	UUID  string `koanf:"uuid"`  // "auto" = auto-capture, saves on first connect; or explicit UUID
	MAC   string `koanf:"mac"`   // Explicit MAC address (optional)
}

// OutputConfig is a polymorphic output configuration
type OutputConfig struct {
	Type     string                 `koanf:"type"`     // Backend type: "csv", "json", "influxdb", etc.
	Enabled  bool                   `koanf:"enabled"`  // Whether this backend is active
	Name     string                 `koanf:"name"`     // Optional instance name (defaults to type)
	Settings map[string]interface{} `koanf:"settings"` // Type-specific settings
}

// DaemonConfig controls daemon/service mode
type DaemonConfig struct {
	Enabled   bool          `koanf:"enabled"`
	Interval  time.Duration `koanf:"interval"`   // Sync interval (e.g., "3h", "30m")
	TimeSync  bool          `koanf:"time_sync"`  // Sync device clock on each sync
	OnFailure string        `koanf:"on_failure"` // "continue", "backoff", "exit"
}

// LoggingConfig controls logging behavior
type LoggingConfig struct {
	Level  string `koanf:"level"`  // "debug", "info", "warn", "error", "off"
	Format string `koanf:"format"` // "text", "json"
	Output string `koanf:"output"` // "stderr", "stdout", "file", "off"
	File   string `koanf:"file"`   // File path when output=file
}

// CSVOutputSettings configures the CSV output backend
type CSVOutputSettings struct {
	Directory     string `koanf:"directory"`
	FilePattern   string `koanf:"file_pattern"`   // e.g., "user{user}.csv"
	CreateBackups bool   `koanf:"create_backups"`
}

// JSONOutputSettings configures the JSON output backend
type JSONOutputSettings struct {
	FilePath string `koanf:"file_path"`
	Format   string `koanf:"format"` // "ubpm", "raw"
	Pretty   bool   `koanf:"pretty"`
}

// InfluxDBOutputSettings configures the InfluxDB output backend
type InfluxDBOutputSettings struct {
	URL         string            `koanf:"url"`
	Token       string            `koanf:"token"`
	Org         string            `koanf:"org"`
	Bucket      string            `koanf:"bucket"`
	Measurement string            `koanf:"measurement"`
	Tags        map[string]string `koanf:"tags"`
	BatchSize   int               `koanf:"batch_size"`
	RetryCount  int               `koanf:"retry_count"`
}

// WebhookOutputSettings configures the webhook output backend
type WebhookOutputSettings struct {
	URL        string            `koanf:"url"`
	Method     string            `koanf:"method"` // "POST", "PUT"
	Headers    map[string]string `koanf:"headers"`
	Format     string            `koanf:"format"` // "json", "form"
	Timeout    time.Duration     `koanf:"timeout"`
	RetryCount int               `koanf:"retry_count"`
	RetryDelay time.Duration     `koanf:"retry_delay"`
}

// DatabaseOutputSettings configures the SQL database output backend
type DatabaseOutputSettings struct {
	Driver     string `koanf:"driver"` // "postgres", "mysql", "sqlite"
	DSN        string `koanf:"dsn"`
	Table      string `koanf:"table"`
	BatchSize  int    `koanf:"batch_size"`
	RetryCount int    `koanf:"retry_count"`
}

// ScriptOutputSettings configures the script execution output backend
type ScriptOutputSettings struct {
	Command   string            `koanf:"command"`
	Args      []string          `koanf:"args"`
	Env       map[string]string `koanf:"env"`
	Timeout   time.Duration     `koanf:"timeout"`
	InputMode string            `koanf:"input_mode"` // "stdin", "args", "env"
}

// PrometheusOutputSettings configures the Prometheus metrics backend
type PrometheusOutputSettings struct {
	Port int    `koanf:"port"`
	Path string `koanf:"path"` // Metrics endpoint path (default: /metrics)
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Device: DeviceSettings{
			UUID: "auto",
		},
		Daemon: DaemonConfig{
			Enabled:   false,
			Interval:  3 * time.Hour,
			TimeSync:  true,
			OnFailure: "backoff",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stderr",
		},
		Outputs: []OutputConfig{
			{
				Type:    "csv",
				Enabled: true,
				Settings: map[string]interface{}{
					"directory":      ".",
					"file_pattern":   "user{user}.csv",
					"create_backups": true,
				},
			},
		},
	}
}
