package output_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"omblego/internal/output"
)

func TestWebhookBackend_Write(t *testing.T) {
	var receivedPayload output.WebhookPayload
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	backend, err := output.NewWebhookBackend("test-webhook", output.WebhookBackendConfig{
		URL: server.URL,
		Headers: map[string]string{
			"X-Custom-Header": "test-value",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	records := [][]output.Record{
		{
			{
				Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				Systolic:  120,
				Diastolic: 80,
				Pulse:     72,
				Movement:  true,
				IHB:       false,
			},
		},
	}

	metadata := output.Metadata{
		DeviceName:    "BP7000",
		SyncTimestamp: time.Now(),
		SessionID:     "test-session",
	}

	err = backend.Write(context.Background(), records, metadata)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify payload
	if receivedPayload.DeviceName != "BP7000" {
		t.Errorf("Expected device BP7000, got %s", receivedPayload.DeviceName)
	}

	if len(receivedPayload.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(receivedPayload.Users))
	}

	if receivedPayload.TotalRecords != 1 {
		t.Errorf("Expected 1 total record, got %d", receivedPayload.TotalRecords)
	}

	// Verify custom header
	if receivedHeaders.Get("X-Custom-Header") != "test-value" {
		t.Error("Custom header not received")
	}
}

func TestWebhookBackend_WriteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	backend, _ := output.NewWebhookBackend("test", output.WebhookBackendConfig{
		URL: server.URL,
	})

	records := [][]output.Record{{}}
	metadata := output.Metadata{}

	err := backend.Write(context.Background(), records, metadata)
	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestWebhookBackend_Type(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	backend, _ := output.NewWebhookBackend("test", output.WebhookBackendConfig{
		URL: server.URL,
	})

	if backend.Type() != "webhook" {
		t.Errorf("Expected type 'webhook', got '%s'", backend.Type())
	}
}

func TestWebhookBackend_RetryConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	backend, _ := output.NewWebhookBackend("test", output.WebhookBackendConfig{
		URL: server.URL,
	})

	cfg := backend.RetryConfig()
	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected 3 max attempts, got %d", cfg.MaxAttempts)
	}
}

func TestWebhookBackend_RequiresURL(t *testing.T) {
	_, err := output.NewWebhookBackend("test", output.WebhookBackendConfig{})
	if err == nil {
		t.Error("Expected error when URL is missing")
	}
}
