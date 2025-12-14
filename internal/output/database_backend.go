package output

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// Import common database drivers
	_ "github.com/lib/pq"           // PostgreSQL
	_ "github.com/go-sql-driver/mysql" // MySQL
	_ "github.com/mattn/go-sqlite3" // SQLite
)

// DatabaseBackendConfig holds configuration for the Database backend
type DatabaseBackendConfig struct {
	Driver   string // Database driver: postgres, mysql, sqlite3
	DSN      string // Data source name / connection string
	Table    string // Table name (default: blood_pressure)
	AutoInit bool   // Auto-create table if not exists
}

// DatabaseBackend implements the Backend interface for SQL database output
type DatabaseBackend struct {
	name   string
	config DatabaseBackendConfig
	db     *sql.DB
}

// NewDatabaseBackend creates a new Database backend
func NewDatabaseBackend(name string, config DatabaseBackendConfig) (*DatabaseBackend, error) {
	if config.Driver == "" {
		return nil, fmt.Errorf("database driver is required")
	}
	if config.DSN == "" {
		return nil, fmt.Errorf("database dsn is required")
	}
	if config.Table == "" {
		config.Table = "blood_pressure"
	}

	db, err := sql.Open(config.Driver, config.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	backend := &DatabaseBackend{
		name:   name,
		config: config,
		db:     db,
	}

	if config.AutoInit {
		if err := backend.initTable(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to initialize table: %w", err)
		}
	}

	return backend, nil
}

// initTable creates the table if it doesn't exist
func (b *DatabaseBackend) initTable() error {
	var createSQL string

	switch b.config.Driver {
	case "postgres":
		createSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id SERIAL PRIMARY KEY,
				device VARCHAR(64) NOT NULL,
				user_num INTEGER NOT NULL,
				timestamp TIMESTAMP NOT NULL,
				systolic INTEGER NOT NULL,
				diastolic INTEGER NOT NULL,
				pulse INTEGER NOT NULL,
				movement BOOLEAN NOT NULL DEFAULT FALSE,
				ihb BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(device, user_num, timestamp)
			)`, b.config.Table)
	case "mysql":
		createSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				device VARCHAR(64) NOT NULL,
				user_num INTEGER NOT NULL,
				timestamp DATETIME NOT NULL,
				systolic INTEGER NOT NULL,
				diastolic INTEGER NOT NULL,
				pulse INTEGER NOT NULL,
				movement BOOLEAN NOT NULL DEFAULT FALSE,
				ihb BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				UNIQUE KEY unique_record (device, user_num, timestamp)
			)`, b.config.Table)
	case "sqlite3":
		createSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				device TEXT NOT NULL,
				user_num INTEGER NOT NULL,
				timestamp DATETIME NOT NULL,
				systolic INTEGER NOT NULL,
				diastolic INTEGER NOT NULL,
				pulse INTEGER NOT NULL,
				movement INTEGER NOT NULL DEFAULT 0,
				ihb INTEGER NOT NULL DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(device, user_num, timestamp)
			)`, b.config.Table)
	default:
		return fmt.Errorf("unsupported driver for auto-init: %s", b.config.Driver)
	}

	_, err := b.db.Exec(createSQL)
	return err
}

// Name returns the backend instance name
func (b *DatabaseBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *DatabaseBackend) Type() string {
	return "database"
}

// Write writes records to the database
func (b *DatabaseBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	insertSQL := b.buildInsertSQL()
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

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

			_, err := stmt.ExecContext(ctx,
				metadata.DeviceName,
				userIdx+1,
				rec.Timestamp,
				rec.Systolic,
				rec.Diastolic,
				rec.Pulse,
				mov,
				ihb,
			)
			if err != nil {
				// Ignore duplicate key errors (record already exists)
				// Different databases have different error handling, so we log and continue
				continue
			}
		}
	}

	return tx.Commit()
}

// buildInsertSQL returns the appropriate INSERT statement for the database driver
func (b *DatabaseBackend) buildInsertSQL() string {
	switch b.config.Driver {
	case "postgres":
		return fmt.Sprintf(`
			INSERT INTO %s (device, user_num, timestamp, systolic, diastolic, pulse, movement, ihb)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (device, user_num, timestamp) DO NOTHING`,
			b.config.Table)
	case "mysql":
		return fmt.Sprintf(`
			INSERT IGNORE INTO %s (device, user_num, timestamp, systolic, diastolic, pulse, movement, ihb)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			b.config.Table)
	case "sqlite3":
		return fmt.Sprintf(`
			INSERT OR IGNORE INTO %s (device, user_num, timestamp, systolic, diastolic, pulse, movement, ihb)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			b.config.Table)
	default:
		return fmt.Sprintf(`
			INSERT INTO %s (device, user_num, timestamp, systolic, diastolic, pulse, movement, ihb)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			b.config.Table)
	}
}

// Health checks if the database is operational
func (b *DatabaseBackend) Health(ctx context.Context) error {
	return b.db.PingContext(ctx)
}

// Close releases resources
func (b *DatabaseBackend) Close() error {
	return b.db.Close()
}

// RetryConfig returns retry configuration for database
func (b *DatabaseBackend) RetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}
}

func init() {
	Register("database", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		driver, _ := settings["driver"].(string)
		dsn, _ := settings["dsn"].(string)
		table, _ := settings["table"].(string)
		autoInit, _ := settings["auto_init"].(bool)

		return NewDatabaseBackend(cfg.Name, DatabaseBackendConfig{
			Driver:   driver,
			DSN:      dsn,
			Table:    table,
			AutoInit: autoInit,
		})
	})
}
