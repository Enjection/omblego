package output

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// JSONBackendConfig holds configuration for the JSON backend
type JSONBackendConfig struct {
	Directory string // Directory to store JSON files
	Filename  string // Output filename (default: ubpm.json)
}

// JSONBackend implements the Backend interface for JSON file output
type JSONBackend struct {
	name   string
	config JSONBackendConfig
}

// NewJSONBackend creates a new JSON backend
func NewJSONBackend(name string, config JSONBackendConfig) (*JSONBackend, error) {
	if config.Directory == "" {
		config.Directory = "."
	}
	if config.Filename == "" {
		config.Filename = "ubpm.json"
	}

	// Expand ~ to home directory
	if strings.HasPrefix(config.Directory, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		config.Directory = filepath.Join(home, config.Directory[2:])
	}

	// Ensure directory exists
	if err := os.MkdirAll(config.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &JSONBackend{
		name:   name,
		config: config,
	}, nil
}

// Name returns the backend instance name
func (b *JSONBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *JSONBackend) Type() string {
	return "json"
}

// Write writes records to a JSON file
func (b *JSONBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	path := filepath.Join(b.config.Directory, b.config.Filename)
	return WriteJSONFile(path, userRecords)
}

// Health checks if the backend is operational
func (b *JSONBackend) Health(ctx context.Context) error {
	// Check if directory is writable
	testFile := filepath.Join(b.config.Directory, ".health_check")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// Close releases resources (no-op for JSON)
func (b *JSONBackend) Close() error {
	return nil
}

func init() {
	Register("json", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		directory, _ := settings["directory"].(string)
		filename, _ := settings["filename"].(string)

		return NewJSONBackend(cfg.Name, JSONBackendConfig{
			Directory: directory,
			Filename:  filename,
		})
	})
}
