package mock

import (
	"context"
	"sync"

	"omblego/internal/output"
)

// Backend is a mock output backend for testing
type Backend struct {
	mu sync.RWMutex

	name       string
	backendType string

	// Configurable behavior
	WriteFunc   func(ctx context.Context, records [][]output.Record, metadata output.Metadata) error
	HealthFunc  func(ctx context.Context) error
	WriteErr    error
	HealthErr   error
	RetryConf   *output.RetryConfig

	// Tracking
	WriteCalls     int
	WrittenRecords [][][]output.Record
	WrittenMeta    []output.Metadata
	HealthCalls    int
	CloseCalled    bool
}

// NewBackend creates a new mock backend
func NewBackend(name string) *Backend {
	return &Backend{
		name:        name,
		backendType: "mock",
	}
}

// Name implements output.Backend
func (b *Backend) Name() string {
	return b.name
}

// Type implements output.Backend
func (b *Backend) Type() string {
	return b.backendType
}

// Write implements output.Backend
func (b *Backend) Write(ctx context.Context, userRecords [][]output.Record, metadata output.Metadata) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.WriteCalls++

	// Store copies of data
	recordsCopy := make([][]output.Record, len(userRecords))
	for i, records := range userRecords {
		recordsCopy[i] = make([]output.Record, len(records))
		copy(recordsCopy[i], records)
	}
	b.WrittenRecords = append(b.WrittenRecords, recordsCopy)
	b.WrittenMeta = append(b.WrittenMeta, metadata)

	if b.WriteFunc != nil {
		return b.WriteFunc(ctx, userRecords, metadata)
	}

	return b.WriteErr
}

// Health implements output.Backend
func (b *Backend) Health(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.HealthCalls++

	if b.HealthFunc != nil {
		return b.HealthFunc(ctx)
	}

	return b.HealthErr
}

// Close implements output.Backend
func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.CloseCalled = true
	return nil
}

// RetryConfig implements output.RetryableBackend (optional)
func (b *Backend) RetryConfig() output.RetryConfig {
	if b.RetryConf != nil {
		return *b.RetryConf
	}
	return output.RetryConfig{}
}

// WithWriteError configures the backend to return an error on write
func (b *Backend) WithWriteError(err error) *Backend {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.WriteErr = err
	return b
}

// WithHealthError configures the backend to return an error on health check
func (b *Backend) WithHealthError(err error) *Backend {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.HealthErr = err
	return b
}

// WithWriteFunc configures custom write behavior
func (b *Backend) WithWriteFunc(fn func(ctx context.Context, records [][]output.Record, metadata output.Metadata) error) *Backend {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.WriteFunc = fn
	return b
}

// WithRetryConfig configures retry behavior (makes backend implement RetryableBackend)
func (b *Backend) WithRetryConfig(cfg output.RetryConfig) *Backend {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.RetryConf = &cfg
	return b
}

// GetWriteCalls returns the number of times Write was called
func (b *Backend) GetWriteCalls() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.WriteCalls
}

// GetWrittenRecords returns all records written to this backend
func (b *Backend) GetWrittenRecords() [][][]output.Record {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.WrittenRecords
}

// GetWrittenMetadata returns all metadata from write calls
func (b *Backend) GetWrittenMetadata() []output.Metadata {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.WrittenMeta
}

// Reset clears all tracking data
func (b *Backend) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.WriteCalls = 0
	b.WrittenRecords = nil
	b.WrittenMeta = nil
	b.HealthCalls = 0
	b.CloseCalled = false
	b.WriteErr = nil
	b.HealthErr = nil
}

// Verify Backend implements output.Backend
var _ output.Backend = (*Backend)(nil)

// BackendFactory returns a factory function for creating mock backends
func BackendFactory(cfg output.BackendConfig) (output.Backend, error) {
	name := cfg.Name
	if name == "" {
		name = "mock"
	}
	return NewBackend(name), nil
}

func init() {
	output.Register("mock", BackendFactory)
}
