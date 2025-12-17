package output

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusBackendConfig holds configuration for the Prometheus backend
type PrometheusBackendConfig struct {
	Port       int    // HTTP server port for /metrics endpoint
	Path       string // Metrics path (default: /metrics)
	Namespace  string // Metric namespace prefix (default: omblego)
	Subsystem  string // Metric subsystem prefix (default: blood_pressure)
}

// PrometheusBackend implements the Backend interface for Prometheus metrics
type PrometheusBackend struct {
	name     string
	config   PrometheusBackendConfig
	server   *http.Server
	registry *prometheus.Registry

	// Gauges for latest values
	systolicGauge  *prometheus.GaugeVec
	diastolicGauge *prometheus.GaugeVec
	pulseGauge     *prometheus.GaugeVec
	movementGauge  *prometheus.GaugeVec
	ihbGauge       *prometheus.GaugeVec
	timestampGauge *prometheus.GaugeVec

	// Counter for total syncs
	syncCounter *prometheus.CounterVec

	// Gauge for last sync timestamp
	lastSyncGauge *prometheus.GaugeVec

	// Gauge for battery level
	batteryGauge *prometheus.GaugeVec

	mu sync.RWMutex
}

// NewPrometheusBackend creates a new Prometheus backend
func NewPrometheusBackend(name string, config PrometheusBackendConfig) (*PrometheusBackend, error) {
	if config.Port == 0 {
		config.Port = 9090
	}
	if config.Path == "" {
		config.Path = "/metrics"
	}
	if config.Namespace == "" {
		config.Namespace = "omblego"
	}
	if config.Subsystem == "" {
		config.Subsystem = "blood_pressure"
	}

	registry := prometheus.NewRegistry()

	labels := []string{"device", "user"}

	systolicGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "systolic_mmhg",
			Help:      "Systolic blood pressure in mmHg",
		},
		labels,
	)

	diastolicGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "diastolic_mmhg",
			Help:      "Diastolic blood pressure in mmHg",
		},
		labels,
	)

	pulseGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "pulse_bpm",
			Help:      "Pulse rate in beats per minute",
		},
		labels,
	)

	movementGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "movement_detected",
			Help:      "Movement detected during measurement (1=yes, 0=no)",
		},
		labels,
	)

	ihbGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "irregular_heartbeat",
			Help:      "Irregular heartbeat detected (1=yes, 0=no)",
		},
		labels,
	)

	timestampGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "last_reading_timestamp_seconds",
			Help:      "Unix timestamp of the last reading",
		},
		labels,
	)

	syncCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Name:      "sync_total",
			Help:      "Total number of syncs performed",
		},
		[]string{"device", "status"},
	)

	lastSyncGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Name:      "last_sync_timestamp_seconds",
			Help:      "Unix timestamp of the last successful sync",
		},
		[]string{"device"},
	)

	batteryGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Name:      "battery_level_percent",
			Help:      "Device battery level in percent",
		},
		[]string{"device"},
	)

	// Register all metrics
	registry.MustRegister(systolicGauge)
	registry.MustRegister(diastolicGauge)
	registry.MustRegister(pulseGauge)
	registry.MustRegister(movementGauge)
	registry.MustRegister(ihbGauge)
	registry.MustRegister(timestampGauge)
	registry.MustRegister(syncCounter)
	registry.MustRegister(lastSyncGauge)
	registry.MustRegister(batteryGauge)

	backend := &PrometheusBackend{
		name:           name,
		config:         config,
		registry:       registry,
		systolicGauge:  systolicGauge,
		diastolicGauge: diastolicGauge,
		pulseGauge:     pulseGauge,
		movementGauge:  movementGauge,
		ihbGauge:       ihbGauge,
		timestampGauge: timestampGauge,
		syncCounter:    syncCounter,
		lastSyncGauge:  lastSyncGauge,
		batteryGauge:   batteryGauge,
	}

	// Start HTTP server for metrics
	if err := backend.startServer(); err != nil {
		return nil, err
	}

	return backend, nil
}

// startServer starts the HTTP server for metrics
func (b *PrometheusBackend) startServer() error {
	mux := http.NewServeMux()
	mux.Handle(b.config.Path, promhttp.HandlerFor(b.registry, promhttp.HandlerOpts{}))

	// Add a simple health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	b.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", b.config.Port),
		Handler: mux,
	}

	go func() {
		if err := b.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't fail - server might be starting slowly
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Name returns the backend instance name
func (b *PrometheusBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *PrometheusBackend) Type() string {
	return "prometheus"
}

// Write updates Prometheus metrics with the latest records
func (b *PrometheusBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	device := metadata.DeviceName
	if device == "" {
		device = "unknown"
	}

	for userIdx, records := range userRecords {
		if len(records) == 0 {
			continue
		}

		// Find the latest record for this user
		var latest *Record
		for i := range records {
			if latest == nil || records[i].Timestamp.After(latest.Timestamp) {
				latest = &records[i]
			}
		}

		if latest == nil {
			continue
		}

		user := fmt.Sprintf("user%d", userIdx+1)

		b.systolicGauge.WithLabelValues(device, user).Set(float64(latest.Systolic))
		b.diastolicGauge.WithLabelValues(device, user).Set(float64(latest.Diastolic))
		b.pulseGauge.WithLabelValues(device, user).Set(float64(latest.Pulse))
		b.timestampGauge.WithLabelValues(device, user).Set(float64(latest.Timestamp.Unix()))

		mov := 0.0
		if latest.Movement {
			mov = 1.0
		}
		b.movementGauge.WithLabelValues(device, user).Set(mov)

		ihb := 0.0
		if latest.IHB {
			ihb = 1.0
		}
		b.ihbGauge.WithLabelValues(device, user).Set(ihb)
	}

	// Increment sync counter and update last sync timestamp
	b.syncCounter.WithLabelValues(device, "success").Inc()
	b.lastSyncGauge.WithLabelValues(device).Set(float64(time.Now().Unix()))

	// Update battery level if available
	if metadata.BatteryLevel != nil {
		b.batteryGauge.WithLabelValues(device).Set(float64(*metadata.BatteryLevel))
	}

	return nil
}

// Health checks if the Prometheus server is running
func (b *PrometheusBackend) Health(ctx context.Context) error {
	if b.server == nil {
		return fmt.Errorf("prometheus server not initialized")
	}

	// Try to connect to the health endpoint
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", b.config.Port))
	if err != nil {
		return fmt.Errorf("prometheus server not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus server returned status %d", resp.StatusCode)
	}

	return nil
}

// Close shuts down the Prometheus HTTP server
func (b *PrometheusBackend) Close() error {
	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return b.server.Shutdown(ctx)
	}
	return nil
}

func init() {
	Register("prometheus", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		port := 9090
		if p, ok := settings["port"].(int); ok {
			port = p
		} else if p, ok := settings["port"].(float64); ok {
			port = int(p)
		}

		path, _ := settings["path"].(string)
		namespace, _ := settings["namespace"].(string)
		subsystem, _ := settings["subsystem"].(string)

		return NewPrometheusBackend(cfg.Name, PrometheusBackendConfig{
			Port:      port,
			Path:      path,
			Namespace: namespace,
			Subsystem: subsystem,
		})
	})
}
