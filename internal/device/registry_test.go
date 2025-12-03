package device

import (
	"testing"
	"time"
)

// MockDriver for testing
type MockDriver struct {
	config Config
}

func (d *MockDriver) GetConfig() Config {
	return d.config
}

func (d *MockDriver) ParseRecord(data []byte) (*BloodPressureRecord, error) {
	return nil, nil
}

func (d *MockDriver) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return nil, nil
}

func TestRegisterAndGetDriver(t *testing.T) {
	// Register a test driver
	testConfig := Config{Name: "TEST-DEVICE"}
	Register("test-device", func() Driver {
		return &MockDriver{config: testConfig}
	})

	// Get it back
	driver, err := GetDriver("test-device")
	if err != nil {
		t.Fatalf("GetDriver error: %v", err)
	}

	if driver.GetConfig().Name != "TEST-DEVICE" {
		t.Errorf("Driver name = %s, want TEST-DEVICE", driver.GetConfig().Name)
	}
}

func TestGetDriver_CaseInsensitive(t *testing.T) {
	Register("case-test", func() Driver {
		return &MockDriver{config: Config{Name: "CASE-TEST"}}
	})

	testCases := []string{
		"case-test",
		"CASE-TEST",
		"Case-Test",
		"CASE-test",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			driver, err := GetDriver(tc)
			if err != nil {
				t.Errorf("GetDriver(%q) error: %v", tc, err)
				return
			}
			if driver == nil {
				t.Errorf("GetDriver(%q) returned nil", tc)
			}
		})
	}
}

func TestGetDriver_NotFound(t *testing.T) {
	_, err := GetDriver("nonexistent-device")
	if err == nil {
		t.Error("GetDriver should error for nonexistent device")
	}
}

func TestListDevices(t *testing.T) {
	// Register some test devices
	Register("list-test-a", func() Driver { return &MockDriver{} })
	Register("list-test-b", func() Driver { return &MockDriver{} })

	devices := ListDevices()

	// Should contain our test devices (uppercase)
	found := make(map[string]bool)
	for _, d := range devices {
		found[d] = true
	}

	if !found["LIST-TEST-A"] {
		t.Error("LIST-TEST-A not found in device list")
	}
	if !found["LIST-TEST-B"] {
		t.Error("LIST-TEST-B not found in device list")
	}
}

func TestListDevices_Sorted(t *testing.T) {
	devices := ListDevices()

	// Verify sorted order
	for i := 1; i < len(devices); i++ {
		if devices[i] < devices[i-1] {
			t.Errorf("Device list not sorted: %s comes before %s", devices[i-1], devices[i])
		}
	}
}

func TestIsDeviceSupported(t *testing.T) {
	Register("supported-test", func() Driver { return &MockDriver{} })

	if !IsDeviceSupported("supported-test") {
		t.Error("IsDeviceSupported should return true for registered device")
	}

	if !IsDeviceSupported("SUPPORTED-TEST") {
		t.Error("IsDeviceSupported should be case insensitive")
	}

	if IsDeviceSupported("not-supported") {
		t.Error("IsDeviceSupported should return false for unregistered device")
	}
}
