// Package commands implements the CLI commands for omblego.
package commands

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"omblego/internal/config"
)

var (
	// Global flags
	configFile string
	debugMode  bool
	logLevel   string
	logFormat  string
	logOutput  string
)

// Version is set at build time
var Version = "dev"

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "omblego",
	Short: "Read records from Omron BLE blood pressure monitors",
	Long: `omblego is a CLI tool to read blood pressure measurement records
from Omron Bluetooth Low Energy (BLE) medical devices.

Supported devices: HEM-7322T, HEM-7600T, HEM-6232T, HEM-7530T,
                   HEM-7155T, HEM-7150T, HEM-7342T, HEM-7361T,
                   BP7000, BP5250, EVOLV

Examples:
  # First-time pairing (device must show -P- on display)
  omblego pair -d BP7000

  # Sync records after pairing
  omblego sync -d BP7000

  # Run as daemon (syncs on interval)
  omblego daemon -d BP7000 --interval 3h

  # List supported devices
  omblego list-devices

  # Scan for nearby devices
  omblego scan`,
	SilenceUsage: true,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "",
		fmt.Sprintf("Config file path\n  Search order: 1) explicit path 2) %s 3) %s",
			config.DefaultUserConfigPath(), config.DefaultSystemConfigPath()))
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error, off")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "Log format: text, json")
	rootCmd.PersistentFlags().StringVar(&logOutput, "log-output", "stderr", "Log output: stderr, stdout, off")
}

// initConfig initializes the configuration
func initConfig() {
	// Set up logging based on flags
	setupLogging()

	// Load config file if specified
	if configFile != "" {
		cfg, err := config.Load(configFile)
		if err != nil {
			slog.Warn("failed to load config file", "path", configFile, "error", err)
		} else {
			// Apply logging settings from config (CLI flags override)
			if logLevel == "info" && cfg.Logging.Level != "" {
				logLevel = cfg.Logging.Level
			}
			if logFormat == "text" && cfg.Logging.Format != "" {
				logFormat = cfg.Logging.Format
			}
			if logOutput == "stderr" && cfg.Logging.Output != "" {
				logOutput = cfg.Logging.Output
			}
			setupLogging() // Re-setup with config values
		}
	}
}

// setupLogging configures the slog logger based on flags
func setupLogging() {
	// Handle log level
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "off":
		// Set to max level to disable most logging
		level = slog.Level(100)
	default:
		level = slog.LevelInfo
	}

	// Handle output destination
	var output *os.File
	switch strings.ToLower(logOutput) {
	case "stdout":
		output = os.Stdout
	case "off":
		output = nil
	default:
		output = os.Stderr
	}

	if output == nil {
		// Discard all logs
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.Level(100)})))
		return
	}

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	switch strings.ToLower(logFormat) {
	case "json":
		handler = slog.NewJSONHandler(output, opts)
	default:
		handler = slog.NewTextHandler(output, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// GetLogger returns the configured logger
func GetLogger() *slog.Logger {
	return slog.Default()
}

// GetConfigFile returns the config file path
func GetConfigFile() string {
	return configFile
}

// IsDebugMode returns whether debug mode is enabled
func IsDebugMode() bool {
	return debugMode
}

// AddCommand adds a subcommand to the root command
func AddCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

// printError prints an error message to stderr
func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}
