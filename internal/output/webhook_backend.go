package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookBackendConfig holds configuration for the Webhook backend
type WebhookBackendConfig struct {
	URL         string            // Webhook URL
	Method      string            // HTTP method (default: POST)
	Headers     map[string]string // Custom headers
	Timeout     time.Duration     // Request timeout
	ContentType string            // Content-Type header (default: application/json)
}

// WebhookBackend implements the Backend interface for webhook output
type WebhookBackend struct {
	name   string
	config WebhookBackendConfig
	client *http.Client
}

// WebhookPayload is the JSON structure sent to the webhook
type WebhookPayload struct {
	DeviceName    string              `json:"device_name"`
	SyncTimestamp time.Time           `json:"sync_timestamp"`
	IsFullSync    bool                `json:"is_full_sync"`
	SessionID     string              `json:"session_id"`
	Users         []WebhookUserData   `json:"users"`
	TotalRecords  int                 `json:"total_records"`
}

// WebhookUserData represents records for a single user
type WebhookUserData struct {
	UserID  int             `json:"user_id"`
	Records []WebhookRecord `json:"records"`
}

// WebhookRecord represents a single blood pressure record
type WebhookRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Systolic  int       `json:"systolic"`
	Diastolic int       `json:"diastolic"`
	Pulse     int       `json:"pulse"`
	Movement  bool      `json:"movement"`
	IHB       bool      `json:"ihb"`
}

// NewWebhookBackend creates a new Webhook backend
func NewWebhookBackend(name string, config WebhookBackendConfig) (*WebhookBackend, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("webhook url is required")
	}
	if config.Method == "" {
		config.Method = http.MethodPost
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.ContentType == "" {
		config.ContentType = "application/json"
	}

	return &WebhookBackend{
		name:   name,
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

// Name returns the backend instance name
func (b *WebhookBackend) Name() string {
	return b.name
}

// Type returns the backend type
func (b *WebhookBackend) Type() string {
	return "webhook"
}

// Write sends records to the webhook
func (b *WebhookBackend) Write(ctx context.Context, userRecords [][]Record, metadata Metadata) error {
	payload := b.buildPayload(userRecords, metadata)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, b.config.Method, b.config.URL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", b.config.ContentType)
	for key, value := range b.config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// buildPayload creates the webhook payload from records
func (b *WebhookBackend) buildPayload(userRecords [][]Record, metadata Metadata) WebhookPayload {
	payload := WebhookPayload{
		DeviceName:    metadata.DeviceName,
		SyncTimestamp: metadata.SyncTimestamp,
		IsFullSync:    metadata.IsFullSync,
		SessionID:     metadata.SessionID,
		Users:         make([]WebhookUserData, 0, len(userRecords)),
	}

	for userIdx, records := range userRecords {
		if len(records) == 0 {
			continue
		}

		userData := WebhookUserData{
			UserID:  userIdx + 1,
			Records: make([]WebhookRecord, len(records)),
		}

		for i, rec := range records {
			userData.Records[i] = WebhookRecord{
				Timestamp: rec.Timestamp,
				Systolic:  rec.Systolic,
				Diastolic: rec.Diastolic,
				Pulse:     rec.Pulse,
				Movement:  rec.Movement,
				IHB:       rec.IHB,
			}
			payload.TotalRecords++
		}

		payload.Users = append(payload.Users, userData)
	}

	return payload
}

// Health checks if the webhook endpoint is reachable
func (b *WebhookBackend) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook endpoint not reachable: %w", err)
	}
	defer resp.Body.Close()

	// Accept any 2xx or 405 (method not allowed is fine for HEAD)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("webhook endpoint returned error: %d", resp.StatusCode)
	}

	return nil
}

// Close releases resources
func (b *WebhookBackend) Close() error {
	b.client.CloseIdleConnections()
	return nil
}

// RetryConfig returns retry configuration for webhook
func (b *WebhookBackend) RetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

func init() {
	Register("webhook", func(cfg BackendConfig) (Backend, error) {
		settings := cfg.Settings

		url, _ := settings["url"].(string)
		method, _ := settings["method"].(string)
		contentType, _ := settings["content_type"].(string)

		headers := make(map[string]string)
		if h, ok := settings["headers"].(map[string]interface{}); ok {
			for k, v := range h {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
		}

		var timeout time.Duration
		if t, ok := settings["timeout"].(string); ok {
			timeout, _ = time.ParseDuration(t)
		}

		return NewWebhookBackend(cfg.Name, WebhookBackendConfig{
			URL:         url,
			Method:      method,
			Headers:     headers,
			Timeout:     timeout,
			ContentType: contentType,
		})
	})
}
