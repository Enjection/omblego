package mock

import (
	"context"
	"sync"
	"time"

	"omblego/internal/ble"
)

// Adapter is a mock BLE adapter for testing
type Adapter struct {
	mu sync.RWMutex

	// Configurable behavior
	EnableFunc  func(ctx context.Context) error
	ScanResults []ble.ScanResult
	ScanErr     error
	ConnectFunc func(ctx context.Context, address string) (ble.Connection, error)

	// Tracking
	EnableCalled  bool
	ScanCalled    bool
	ConnectCalled bool
	CloseCalled   bool
	Connections   []*Connection
}

// NewAdapter creates a new mock adapter with default behavior
func NewAdapter() *Adapter {
	return &Adapter{
		ScanResults: []ble.ScanResult{},
	}
}

// Enable implements ble.Adapter
func (a *Adapter) Enable(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.EnableCalled = true

	if a.EnableFunc != nil {
		return a.EnableFunc(ctx)
	}
	return nil
}

// Scan implements ble.Adapter
func (a *Adapter) Scan(ctx context.Context, duration time.Duration) ([]ble.ScanResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ScanCalled = true

	if a.ScanErr != nil {
		return nil, a.ScanErr
	}

	// Simulate scan duration if context allows
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(duration):
	}

	return a.ScanResults, nil
}

// Connect implements ble.Adapter
func (a *Adapter) Connect(ctx context.Context, address string) (ble.Connection, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ConnectCalled = true

	if a.ConnectFunc != nil {
		return a.ConnectFunc(ctx, address)
	}

	// Default: create a new mock connection
	conn := NewConnection(address)
	a.Connections = append(a.Connections, conn)
	return conn, nil
}

// Close implements ble.Adapter
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.CloseCalled = true
	return nil
}

// WithScanResults configures scan results
func (a *Adapter) WithScanResults(results ...ble.ScanResult) *Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ScanResults = results
	return a
}

// WithScanError configures scan to return an error
func (a *Adapter) WithScanError(err error) *Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ScanErr = err
	return a
}

// WithConnectFunc configures custom connect behavior
func (a *Adapter) WithConnectFunc(fn func(ctx context.Context, address string) (ble.Connection, error)) *Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ConnectFunc = fn
	return a
}

// Verify Adapter implements ble.Adapter
var _ ble.Adapter = (*Adapter)(nil)
