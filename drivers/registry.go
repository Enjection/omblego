package drivers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// DriverFactory is a function that creates a new Driver instance
type DriverFactory func() Driver

var (
	registry     = make(map[string]DriverFactory)
	registryLock sync.RWMutex
)

// Register registers a driver factory with the given name.
// Names are case-insensitive.
func Register(name string, factory DriverFactory) {
	registryLock.Lock()
	defer registryLock.Unlock()
	registry[strings.ToLower(name)] = factory
}

// Get returns a new driver instance for the given device name.
// Names are case-insensitive.
func Get(name string) (Driver, error) {
	registryLock.RLock()
	factory, ok := registry[strings.ToLower(name)]
	registryLock.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown device: %s (available: %v)", name, List())
	}

	return factory(), nil
}

// IsSupported returns true if a driver exists for the given device name.
func IsSupported(name string) bool {
	registryLock.RLock()
	defer registryLock.RUnlock()
	_, ok := registry[strings.ToLower(name)]
	return ok
}

// List returns a sorted list of all registered device names.
func List() []string {
	registryLock.RLock()
	defer registryLock.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Count returns the number of registered drivers.
func Count() int {
	registryLock.RLock()
	defer registryLock.RUnlock()
	return len(registry)
}
