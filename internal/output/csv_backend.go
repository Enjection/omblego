package output

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CSVBackendConfig holds configuration for the CSV backend
type CSVBackendConfig struct {
	Directory   string // Directory to store CSV files
	FilePattern string // File naming pattern (e.g., "user{user}.csv")
	CreateBackup bool  // Whether to create backups before overwriting
}

// CSVBackend implements the Backend interface for CSV file output
type CSVBackend struct {
	name   string
	config CSVBackendConfig
}

// NewCSVBackend creates a new CSV backend
func NewCSVBackend(name string, config CSVBackendConfig) (*CSVBackend, error) {
	if config.Directory == "" {
		config.Directory = "."
	}
	if config.FilePattern == "" {
		config.FilePattern = "user{user}.csv"
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

	return &CSVBackend{
		name:   name,
		config: config,
	}, nil
}

// Name returns the backend instance name
func (b *CSVBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *CSVBackend) Type() string {
	return "csv"
}

// Write writes records to CSV files
func (b *CSVBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	for userIdx, records := range userRecords {
		if len(records) == 0 {
			continue
		}

		// Generate filename for this user
		filename := b.filenameForUser(userIdx + 1)
		filepath := filepath.Join(b.config.Directory, filename)

		// Use existing merge logic
		if err := b.writeUserRecords(filepath, userIdx+1, records); err != nil {
			return fmt.Errorf("failed to write CSV for user %d: %w", userIdx+1, err)
		}
	}
	return nil
}

// writeUserRecords writes a single user's records, merging with existing data
func (b *CSVBackend) writeUserRecords(path string, userNum int, newRecords []Record) error {
	// Read existing records if file exists
	existingRecords, err := readCSVFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing CSV: %w", err)
	}

	// Create backup if enabled and file exists
	if b.config.CreateBackup && len(existingRecords) > 0 {
		backupName := fmt.Sprintf("backup_%s", filepath.Base(path))
		backupPath := filepath.Join(filepath.Dir(path), backupName)
		if err := copyFile(path, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Merge and sort records
	mergedRecords := mergeRecords(existingRecords, newRecords)

	return writeCSVFile(path, mergedRecords)
}

// filenameForUser generates a filename for a given user number
func (b *CSVBackend) filenameForUser(userNum int) string {
	return strings.ReplaceAll(b.config.FilePattern, "{user}", fmt.Sprintf("%d", userNum))
}

// Health checks if the backend is operational
func (b *CSVBackend) Health(ctx context.Context) error {
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

// Close releases resources (no-op for CSV)
func (b *CSVBackend) Close() error {
	return nil
}

func init() {
	Register("csv", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		directory, _ := settings["directory"].(string)
		filePattern, _ := settings["file_pattern"].(string)
		createBackup, _ := settings["create_backup"].(bool)

		return NewCSVBackend(cfg.Name, CSVBackendConfig{
			Directory:    directory,
			FilePattern:  filePattern,
			CreateBackup: createBackup,
		})
	})
}
