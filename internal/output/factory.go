package output

import (
	"fmt"
	"sort"
	"sync"
)

// BackendConfig is a generic configuration for creating backends
type BackendConfig struct {
	Type     string                 // Backend type (e.g., "csv", "influxdb")
	Name     string                 // Instance name (optional, defaults to type)
	Enabled  bool                   // Whether this backend is enabled
	Settings map[string]interface{} // Type-specific settings
}

// BackendFactory creates a Backend from configuration
type BackendFactory func(cfg BackendConfig) (Backend, error)

var (
	factories     = make(map[string]BackendFactory)
	factoriesLock sync.RWMutex
)

// Register registers a backend factory for a given type.
// This is typically called in init() functions of backend packages.
func Register(backendType string, factory BackendFactory) {
	factoriesLock.Lock()
	defer factoriesLock.Unlock()
	factories[backendType] = factory
}

// Create creates a backend from configuration
func Create(cfg BackendConfig) (Backend, error) {
	factoriesLock.RLock()
	factory, ok := factories[cfg.Type]
	factoriesLock.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown output backend type: %q (registered: %v)", cfg.Type, ListTypes())
	}

	return factory(cfg)
}

// IsRegistered checks if a backend type is registered
func IsRegistered(backendType string) bool {
	factoriesLock.RLock()
	defer factoriesLock.RUnlock()
	_, ok := factories[backendType]
	return ok
}

// ListTypes returns all registered backend types
func ListTypes() []string {
	factoriesLock.RLock()
	defer factoriesLock.RUnlock()

	types := make([]string, 0, len(factories))
	for t := range factories {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// BackendInfo provides information about a registered backend type
type BackendInfo struct {
	Type        string
	Description string
}

// ListBackends returns information about all registered backends
func ListBackends() []BackendInfo {
	factoriesLock.RLock()
	defer factoriesLock.RUnlock()

	info := make([]BackendInfo, 0, len(factories))
	for name := range factories {
		info = append(info, BackendInfo{
			Type:        name,
			Description: getBackendDescription(name),
		})
	}
	return info
}

func getBackendDescription(t string) string {
	descriptions := map[string]string{
		"csv":        "Writes records to CSV files, one per user",
		"json":       "Writes records to JSON files in UBPM format",
		"influxdb":   "Sends records to InfluxDB time-series database",
		"webhook":    "Posts records to HTTP endpoints",
		"database":   "Stores records in SQL databases (PostgreSQL, MySQL, SQLite)",
		"script":     "Executes external scripts with record data",
		"prometheus": "Exposes metrics endpoint for Prometheus scraping",
	}
	if desc, ok := descriptions[t]; ok {
		return desc
	}
	return "No description available"
}
