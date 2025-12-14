package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Loader handles configuration loading from multiple sources
type Loader struct {
	k *koanf.Koanf
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	return &Loader{
		k: koanf.New("."),
	}
}

// Load loads configuration with the following precedence (highest to lowest):
// 1. Environment variables (OMBLEGO_*)
// 2. Config file (explicit path or default locations)
// 3. Default values
//
// CLI flags should be applied after Load() by calling ApplyOverrides.
func (l *Loader) Load(configPath string) (*Config, error) {
	// 1. Load defaults
	defaults := DefaultConfig()
	if err := l.k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// 2. Load config file
	path := configPath
	if path == "" {
		path = l.findConfigFile()
	}
	if path != "" {
		if err := l.k.Load(file.Provider(path), yaml.Parser()); err != nil {
			// Only error if explicit path was provided
			if configPath != "" {
				return nil, fmt.Errorf("failed to load config file %s: %w", path, err)
			}
			// Silently ignore if auto-discovered file fails
		}
	}

	// 3. Load environment variables
	// OMBLEGO_DEVICE_MODEL -> device.model
	// OMBLEGO_LOGGING_LEVEL -> logging.level
	if err := l.k.Load(env.Provider("OMBLEGO_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "OMBLEGO_")), "_", ".", -1)
	}), nil); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// 4. Unmarshal to struct
	var cfg Config
	if err := l.k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// LoadFromFile loads configuration from a specific file path
func LoadFromFile(path string) (*Config, error) {
	loader := NewLoader()
	return loader.Load(path)
}

// LoadDefault loads configuration using default file locations and environment
func LoadDefault() (*Config, error) {
	loader := NewLoader()
	return loader.Load("")
}

// findConfigFile searches for config in standard locations
func (l *Loader) findConfigFile() string {
	locations := []string{
		"./omblego.yaml",
		"./omblego.yml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		locations = append(locations,
			filepath.Join(home, ".config", "omblego", "config.yaml"),
			filepath.Join(home, ".config", "omblego", "config.yml"),
			filepath.Join(home, ".omblego.yaml"),
			filepath.Join(home, ".omblego.yml"),
		)
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}
	return ""
}

// GetRaw returns the raw koanf instance for advanced usage
func (l *Loader) GetRaw() *koanf.Koanf {
	return l.k
}

// Set sets a configuration value by key path
func (l *Loader) Set(key string, value interface{}) error {
	return l.k.Set(key, value)
}

// Get retrieves a configuration value by key path
func (l *Loader) Get(key string) interface{} {
	return l.k.Get(key)
}

// Unmarshal unmarshals the configuration into a struct
func (l *Loader) Unmarshal(path string, o interface{}) error {
	return l.k.Unmarshal(path, o)
}

// ConfigPaths returns the default config file search paths
func ConfigPaths() []string {
	paths := []string{
		"./omblego.yaml",
		"./omblego.yml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".config", "omblego", "config.yaml"),
			filepath.Join(home, ".config", "omblego", "config.yml"),
		)
	}

	return paths
}

// Load is a convenience function that loads config from a file path.
// If path is empty, it searches default locations.
func Load(path string) (*Config, error) {
	if path == "" {
		return LoadDefault()
	}
	return LoadFromFile(path)
}

// Save saves the configuration to a YAML file
func Save(cfg *Config, path string) error {
	data, err := yaml.Parser().Marshal(map[string]interface{}{
		"device":  cfg.Device,
		"daemon":  cfg.Daemon,
		"logging": cfg.Logging,
		"outputs": cfg.Outputs,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
