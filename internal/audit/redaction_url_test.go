package audit

import (
	"strings"
	"testing"
)

func TestRedactURLCredentials(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantRedact   bool
		wantContains string // substring that should be in output if redacted
	}{
		{
			name:         "Slack webhook URL with token",
			input:        "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			wantRedact:   true,
			wantContains: "[REDACTED]",
		},
		{
			name:         "URL with token in query param",
			input:        "https://example.com/webhook?token=ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
			wantRedact:   true,
			wantContains: "token=[REDACTED]",
		},
		{
			name:       "Normal URL without credentials",
			input:      "https://api.example.com/v1/normal/path",
			wantRedact: false,
		},
		{
			name:       "URL with short path segment",
			input:      "https://example.com/api/v1/users",
			wantRedact: false,
		},
		{
			name:         "URL with hex token in path",
			input:        "https://api.example.com/webhook/1234567890abcdef1234567890abcdef",
			wantRedact:   true,
			wantContains: "[REDACTED]",
		},
		{
			name:       "Non-URL string",
			input:      "not-a-url",
			wantRedact: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, redacted := redactURLCredentials(tt.input)
			if redacted != tt.wantRedact {
				t.Errorf("redactURLCredentials() redacted = %v, want %v", redacted, tt.wantRedact)
			}
			if tt.wantRedact && tt.wantContains != "" && !strings.Contains(result, tt.wantContains) {
				t.Errorf("redactURLCredentials() = %v, want to contain %v", result, tt.wantContains)
			}
			if !tt.wantRedact && result != tt.input {
				t.Errorf("redactURLCredentials() = %v, want unchanged %v", result, tt.input)
			}
		})
	}
}

func TestRedactSensitiveWithURL(t *testing.T) {
	data := map[string]interface{}{
		"url":          "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
		"callback_url": "https://example.com/webhook?token=ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
		"normal_url":   "https://api.example.com/v1/users",
		"password":     "secret123",
	}

	RedactSensitive(data)

	// URL fields should have credentials redacted (either full field or just the token)
	if url, ok := data["url"].(string); !ok || !strings.Contains(url, "[REDACTED]") {
		t.Errorf("url field not redacted: %v", data["url"])
	}

	if callbackURL, ok := data["callback_url"].(string); !ok || !strings.Contains(callbackURL, "[REDACTED]") {
		t.Errorf("callback_url field not redacted: %v", data["callback_url"])
	}

	// Normal URL should be unchanged
	if normalURL, ok := data["normal_url"].(string); !ok || normalURL != "https://api.example.com/v1/users" {
		t.Errorf("normal_url field changed unexpectedly: %v", data["normal_url"])
	}

	// Password field should be redacted by field name
	if password, ok := data["password"].(string); !ok || password != "[REDACTED]" {
		t.Errorf("password field not redacted: %v", data["password"])
	}
}

func TestRedactMetadataWithURL(t *testing.T) {
	metadata := map[string]string{
		"url": "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
	}

	result := RedactMetadata(metadata)

	if url, ok := result["url"]; !ok || !strings.Contains(url, "[REDACTED]") {
		t.Errorf("metadata url field not redacted: %v", result["url"])
	}
}
