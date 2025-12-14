package commands

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"omblego/internal/config"
)

var (
	migrateInputFile  string
	migrateOutputFile string
)

var migrateCmd = &cobra.Command{
	Use:   "migrate-config",
	Short: "Migrate legacy JSON config to YAML format",
	Long: `Converts a legacy JSON configuration file to the new YAML format.

The legacy JSON format stored device settings and InfluxDB configuration.
The new YAML format supports multiple output backends and more options.

Examples:
  # Auto-detect and migrate
  omblego migrate-config

  # Specify input and output files
  omblego migrate-config --input old-config.json --output ~/.config/omblego/config.yaml`,
	RunE: runMigrate,
}

func init() {
	migrateCmd.Flags().StringVar(&migrateInputFile, "input", "", "Legacy JSON config file")
	migrateCmd.Flags().StringVar(&migrateOutputFile, "output", "", "Output YAML file (default: ~/.config/omblego/config.yaml)")

	AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	// Find input file if not specified
	if migrateInputFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		// Try common locations for legacy config
		candidates := []string{
			filepath.Join(home, ".omblego", "config.json"),
			filepath.Join(home, ".config", "omblego", "config.json"),
			"omblego.json",
			"config.json",
		}

		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				migrateInputFile = path
				break
			}
		}

		if migrateInputFile == "" {
			return fmt.Errorf("no legacy config file found. Specify --input to provide the path")
		}
	}

	// Set default output file if not specified
	if migrateOutputFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		migrateOutputFile = filepath.Join(home, ".config", "omblego", "config.yaml")
	}

	// Check if output file already exists
	if _, err := os.Stat(migrateOutputFile); err == nil {
		return fmt.Errorf("output file already exists: %s (use --output to specify a different file)", migrateOutputFile)
	}

	slog.Info("migrating config", "from", migrateInputFile, "to", migrateOutputFile)

	// Load legacy config
	legacyCfg, err := config.LoadLegacyFromFile(migrateInputFile)
	if err != nil {
		return fmt.Errorf("failed to load legacy config: %w", err)
	}

	// Convert to new format
	newCfg := convertLegacyConfig(legacyCfg)

	// Ensure output directory exists
	outputDir := filepath.Dir(migrateOutputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write new config
	if err := config.Save(newCfg, migrateOutputFile); err != nil {
		return fmt.Errorf("failed to write new config: %w", err)
	}

	slog.Info("migration complete", "output", migrateOutputFile)
	fmt.Printf("\nMigration complete!\n")
	fmt.Printf("New config written to: %s\n\n", migrateOutputFile)
	fmt.Println("You can now use the new config with:")
	fmt.Printf("  omblego sync -d %s --config %s\n", newCfg.Device.Model, migrateOutputFile)

	return nil
}

func convertLegacyConfig(legacy *config.LegacyConfig) *config.Config {
	cfg := &config.Config{
		Device: config.DeviceSettings{
			Model: legacy.Device,
			UUID:  legacy.UUID,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stderr",
		},
	}

	// Convert InfluxDB settings
	if legacy.InfluxDB != nil {
		cfg.Outputs = append(cfg.Outputs, config.OutputConfig{
			Type:    "influxdb",
			Enabled: true,
			Settings: map[string]interface{}{
				"url":    legacy.InfluxDB.URL,
				"token":  legacy.InfluxDB.Token,
				"org":    legacy.InfluxDB.Org,
				"bucket": legacy.InfluxDB.Bucket,
			},
		})
	}

	// Convert daemon settings
	if legacy.Daemon != nil {
		cfg.Daemon = config.DaemonConfig{
			Enabled:  true,
			Interval: legacy.GetInterval(),
			TimeSync: legacy.Daemon.TimeSync,
		}
	}

	// Add default CSV output
	cfg.Outputs = append(cfg.Outputs, config.OutputConfig{
		Type:    "csv",
		Enabled: true,
		Settings: map[string]interface{}{
			"directory":    "~/.omblego/data",
			"file_pattern": "user{user}.csv",
		},
	})

	return cfg
}
