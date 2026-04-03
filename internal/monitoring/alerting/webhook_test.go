package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewWebhookHandler(t *testing.T) {
	config := WebhookConfig{
		URL:       "http://localhost:5001/webhook",
		MaxAlerts: 10,
	}

	handler := NewWebhookHandler(config)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.httpClient)
	assert.Equal(t, config, handler.config)
}

func TestWebhookHandler_ValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid HTTP URL",
			url:     "http://localhost:5001/webhook",
			wantErr: false,
		},
		{
			name:    "valid HTTPS URL",
			url:     "https://example.com/webhook",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid URL scheme",
			url:     "ftp://localhost/webhook",
			wantErr: true,
		},
		{
			name:    "URL without scheme",
			url:     "localhost:5001/webhook",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWebhookHandler(WebhookConfig{URL: tt.url})
			err := handler.ValidateURL()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWebhookHandler_Send_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		SendResolved: true,
	}

	handler := NewWebhookHandler(config)

	alert := NewAlert("TestAlert", SeverityWarning, "Test", "Test", nil)
	group := NewAlertGroupBuilder("test").
		AddAlert(*alert).
		Build()
	payload := group.ToWebhookPayload()

	err := handler.Send(payload)
	assert.NoError(t, err)
}

func TestWebhookHandler_Send_ResolvedNotSent(t *testing.T) {
	// Create test server that should not receive any requests
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		SendResolved: false,
	}

	handler := NewWebhookHandler(config)

	// Create resolved alert
	alert := NewAlert("TestAlert", SeverityWarning, "Test", "Test", nil)
	alert = alert.Resolved()
	group := NewAlertGroupBuilder("test").
		AddAlert(*alert).
		Build()
	payload := group.ToWebhookPayload()

	err := handler.Send(payload)
	assert.NoError(t, err)
	assert.Equal(t, 0, requestCount, "Request should not have been sent for resolved alert")
}

func TestWebhookHandler_Send_MaxAlerts(t *testing.T) {
	alertsReceived := 0

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload AlertManagerWebhookPayload
		err := readJSONBody(r, &payload)
		assert.NoError(t, err)
		alertsReceived = len(payload.Alerts)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		SendResolved: true,
		MaxAlerts:    3,
	}

	handler := NewWebhookHandler(config)

	// Create builder with more alerts than max
	builder := NewAlertGroupBuilder("test")
	for i := 0; i < 10; i++ {
		builder.AddAlert(*NewAlert("TestAlert", SeverityWarning, "Test", "Test", nil))
	}
	group := builder.Build()
	payload := group.ToWebhookPayload()

	err := handler.Send(payload)
	assert.NoError(t, err)
	assert.Equal(t, 3, alertsReceived, "Should only send 3 alerts")
}

func TestWebhookHandler_Send_Error(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		SendResolved: true,
	}

	handler := NewWebhookHandler(config)

	alert := NewAlert("TestAlert", SeverityWarning, "Test", "Test", nil)
	group := NewAlertGroupBuilder("test").
		AddAlert(*alert).
		Build()
	payload := group.ToWebhookPayload()

	err := handler.Send(payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-OK status: 500")
}

func TestWebhookHandler_Send_NoServer(t *testing.T) {
	config := WebhookConfig{
		URL:          "http://localhost:1/webhook",
		SendResolved: true,
	}

	handler := NewWebhookHandler(config)

	alert := NewAlert("TestAlert", SeverityWarning, "Test", "Test", nil)
	group := NewAlertGroupBuilder("test").
		AddAlert(*alert).
		Build()
	payload := group.ToWebhookPayload()

	err := handler.Send(payload)
	assert.Error(t, err)
}

// Helper function to read JSON body
func readJSONBody(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	defer func() {
		_ = r.Body.Close()
	}()
	return dec.Decode(v)
}
