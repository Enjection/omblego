package drivers_test

import (
	"testing"
	"time"

	"omblego/drivers"
)

// mockDriver is a simple test driver
type mockDriver struct {
	name   string
	config drivers.DeviceConfig
}

func (d *mockDriver) Name() string {
	return d.name
}

func (d *mockDriver) Config() drivers.DeviceConfig {
	return d.config
}

func (d *mockDriver) ParseRecord(data []byte) (*drivers.BloodPressureRecord, error) {
	return &drivers.BloodPressureRecord{
		Timestamp: time.Now(),
		Systolic:  120,
		Diastolic: 80,
		Pulse:     70,
	}, nil
}

func (d *mockDriver) BuildTimeSyncBytes(cachedSettings []byte, newTime time.Time) ([]byte, error) {
	return nil, nil
}

func newMockDriver(name string) drivers.DriverFactory {
	return func() drivers.Driver {
		return &mockDriver{
			name: name,
			config: drivers.DeviceConfig{
				Name:     name,
				Protocol: drivers.ProtocolOmronCustom,
			},
		}
	}
}

func TestRegister_And_Get(t *testing.T) {
	drivers.Register("test-device", newMockDriver("TEST-DEVICE"))

	drv, err := drivers.Get("test-device")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if drv.Name() != "TEST-DEVICE" {
		t.Errorf("Expected name TEST-DEVICE, got %s", drv.Name())
	}
}

func TestGet_CaseInsensitive(t *testing.T) {
	drivers.Register("case-test", newMockDriver("CASE-TEST"))

	// Should find regardless of case
	_, err := drivers.Get("CASE-TEST")
	if err != nil {
		t.Errorf("Get with uppercase failed: %v", err)
	}

	_, err = drivers.Get("Case-Test")
	if err != nil {
		t.Errorf("Get with mixed case failed: %v", err)
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := drivers.Get("nonexistent-device-xyz")
	if err == nil {
		t.Error("Expected error for unknown device")
	}
}

func TestIsSupported(t *testing.T) {
	drivers.Register("supported-device", newMockDriver("SUPPORTED"))

	if !drivers.IsSupported("supported-device") {
		t.Error("Expected IsSupported to return true")
	}

	if drivers.IsSupported("not-supported-abc") {
		t.Error("Expected IsSupported to return false for unknown device")
	}
}

func TestList(t *testing.T) {
	drivers.Register("list-test-a", newMockDriver("A"))
	drivers.Register("list-test-b", newMockDriver("B"))

	list := drivers.List()

	// Should contain our registered devices
	found := map[string]bool{"list-test-a": false, "list-test-b": false}
	for _, name := range list {
		if _, ok := found[name]; ok {
			found[name] = true
		}
	}

	for name, f := range found {
		if !f {
			t.Errorf("Expected %s in list", name)
		}
	}

	// Should be sorted
	for i := 1; i < len(list); i++ {
		if list[i] < list[i-1] {
			t.Error("List should be sorted")
			break
		}
	}
}

func TestDriverInterface(t *testing.T) {
	drivers.Register("interface-test", newMockDriver("INTERFACE-TEST"))

	drv, _ := drivers.Get("interface-test")

	// Test Config()
	cfg := drv.Config()
	if cfg.Name != "INTERFACE-TEST" {
		t.Errorf("Config().Name mismatch")
	}
	if cfg.Protocol != drivers.ProtocolOmronCustom {
		t.Errorf("Config().Protocol mismatch")
	}

	// Test ParseRecord()
	record, err := drv.ParseRecord([]byte{0x01, 0x02})
	if err != nil {
		t.Errorf("ParseRecord failed: %v", err)
	}
	if record.Systolic != 120 {
		t.Errorf("Expected systolic 120, got %d", record.Systolic)
	}

	// Test BuildTimeSyncBytes()
	_, err = drv.BuildTimeSyncBytes([]byte{}, time.Now())
	if err != nil {
		t.Errorf("BuildTimeSyncBytes failed: %v", err)
	}
}
