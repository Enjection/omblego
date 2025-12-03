package device

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// DriverFactory creates a new driver instance
type DriverFactory func() Driver

var (
	registry     = make(map[string]DriverFactory)
	registryLock sync.RWMutex
)

// Register adds a device driver to the registry
func Register(name string, factory DriverFactory) {
	registryLock.Lock()
	defer registryLock.Unlock()
	registry[strings.ToLower(name)] = factory
}

// GetDriver returns a driver for the specified device name
func GetDriver(name string) (Driver, error) {
	registryLock.RLock()
	defer registryLock.RUnlock()

	factory, ok := registry[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unsupported device: %s (use list-devices to see available devices)", name)
	}
	return factory(), nil
}

// ListDevices returns all registered device names sorted alphabetically
func ListDevices() []string {
	registryLock.RLock()
	defer registryLock.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, strings.ToUpper(name))
	}
	sort.Strings(names)
	return names
}

// IsDeviceSupported checks if a device is in the registry
func IsDeviceSupported(name string) bool {
	registryLock.RLock()
	defer registryLock.RUnlock()

	_, ok := registry[strings.ToLower(name)]
	return ok
}
