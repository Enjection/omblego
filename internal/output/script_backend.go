package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ScriptBackendConfig holds configuration for the Script backend
type ScriptBackendConfig struct {
	Command     string            // Command to execute
	Args        []string          // Command arguments
	WorkingDir  string            // Working directory
	Environment map[string]string // Additional environment variables
	Timeout     time.Duration     // Execution timeout
	Format      string            // Input format: json (default), csv, env
}

// ScriptBackend implements the Backend interface for script execution
type ScriptBackend struct {
	name   string
	config ScriptBackendConfig
}

// ScriptInput is the JSON structure passed to the script
type ScriptInput struct {
	DeviceName    string           `json:"device_name"`
	SyncTimestamp time.Time        `json:"sync_timestamp"`
	IsFullSync    bool             `json:"is_full_sync"`
	SessionID     string           `json:"session_id"`
	Users         []ScriptUserData `json:"users"`
	TotalRecords  int              `json:"total_records"`
}

// ScriptUserData represents records for a single user
type ScriptUserData struct {
	UserID  int            `json:"user_id"`
	Records []ScriptRecord `json:"records"`
}

// ScriptRecord represents a single blood pressure record
type ScriptRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Systolic  int       `json:"systolic"`
	Diastolic int       `json:"diastolic"`
	Pulse     int       `json:"pulse"`
	Movement  bool      `json:"movement"`
	IHB       bool      `json:"ihb"`
}

// NewScriptBackend creates a new Script backend
func NewScriptBackend(name string, config ScriptBackendConfig) (*ScriptBackend, error) {
	if config.Command == "" {
		return nil, fmt.Errorf("script command is required")
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.Format == "" {
		config.Format = "json"
	}

	// Validate format
	switch config.Format {
	case "json", "csv", "env":
		// Valid formats
	default:
		return nil, fmt.Errorf("invalid format: %s (must be json, csv, or env)", config.Format)
	}

	return &ScriptBackend{
		name:   name,
		config: config,
	}, nil
}

// Name returns the backend instance name
func (b *ScriptBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *ScriptBackend) Type() string {
	return "script"
}

// Write executes the script with records as input
func (b *ScriptBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	input, err := b.formatInput(userRecords, metadata)
	if err != nil {
		return fmt.Errorf("failed to format input: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, b.config.Command, b.config.Args...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = b.buildEnv(metadata)

	if b.config.WorkingDir != "" {
		cmd.Dir = b.config.WorkingDir
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("script execution timed out")
		}
		return fmt.Errorf("script execution failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// formatInput formats the records in the configured format
func (b *ScriptBackend) formatInput(userRecords [][]Record, metadata Metadata) ([]byte, error) {
	switch b.config.Format {
	case "json":
		return b.formatJSON(userRecords, metadata)
	case "csv":
		return b.formatCSV(userRecords)
	case "env":
		return b.formatEnv(userRecords, metadata)
	default:
		return b.formatJSON(userRecords, metadata)
	}
}

// formatJSON formats records as JSON
func (b *ScriptBackend) formatJSON(userRecords [][]Record, metadata Metadata) ([]byte, error) {
	input := ScriptInput{
		DeviceName:    metadata.DeviceName,
		SyncTimestamp: metadata.SyncTimestamp,
		IsFullSync:    metadata.IsFullSync,
		SessionID:     metadata.SessionID,
		Users:         make([]ScriptUserData, 0, len(userRecords)),
	}

	for userIdx, records := range userRecords {
		if len(records) == 0 {
			continue
		}

		userData := ScriptUserData{
			UserID:  userIdx + 1,
			Records: make([]ScriptRecord, len(records)),
		}

		for i, rec := range records {
			userData.Records[i] = ScriptRecord{
				Timestamp: rec.Timestamp,
				Systolic:  rec.Systolic,
				Diastolic: rec.Diastolic,
				Pulse:     rec.Pulse,
				Movement:  rec.Movement,
				IHB:       rec.IHB,
			}
			input.TotalRecords++
		}

		input.Users = append(input.Users, userData)
	}

	return json.Marshal(input)
}

// formatCSV formats records as CSV
func (b *ScriptBackend) formatCSV(userRecords [][]Record) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("user,datetime,systolic,diastolic,pulse,movement,ihb\n")

	for userIdx, records := range userRecords {
		for _, rec := range records {
			mov := 0
			if rec.Movement {
				mov = 1
			}
			ihb := 0
			if rec.IHB {
				ihb = 1
			}
			fmt.Fprintf(&buf, "%d,%s,%d,%d,%d,%d,%d\n",
				userIdx+1,
				rec.Timestamp.Format("2006-01-02 15:04:05"),
				rec.Systolic,
				rec.Diastolic,
				rec.Pulse,
				mov,
				ihb,
			)
		}
	}

	return buf.Bytes(), nil
}

// formatEnv formats the latest record as environment-style key=value pairs
func (b *ScriptBackend) formatEnv(userRecords [][]Record, metadata Metadata) ([]byte, error) {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "OMBLEGO_DEVICE=%s\n", metadata.DeviceName)
	fmt.Fprintf(&buf, "OMBLEGO_SYNC_TIME=%s\n", metadata.SyncTimestamp.Format(time.RFC3339))
	fmt.Fprintf(&buf, "OMBLEGO_FULL_SYNC=%t\n", metadata.IsFullSync)

	// Find and output the latest record
	var latestRec *Record
	var latestUser int

	for userIdx, records := range userRecords {
		for i := range records {
			if latestRec == nil || records[i].Timestamp.After(latestRec.Timestamp) {
				latestRec = &records[i]
				latestUser = userIdx + 1
			}
		}
	}

	if latestRec != nil {
		fmt.Fprintf(&buf, "OMBLEGO_USER=%d\n", latestUser)
		fmt.Fprintf(&buf, "OMBLEGO_TIMESTAMP=%s\n", latestRec.Timestamp.Format(time.RFC3339))
		fmt.Fprintf(&buf, "OMBLEGO_SYSTOLIC=%d\n", latestRec.Systolic)
		fmt.Fprintf(&buf, "OMBLEGO_DIASTOLIC=%d\n", latestRec.Diastolic)
		fmt.Fprintf(&buf, "OMBLEGO_PULSE=%d\n", latestRec.Pulse)
		fmt.Fprintf(&buf, "OMBLEGO_MOVEMENT=%t\n", latestRec.Movement)
		fmt.Fprintf(&buf, "OMBLEGO_IHB=%t\n", latestRec.IHB)
	}

	return buf.Bytes(), nil
}

// buildEnv builds the environment for the script
func (b *ScriptBackend) buildEnv(metadata Metadata) []string {
	env := os.Environ()

	// Add metadata as environment variables
	env = append(env,
		fmt.Sprintf("OMBLEGO_DEVICE=%s", metadata.DeviceName),
		fmt.Sprintf("OMBLEGO_SYNC_TIME=%s", metadata.SyncTimestamp.Format(time.RFC3339)),
		fmt.Sprintf("OMBLEGO_FULL_SYNC=%t", metadata.IsFullSync),
		fmt.Sprintf("OMBLEGO_SESSION_ID=%s", metadata.SessionID),
	)

	// Add custom environment variables
	for key, value := range b.config.Environment {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// Health checks if the script is executable
func (b *ScriptBackend) Health(ctx context.Context) error {
	// Check if command exists
	path, err := exec.LookPath(b.config.Command)
	if err != nil {
		return fmt.Errorf("command not found: %s", b.config.Command)
	}

	// Check if it's executable
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat command: %w", err)
	}

	if info.Mode()&0111 == 0 {
		return fmt.Errorf("command is not executable: %s", path)
	}

	return nil
}

// Close releases resources (no-op for script)
func (b *ScriptBackend) Close() error {
	return nil
}

func init() {
	Register("script", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		command, _ := settings["command"].(string)
		format, _ := settings["format"].(string)
		workingDir, _ := settings["working_dir"].(string)

		var args []string
		if a, ok := settings["args"].([]interface{}); ok {
			for _, arg := range a {
				if s, ok := arg.(string); ok {
					args = append(args, s)
				}
			}
		}
		// Also support args as space-separated string
		if argsStr, ok := settings["args"].(string); ok {
			args = strings.Fields(argsStr)
		}

		env := make(map[string]string)
		if e, ok := settings["environment"].(map[string]interface{}); ok {
			for k, v := range e {
				if s, ok := v.(string); ok {
					env[k] = s
				}
			}
		}

		var timeout time.Duration
		if t, ok := settings["timeout"].(string); ok {
			timeout, _ = time.ParseDuration(t)
		}

		return NewScriptBackend(cfg.Name, ScriptBackendConfig{
			Command:     command,
			Args:        args,
			WorkingDir:  workingDir,
			Environment: env,
			Timeout:     timeout,
			Format:      format,
		})
	})
}
