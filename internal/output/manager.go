package output

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager coordinates multiple output backends
type Manager struct {
	backends []Backend
	logger   *slog.Logger
	mu       sync.RWMutex
}

// ManagerOption configures a Manager
type ManagerOption func(*Manager)

// WithLogger sets the logger for the manager
func WithLogger(logger *slog.Logger) ManagerOption {
	return func(m *Manager) {
		m.logger = logger
	}
}

// NewManager creates a manager from backend configurations.
// Only enabled backends are created.
func NewManager(configs []BackendConfig, opts ...ManagerOption) (*Manager, error) {
	m := &Manager{
		backends: make([]Backend, 0, len(configs)),
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(m)
	}

	for _, cfg := range configs {
		if !cfg.Enabled {
			m.logger.Debug("skipping disabled backend", "type", cfg.Type, "name", cfg.Name)
			continue
		}

		backend, err := Create(cfg)
		if err != nil {
			// Close any already-created backends
			m.Close()
			return nil, fmt.Errorf("failed to create backend %q: %w", cfg.Name, err)
		}
		m.backends = append(m.backends, backend)
		m.logger.Debug("created backend", "type", cfg.Type, "name", backend.Name())
	}

	return m, nil
}

// NewManagerWithBackends creates a manager with pre-created backends
func NewManagerWithBackends(backends []Backend, opts ...ManagerOption) *Manager {
	m := &Manager{
		backends: backends,
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Write writes to all backends, collecting results.
// All backends are written to in parallel.
func (m *Manager) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) []WriteResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.backends) == 0 {
		return nil
	}

	results := make([]WriteResult, len(m.backends))
	var wg sync.WaitGroup

	for i, backend := range m.backends {
		wg.Add(1)
		go func(idx int, b Backend) {
			defer wg.Done()

			err := m.writeWithRetry(ctx, b, userRecords, metadata)

			// Count total records
			count := 0
			for _, records := range userRecords {
				count += len(records)
			}

			results[idx] = WriteResult{
				Backend:        b.Name(),
				RecordsWritten: count,
				Error:          err,
			}

			if err != nil {
				m.logger.Error("backend write failed",
					"backend", b.Name(),
					"error", err,
				)
			} else {
				m.logger.Debug("backend write succeeded",
					"backend", b.Name(),
					"records", count,
				)
			}
		}(i, backend)
	}

	wg.Wait()
	return results
}

// writeWithRetry handles retry logic for backends that support it
func (m *Manager) writeWithRetry(ctx context.Context, b Backend, userRecords [][]Record, metadata Metadata) error {
	rb, ok := b.(RetryableBackend)
	if !ok {
		// No retry support, just write once
		return b.Write(ctx, userRecords, metadata)
	}

	cfg := rb.RetryConfig()

	// If MaxAttempts is 0 or not set, don't use retry logic
	if cfg.MaxAttempts <= 0 {
		return b.Write(ctx, userRecords, metadata)
	}

	delay := cfg.InitialDelay
	if delay == 0 {
		delay = time.Second
	}

	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			m.logger.Debug("retrying backend write",
				"backend", b.Name(),
				"attempt", attempt+1,
				"delay", delay,
			)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			// Exponential backoff
			multiplier := cfg.Multiplier
			if multiplier == 0 {
				multiplier = 2.0
			}
			delay = time.Duration(float64(delay) * multiplier)
			if cfg.MaxDelay > 0 && delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
		}

		if err := b.Write(ctx, userRecords, metadata); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// HealthCheck checks all backends and returns their status
func (m *Manager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for _, b := range m.backends {
		results[b.Name()] = b.Health(ctx)
	}
	return results
}

// Backends returns all managed backends
func (m *Manager) Backends() []Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent modification
	result := make([]Backend, len(m.backends))
	copy(result, m.backends)
	return result
}

// Close closes all backends
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for _, b := range m.backends {
		if err := b.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name(), err))
		}
	}
	m.backends = nil

	if len(errs) > 0 {
		return fmt.Errorf("errors closing backends: %v", errs)
	}
	return nil
}
