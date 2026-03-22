package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		setup   func()
		cleanup func()
	}{
		{
			name: "valid config",
			config: Config{
				Enabled:         true,
				StorageType:     "file",
				RetentionDays:   365,
				BufferSize:      1000,
				FlushInterval:   time.Second * 5,
				SensitiveFields: []string{"password"},
			},
			wantErr: false,
		},
		{
			name: "missing sensitive fields adds defaults",
			config: Config{
				Enabled:         true,
				StorageType:     "file",
				SensitiveFields: []string{},
			},
			wantErr: false,
		},
		{
			name: "include body in production fails",
			config: Config{
				Enabled:     true,
				IncludeBody: true,
			},
			wantErr: true,
			setup: func() {
				os.Setenv("ENVIRONMENT", "production")
			},
			cleanup: func() {
				os.Unsetenv("ENVIRONMENT")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify mandatory fields are present
				mandatory := []string{"password", "token", "secret", "api_key"}
				for _, field := range mandatory {
					found := false
					for _, sf := range tt.config.SensitiveFields {
						if strings.EqualFold(sf, field) {
							found = true
							break
						}
					}
					assert.True(t, found, "mandatory field %s should be in sensitive fields", field)
				}
			}
		})
	}
}

func TestRedactor_RedactEvent(t *testing.T) {
	redactor := NewRedactor()

	tests := []struct {
		name     string
		event    *Event
		expected map[string]interface{}
	}{
		{
			name: "redact password in context",
			event: &Event{
				Action: "login",
				Context: map[string]interface{}{
					"username": "testuser",
					"password": "secret123",
				},
			},
			expected: map[string]interface{}{
				"username": "testuser",
				"password": "[REDACTED]",
			},
		},
		{
			name: "redact token in context",
			event: &Event{
				Action: "api_call",
				Context: map[string]interface{}{
					"endpoint": "/api/data",
					"token":    "bearer_token_123",
				},
			},
			expected: map[string]interface{}{
				"endpoint": "/api/data",
				"token":    "[REDACTED]",
			},
		},
		{
			name: "redact secret in context",
			event: &Event{
				Action: "config_update",
				Context: map[string]interface{}{
					"api_key": "my_api_key",
					"secret":  "my_secret",
				},
			},
			expected: map[string]interface{}{
				"api_key": "[REDACTED]",
				"secret":  "[REDACTED]",
			},
		},
		{
			name: "redact nested sensitive fields",
			event: &Event{
				Action: "user_update",
				Context: map[string]interface{}{
					"user": map[string]interface{}{
						"name":     "John",
						"password": "john_secret",
					},
				},
			},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"name":     "John",
					"password": "[REDACTED]",
				},
			},
		},
		{
			name: "redact changes fields",
			event: &Event{
				Action: "user_update",
				Changes: []Change{
					{Field: "username", OldValue: "old", NewValue: "new"},
					{Field: "password", OldValue: "old_pass", NewValue: "new_pass"},
				},
			},
			expected: map[string]interface{}{
				"username_change": "unchanged",
				"password_change": "redacted",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := redactor.RedactEvent(tt.event)
			require.NoError(t, err)

			if tt.expected != nil {
				if tt.event.Context != nil {
					for key, expectedVal := range tt.expected {
						if actualVal, ok := tt.event.Context[key]; ok {
							assert.Equal(t, expectedVal, actualVal, "field %s", key)
						}
					}
				}
			}

			// Verify changes redaction
			for _, change := range tt.event.Changes {
				if change.Field == "password" {
					assert.Equal(t, "[REDACTED]", change.OldValue)
					assert.Equal(t, "[REDACTED]", change.NewValue)
				}
			}
		})
	}
}

func TestLogger_Log(t *testing.T) {
	// Create temporary file for testing
	tmpFile, err := os.CreateTemp("", "audit_test_*.jsonl")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	config := Config{
		Enabled:         true,
		StorageType:     "file",
		BufferSize:      10,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password", "token"},
	}

	storage, err := NewFileStorage(tmpFile.Name())
	require.NoError(t, err)

	logger, err := NewLogger(config, storage)
	require.NoError(t, err)
	defer logger.Close()

	ctx := context.Background()

	t.Run("log event with credential redaction", func(t *testing.T) {
		event := &Event{
			Level:    LevelInfo,
			Category: CategoryAuth,
			Action:   "user_login",
			Actor: Actor{
				ID:        "user123",
				Type:      "user",
				Name:      "Test User",
				IPAddress: "192.168.1.1",
			},
			Resource: Resource{
				ID:   "session_abc",
				Type: "session",
			},
			Result: Result{
				Success: true,
				Message: "Login successful",
			},
			Context: map[string]interface{}{
				"username": "testuser",
				"password": "super_secret_password",
				"token":    "auth_token_123",
			},
		}

		err := logger.Log(ctx, event)
		require.NoError(t, err)

		// Flush to ensure it's written
		err = logger.Flush()
		require.NoError(t, err)

		// Read the file and verify
		content, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)

		var loggedEvent Event
		err = json.Unmarshal(content, &loggedEvent)
		require.NoError(t, err)

		// Verify credentials are redacted
		assert.Equal(t, "[REDACTED]", loggedEvent.Context["password"])
		assert.Equal(t, "[REDACTED]", loggedEvent.Context["token"])
		assert.Equal(t, "testuser", loggedEvent.Context["username"])

		// Verify chain of custody fields
		assert.NotEmpty(t, loggedEvent.ID)
		assert.NotEmpty(t, loggedEvent.Hash)
		assert.NotZero(t, loggedEvent.Timestamp)
	})

	t.Run("log event without sensitive data", func(t *testing.T) {
		event := &Event{
			Level:    LevelInfo,
			Category: CategorySystem,
			Action:   "system_startup",
			Actor: Actor{
				ID:   "system",
				Type: "system",
			},
			Resource: Resource{
				ID:   "muster",
				Type: "application",
			},
			Result: Result{
				Success: true,
			},
			Context: map[string]interface{}{
				"version": "1.0.0",
				"pid":     12345,
			},
		}

		err := logger.Log(ctx, event)
		require.NoError(t, err)

		err = logger.Flush()
		require.NoError(t, err)
	})
}

func TestLogger_Log_Disabled(t *testing.T) {
	config := Config{
		Enabled: false,
	}

	storage := &mockStorage{}
	logger, err := NewLogger(config, storage)
	require.NoError(t, err)
	defer logger.Close()

	ctx := context.Background()
	event := &Event{
		Action: "test_action",
	}

	// Should not error when disabled
	err = logger.Log(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, 0, storage.writeCount)
}

func TestLogger_ChainOfCustody(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_chain_test_*.jsonl")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	config := Config{
		Enabled:         true,
		StorageType:     "file",
		BufferSize:      1,
		FlushInterval:   time.Hour, // Long interval to test manual flush
		SensitiveFields: []string{"password"},
	}

	storage, err := NewFileStorage(tmpFile.Name())
	require.NoError(t, err)

	logger, err := NewLogger(config, storage)
	require.NoError(t, err)
	defer logger.Close()

	ctx := context.Background()

	// Log multiple events
	var previousHash string
	for i := 0; i < 3; i++ {
		event := &Event{
			Level:    LevelInfo,
			Category: CategorySystem,
			Action:   fmt.Sprintf("action_%d", i),
			Actor:    Actor{ID: "test"},
			Resource: Resource{ID: "test"},
		}

		err := logger.Log(ctx, event)
		require.NoError(t, err)

		// Verify chain of custody
		if i > 0 {
			assert.Equal(t, previousHash, event.PreviousHash,
				"event %d should reference previous hash", i)
		}
		assert.NotEmpty(t, event.Hash, "event %d should have hash", i)
		previousHash = event.Hash
	}
}

func TestFileStorage(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_storage_test_*.jsonl")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	storage, err := NewFileStorage(tmpFile.Name())
	require.NoError(t, err)
	defer storage.Close()

	t.Run("write and read event", func(t *testing.T) {
		event := &Event{
			ID:        "test-123",
			Timestamp: time.Now().UTC(),
			Action:    "test_action",
			Actor:     Actor{ID: "user1", IPAddress: "127.0.0.1"},
			Resource:  Resource{ID: "res1", Type: "test"},
			Hash:      "abc123",
		}

		err := storage.Write(event)
		require.NoError(t, err)

		// Read back
		content, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)

		var loggedEvent Event
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if line != "" {
				err = json.Unmarshal([]byte(line), &loggedEvent)
				require.NoError(t, err)
				break
			}
		}

		assert.Equal(t, event.ID, loggedEvent.ID)
		assert.Equal(t, event.Action, loggedEvent.Action)
		assert.Equal(t, event.Hash, loggedEvent.Hash)
	})

	t.Run("query returns error", func(t *testing.T) {
		events, err := storage.Query(Filter{})
		assert.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "not supported")
	})
}

func TestDatabaseStorage(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create audit table
	_, err = db.Exec(`
		CREATE TABLE audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action VARCHAR(255) NOT NULL,
			entity_type VARCHAR(255) NOT NULL,
			entity_id INTEGER NOT NULL,
			user_id VARCHAR(255),
			data BLOB,
			ip_address VARCHAR(45),
			user_agent TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	storage, err := NewDatabaseStorage(db)
	require.NoError(t, err)
	defer storage.Close()

	t.Run("write event to database", func(t *testing.T) {
		event := &Event{
			ID:        "test-123",
			Timestamp: time.Now().UTC(),
			Action:    "user_login",
			Actor: Actor{
				ID:        "user123",
				IPAddress: "192.168.1.1",
				UserAgent: "TestAgent/1.0",
			},
			Resource: Resource{
				ID:   "456",
				Type: "session",
			},
			Context: map[string]interface{}{
				"login_method": "password",
			},
		}

		err := storage.Write(event)
		require.NoError(t, err)

		// Verify by querying
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM audit WHERE action = ?", "user_login").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("query events from database", func(t *testing.T) {
		// Insert multiple events
		for i := 0; i < 3; i++ {
			event := &Event{
				ID:        fmt.Sprintf("test-%d", i),
				Timestamp: time.Now().UTC(),
				Action:    fmt.Sprintf("action_%d", i),
				Actor:     Actor{ID: fmt.Sprintf("user%d", i), IPAddress: "127.0.0.1"},
				Resource:  Resource{ID: fmt.Sprintf("res%d", i), Type: "test"},
			}
			err := storage.Write(event)
			require.NoError(t, err)
		}

		// Query by actor
		events, err := storage.Query(Filter{ActorID: "user1"})
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "action_1", events[0].Action)

		// Query by resource type
		events, err = storage.Query(Filter{ResourceType: "test"})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(events), 3)
	})
}

func TestDefaultRedactionRules(t *testing.T) {
	rules := DefaultRedactionRules()
	require.NotEmpty(t, rules)

	// Verify mandatory rules exist
	mandatoryPatterns := []string{"password", "token", "secret", "credit_card"}
	for _, pattern := range mandatoryPatterns {
		found := false
		for _, rule := range rules {
			if strings.Contains(rule.Pattern, pattern) {
				found = true
				assert.True(t, rule.Mandatory, "rule for %s should be mandatory", pattern)
				break
			}
		}
		assert.True(t, found, "should have redaction rule for %s", pattern)
	}
}

// mockStorage is a mock implementation for testing
type mockStorage struct {
	events     []*Event
	writeCount int
	queryCount int
	closeCount int
}

func (m *mockStorage) Write(event *Event) error {
	m.events = append(m.events, event)
	m.writeCount++
	return nil
}

func (m *mockStorage) Query(filter Filter) ([]*Event, error) {
	m.queryCount++
	return m.events, nil
}

func (m *mockStorage) Close() error {
	m.closeCount++
	return nil
}
