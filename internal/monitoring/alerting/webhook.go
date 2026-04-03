package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookHandler handles generic webhook notifications
type WebhookHandler struct {
	config     WebhookConfig
	httpClient *http.Client
}

// NewWebhookHandler creates a new generic webhook handler
func NewWebhookHandler(config WebhookConfig) *WebhookHandler {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &WebhookHandler{
		config:     config,
		httpClient: client,
	}
}

// NewWebhookHandlerWithClient creates a new webhook handler with custom HTTP client
func NewWebhookHandlerWithClient(config WebhookConfig, client *http.Client) *WebhookHandler {
	return &WebhookHandler{
		config:     config,
		httpClient: client,
	}
}

// Send sends the alert payload to the configured webhook URL
func (w *WebhookHandler) Send(payload *AlertManagerWebhookPayload) error {
	if !w.shouldSend(payload) {
		return nil
	}

	// Apply max alerts limit
	alerts := payload.Alerts
	if w.config.MaxAlerts > 0 && len(alerts) > w.config.MaxAlerts {
		alerts = alerts[:w.config.MaxAlerts]
	}

	// Create a copy of payload with limited alerts
	limitedPayload := *payload
	limitedPayload.Alerts = alerts

	data, err := json.Marshal(limitedPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest("POST", w.config.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

// shouldSend determines if the notification should be sent based on the configuration
func (w *WebhookHandler) shouldSend(payload *AlertManagerWebhookPayload) bool {
	// Don't send resolved notifications unless configured to do so
	if payload.Status == AlertStatusResolved && !w.config.SendResolved {
		return false
	}
	return true
}

// ValidateURL validates the webhook URL
func (w *WebhookHandler) ValidateURL() error {
	if w.config.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Basic URL validation - just check it starts with http
	if !bytes.HasPrefix([]byte(w.config.URL), []byte("http://")) &&
		!bytes.HasPrefix([]byte(w.config.URL), []byte("https://")) {
		return fmt.Errorf("webhook URL must start with http:// or https://")
	}

	return nil
}
