package omblego

import "errors"

var (
	// ErrDeviceNotFound indicates the device could not be found during scanning.
	ErrDeviceNotFound = errors.New("device not found")

	// ErrConnectionFailed indicates a connection to the device could not be established.
	ErrConnectionFailed = errors.New("connection failed")

	// ErrConnectionTimeout indicates the connection attempt timed out.
	ErrConnectionTimeout = errors.New("connection timeout")

	// ErrPairingFailed indicates pairing with the device failed.
	ErrPairingFailed = errors.New("pairing failed")

	// ErrCommunicationError indicates a communication error occurred during the sync.
	ErrCommunicationError = errors.New("communication error")

	// ErrUnsupportedDevice indicates the device model is not supported.
	ErrUnsupportedDevice = errors.New("unsupported device")

	// ErrNoAdapter indicates no BLE adapter is available.
	ErrNoAdapter = errors.New("no bluetooth adapter available")

	// ErrBluetoothDisabled indicates Bluetooth is disabled on the system.
	ErrBluetoothDisabled = errors.New("bluetooth is disabled")

	// ErrInvalidConfig indicates the configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrSyncInProgress indicates a sync operation is already in progress.
	ErrSyncInProgress = errors.New("sync already in progress")

	// ErrOutputFailed indicates one or more output backends failed.
	ErrOutputFailed = errors.New("output write failed")
)

// SyncError wraps an error with additional context about the sync operation.
type SyncError struct {
	Op      string // Operation that failed (e.g., "scan", "connect", "read")
	Device  string // Device name if available
	Err     error  // Underlying error
}

func (e *SyncError) Error() string {
	if e.Device != "" {
		return e.Op + " " + e.Device + ": " + e.Err.Error()
	}
	return e.Op + ": " + e.Err.Error()
}

func (e *SyncError) Unwrap() error {
	return e.Err
}

// NewSyncError creates a new SyncError.
func NewSyncError(op, device string, err error) *SyncError {
	return &SyncError{
		Op:     op,
		Device: device,
		Err:    err,
	}
}
