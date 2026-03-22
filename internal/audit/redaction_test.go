package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactionRule_Struct(t *testing.T) {
	rule := RedactionRule{
		Pattern:     `(?i)\bpassword\b`,
		Replacement: "[REDACTED]",
		FieldPaths:  []string{"$..password"},
		Mandatory:   true,
	}

	assert.Equal(t, `(?i)\bpassword\b`, rule.Pattern)
	assert.Equal(t, "[REDACTED]", rule.Replacement)
	assert.Equal(t, []string{"$..password"}, rule.FieldPaths)
	assert.True(t, rule.Mandatory)
}

func TestMandatoryRedactionRules(t *testing.T) {
	// Verify all mandatory rules are defined
	require.NotEmpty(t, MandatoryRedactionRules)
	assert.GreaterOrEqual(t, len(MandatoryRedactionRules), 5)

	// Check that all mandatory rules have required fields
	for _, rule := range MandatoryRedactionRules {
		assert.NotEmpty(t, rule.Pattern, "Rule pattern should not be empty")
		assert.NotEmpty(t, rule.Replacement, "Rule replacement should not be empty")
		assert.NotEmpty(t, rule.FieldPaths, "Rule should have at least one field path")
		assert.True(t, rule.Mandatory, "Rule should be marked as mandatory")
	}

	// Verify specific sensitive fields are covered
	patterns := make([]string, len(MandatoryRedactionRules))
	for i, rule := range MandatoryRedactionRules {
		patterns[i] = rule.Pattern
	}

	// Check for password pattern
	foundPassword := false
	for _, p := range patterns {
		if contains(p, "password") || contains(p, "passwd") || contains(p, "pwd") {
			foundPassword = true
			break
		}
	}
	assert.True(t, foundPassword, "Should have password redaction rule")

	// Check for token pattern
	foundToken := false
	for _, p := range patterns {
		if contains(p, "token") {
			foundToken = true
			break
		}
	}
	assert.True(t, foundToken, "Should have token redaction rule")

	// Check for secret/api_key pattern
	foundSecret := false
	for _, p := range patterns {
		if contains(p, "secret") || contains(p, "api_key") {
			foundSecret = true
			break
		}
	}
	assert.True(t, foundSecret, "Should have secret/api_key redaction rule")
}

func TestNewRedactor(t *testing.T) {
	r := NewRedactor()
	require.NotNil(t, r)

	// Verify rules are loaded
	assert.Equal(t, len(MandatoryRedactionRules), len(r.rules))
	assert.Equal(t, len(MandatoryRedactionRules), len(r.compiled))

	// Verify all patterns are compiled
	for _, re := range r.compiled {
		assert.NotNil(t, re)
	}
}

func TestRedactor_RedactEvent(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name     string
		event    *AuditEvent
		expected map[string]string
	}{
		{
			name: "redact password in context metadata",
			event: &AuditEvent{
				ID:        "test-1",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAuth,
				Action:    "login",
				Actor:     AuditActor{ID: "user-1", Type: "user", Name: "user@example.com"},
				Resource:  AuditResource{ID: "auth-1", Type: "service", Name: "auth"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						"username": "user@example.com",
						"password": "secret123",
					},
				},
			},
			expected: map[string]string{
				"password": "[REDACTED]",
			},
		},
		{
			name: "redact token in context metadata",
			event: &AuditEvent{
				ID:        "test-2",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategoryData,
				Action:    "api_call",
				Actor:     AuditActor{ID: "client-123", Type: "client", Name: "client-123"},
				Resource:  AuditResource{ID: "api-1", Type: "endpoint", Name: "api"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						"endpoint": "/api/data",
						"token":    "Bearer abc123xyz",
					},
				},
			},
			expected: map[string]string{
				"token": "[REDACTED]",
			},
		},
		{
			name: "redact api_key in context metadata",
			event: &AuditEvent{
				ID:        "test-3",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategoryData,
				Action:    "api_call",
				Actor:     AuditActor{ID: "client-456", Type: "client", Name: "client-456"},
				Resource:  AuditResource{ID: "api-1", Type: "endpoint", Name: "api"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						"endpoint": "api/endpoint",
						"api_key":  "sk-1234567890abcdef",
					},
				},
			},
			expected: map[string]string{
				"api_key": "[REDACTED]",
			},
		},
		{
			name: "no sensitive data - unchanged",
			event: &AuditEvent{
				ID:        "test-4",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategoryData,
				Action:    "view",
				Actor:     AuditActor{ID: "user-1", Type: "user", Name: "user@example.com"},
				Resource:  AuditResource{ID: "doc-123", Type: "document", Name: "document"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						"document_id": "doc-123",
					},
				},
			},
			expected: map[string]string{
				"document_id": "doc-123",
			},
		},
		{
			name: "multiple sensitive fields",
			event: &AuditEvent{
				ID:        "test-5",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategoryAdmin,
				Action:    "create",
				Actor:     AuditActor{ID: "admin-1", Type: "admin", Name: "admin@example.com"},
				Resource:  AuditResource{ID: "user-123", Type: "user", Name: "user"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						"username": "newuser",
						"password": "mypassword123",
						"api_key":  "key-secret-789",
						"token":    "access-token-456",
					},
				},
			},
			expected: map[string]string{
				"password": "[REDACTED]",
				"api_key":  "[REDACTED]",
				"token":    "[REDACTED]",
				"username": "newuser",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.RedactEvent(tt.event)
			require.NoError(t, err)

			// Check expected values
			for key, expectedValue := range tt.expected {
				actualValue, ok := tt.event.Context.Metadata[key]
				require.True(t, ok, "Metadata key %s should exist", key)
				assert.Equal(t, expectedValue, actualValue, "Metadata key %s should be redacted correctly", key)
			}
		})
	}
}

func TestRedactor_RedactEvent_CaseInsensitive(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{"lowercase password", "password", "secret", "[REDACTED]"},
		{"uppercase PASSWORD", "PASSWORD", "secret", "[REDACTED]"},
		{"mixed case PaSsWoRd", "PaSsWoRd", "secret", "[REDACTED]"},
		{"lowercase token", "token", "abc123", "[REDACTED]"},
		{"uppercase TOKEN", "TOKEN", "abc123", "[REDACTED]"},
		{"mixed case ToKeN", "ToKeN", "abc123", "[REDACTED]"},
		{"lowercase secret", "secret", "shh", "[REDACTED]"},
		{"uppercase SECRET", "SECRET", "shh", "[REDACTED]"},
		{"lowercase api_key", "api_key", "key123", "[REDACTED]"},
		{"camelCase apiKey", "apiKey", "key123", "[REDACTED]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &AuditEvent{
				ID:        "test",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategorySystem,
				Action:    "test",
				Actor:     AuditActor{ID: "test", Type: "system"},
				Resource:  AuditResource{ID: "test", Type: "test"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						tt.key: tt.value,
					},
				},
			}

			err := r.RedactEvent(event)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, event.Context.Metadata[tt.key])
		})
	}
}

func TestRedactor_ConcurrentAccess(t *testing.T) {
	r := NewRedactor()

	// Test concurrent redaction
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			event := &AuditEvent{
				ID:        "test-concurrent",
				Timestamp: time.Now(),
				Level:     AuditLevelInfo,
				Category:  AuditCategorySystem,
				Action:    "test",
				Actor:     AuditActor{ID: "test", Type: "system"},
				Resource:  AuditResource{ID: "test", Type: "test"},
				Result:    AuditResult{Success: true},
				Context: AuditContext{
					Metadata: map[string]string{
						"password": "secret",
						"index":    string(rune('0' + idx)),
					},
				},
			}
			err := r.RedactEvent(event)
			require.NoError(t, err)
			assert.Equal(t, "[REDACTED]", event.Context.Metadata["password"])
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
