package output_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"omblego/internal/output"
)

func TestPrometheusBackend_Write(t *testing.T) {
	// Use a random port to avoid conflicts
	port := 19090 + time.Now().UnixNano()%1000

	backend, err := output.NewPrometheusBackend("test-prometheus", output.PrometheusBackendConfig{
		Port:      int(port),
		Path:      "/metrics",
		Namespace: "test",
		Subsystem: "bp",
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	records := [][]output.Record{
		{
			{
				Timestamp: time.Now(),
				Systolic:  120,
				Diastolic: 80,
				Pulse:     72,
				Movement:  false,
				IHB:       true,
			},
		},
	}

	metadata := output.Metadata{
		DeviceName:    "BP7000",
		SyncTimestamp: time.Now(),
	}

	err = backend.Write(context.Background(), records, metadata)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Fetch metrics
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Fatalf("Failed to fetch metrics: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	metrics := string(body)

	// Verify metrics are present
	expectedMetrics := []string{
		"test_bp_systolic_mmhg",
		"test_bp_diastolic_mmhg",
		"test_bp_pulse_bpm",
		"test_bp_irregular_heartbeat",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(metrics, metric) {
			t.Errorf("Expected metric %s not found in output", metric)
		}
	}

	// Verify values
	if !strings.Contains(metrics, `systolic_mmhg{device="BP7000",user="user1"} 120`) {
		t.Error("Systolic value not found or incorrect")
	}
}

func TestPrometheusBackend_Type(t *testing.T) {
	port := 19100 + time.Now().UnixNano()%1000

	backend, err := output.NewPrometheusBackend("test", output.PrometheusBackendConfig{
		Port: int(port),
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	if backend.Type() != "prometheus" {
		t.Errorf("Expected type 'prometheus', got '%s'", backend.Type())
	}
}

func TestPrometheusBackend_Health(t *testing.T) {
	port := 19200 + time.Now().UnixNano()%1000

	backend, err := output.NewPrometheusBackend("test", output.PrometheusBackendConfig{
		Port: int(port),
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	err = backend.Health(context.Background())
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestPrometheusBackend_MultipleUsers(t *testing.T) {
	port := 19300 + time.Now().UnixNano()%1000

	backend, err := output.NewPrometheusBackend("test", output.PrometheusBackendConfig{
		Port: int(port),
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	time.Sleep(200 * time.Millisecond)

	records := [][]output.Record{
		{
			{Timestamp: time.Now(), Systolic: 120, Diastolic: 80, Pulse: 72},
		},
		{
			{Timestamp: time.Now(), Systolic: 130, Diastolic: 85, Pulse: 68},
		},
	}

	backend.Write(context.Background(), records, output.Metadata{DeviceName: "BP7000"})

	resp, _ := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	metrics := string(body)

	// Both users should be present
	if !strings.Contains(metrics, `user="user1"`) {
		t.Error("user1 metrics not found")
	}
	if !strings.Contains(metrics, `user="user2"`) {
		t.Error("user2 metrics not found")
	}
}
