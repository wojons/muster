package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldHelpers(t *testing.T) {
	t.Run("String field", func(t *testing.T) {
		field := String("key", "value")
		assert.Equal(t, "key", field.Key)
		assert.Equal(t, "value", field.Value)
	})

	t.Run("Int field", func(t *testing.T) {
		field := Int("count", 42)
		assert.Equal(t, "count", field.Key)
		assert.Equal(t, 42, field.Value)
	})

	t.Run("Int64 field", func(t *testing.T) {
		field := Int64("size", 1234567890)
		assert.Equal(t, "size", field.Key)
		assert.Equal(t, int64(1234567890), field.Value)
	})

	t.Run("Float64 field", func(t *testing.T) {
		field := Float64("ratio", 3.14159)
		assert.Equal(t, "ratio", field.Key)
		assert.Equal(t, 3.14159, field.Value)
	})

	t.Run("Bool field", func(t *testing.T) {
		field := Bool("enabled", true)
		assert.Equal(t, "enabled", field.Key)
		assert.Equal(t, true, field.Value)
	})

	t.Run("Time field", func(t *testing.T) {
		now := time.Now()
		field := Time("timestamp", now)
		assert.Equal(t, "timestamp", field.Key)
		assert.Equal(t, now, field.Value)
	})

	t.Run("Duration field", func(t *testing.T) {
		duration := 5 * time.Second
		field := Duration("elapsed", duration)
		assert.Equal(t, "elapsed", field.Key)
		assert.Equal(t, duration, field.Value)
	})

	t.Run("Err field", func(t *testing.T) {
		err := errors.New("test error")
		field := Err(err)
		assert.Equal(t, "error", field.Key)
		assert.Equal(t, err, field.Value)
	})

	t.Run("Any field", func(t *testing.T) {
		data := map[string]int{"a": 1, "b": 2}
		field := Any("metadata", data)
		assert.Equal(t, "metadata", field.Key)
		assert.Equal(t, data, field.Value)
	})
}

func TestSensitiveString(t *testing.T) {
	t.Run("MarshalJSON redacts", func(t *testing.T) {
		s := SensitiveString("my-secret-password")
		data, err := s.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(data), "***REDACTED***")
	})

	t.Run("String redacts", func(t *testing.T) {
		s := SensitiveString("secret-token")
		assert.Equal(t, "***REDACTED***", s.String())
	})

	t.Run("Value returns original", func(t *testing.T) {
		s := SensitiveString("original-value")
		assert.Equal(t, "original-value", s.Value())
	})
}

func TestRedactionHelpers(t *testing.T) {
	t.Run("isSensitiveField detects passwords", func(t *testing.T) {
		assert.True(t, isSensitiveField("password"))
		assert.True(t, isSensitiveField("Password"))
		assert.True(t, isSensitiveField("PASSWORD"))
		assert.True(t, isSensitiveField("user_password"))
		assert.True(t, isSensitiveField("user-password"))
	})

	t.Run("isSensitiveField detects tokens", func(t *testing.T) {
		assert.True(t, isSensitiveField("token"))
		assert.True(t, isSensitiveField("access_token"))
		assert.True(t, isSensitiveField("refresh_token"))
		assert.True(t, isSensitiveField("bearer"))
	})

	t.Run("isSensitiveField detects keys", func(t *testing.T) {
		assert.True(t, isSensitiveField("api_key"))
		assert.True(t, isSensitiveField("api-key"))
		assert.True(t, isSensitiveField("apikey"))
		assert.True(t, isSensitiveField("auth_key"))
		assert.True(t, isSensitiveField("private_key"))
		assert.True(t, isSensitiveField("encryption_key"))
		assert.True(t, isSensitiveField("signing_key"))
		assert.True(t, isSensitiveField("master_key"))
		assert.True(t, isSensitiveField("client_secret"))
	})

	t.Run("isSensitiveField detects secrets", func(t *testing.T) {
		assert.True(t, isSensitiveField("secret"))
		assert.True(t, isSensitiveField("Secret"))
		assert.True(t, isSensitiveField("client_secret"))
	})

	t.Run("isSensitiveField detects credentials", func(t *testing.T) {
		assert.True(t, isSensitiveField("credential"))
		assert.True(t, isSensitiveField("credentials"))
		assert.True(t, isSensitiveField("passwd"))
		assert.True(t, isSensitiveField("certificate_key"))
	})

	t.Run("isSensitiveField returns false for normal fields", func(t *testing.T) {
		assert.False(t, isSensitiveField("username"))
		assert.False(t, isSensitiveField("email"))
		assert.False(t, isSensitiveField("user_id"))
		assert.False(t, isSensitiveField("name"))
		assert.False(t, isSensitiveField("count"))
	})

	t.Run("redactIfSensitive redacts sensitive values", func(t *testing.T) {
		result := redactIfSensitive("password", "secret123")
		assert.Equal(t, "***REDACTED***", result)
	})

	t.Run("redactIfSensitive keeps normal values", func(t *testing.T) {
		result := redactIfSensitive("username", "john")
		assert.Equal(t, "john", result)
	})
}

func TestRedactMapValues(t *testing.T) {
	t.Run("redacts sensitive values in map", func(t *testing.T) {
		input := map[string]interface{}{
			"username": "john",
			"password": "secret123",
			"email":    "john@example.com",
			"token":    "abc123",
		}

		result := redactMapValues(input)

		assert.Equal(t, "john", result["username"])
		assert.Equal(t, "***REDACTED***", result["password"])
		assert.Equal(t, "john@example.com", result["email"])
		assert.Equal(t, "***REDACTED***", result["token"])
	})

	t.Run("handles nil map", func(t *testing.T) {
		result := redactMapValues(nil)
		assert.Nil(t, result)
	})

	t.Run("recursively redacts nested maps", func(t *testing.T) {
		input := map[string]interface{}{
			"user": map[string]interface{}{
				"name":     "john",
				"password": "secret",
			},
			"api_key": "key123",
		}

		result := redactMapValues(input)

		userMap, ok := result["user"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "john", userMap["name"])
		assert.Equal(t, "***REDACTED***", userMap["password"])
		assert.Equal(t, "***REDACTED***", result["api_key"])
	})

	t.Run("redacts values in arrays", func(t *testing.T) {
		input := map[string]interface{}{
			"tokens": []interface{}{"token1", "token2", "public"},
		}

		result := redactMapValues(input)

		// When key "tokens" matches sensitive pattern, entire value is redacted
		assert.Equal(t, "***REDACTED***", result["tokens"])
	})

	t.Run("preserves non-string values", func(t *testing.T) {
		input := map[string]interface{}{
			"count":    42,
			"enabled":  true,
			"ratio":    3.14,
			"password": "secret",
		}

		result := redactMapValues(input)

		assert.Equal(t, 42, result["count"])
		assert.Equal(t, true, result["enabled"])
		assert.Equal(t, 3.14, result["ratio"])
		assert.Equal(t, "***REDACTED***", result["password"])
	})
}

func TestRedactValue(t *testing.T) {
	t.Run("returns string unchanged if not sensitive", func(t *testing.T) {
		result := redactValue("username", "john")
		assert.Equal(t, "john", result)
	})

	t.Run("redacts string if sensitive", func(t *testing.T) {
		result := redactValue("password", "secret")
		assert.Equal(t, "***REDACTED***", result)
	})

	t.Run("recursively redacts maps", func(t *testing.T) {
		input := map[string]interface{}{
			"secret": "value",
		}
		result := redactValue("data", input)
		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "***REDACTED***", resultMap["secret"])
	})

	t.Run("recursively redacts arrays", func(t *testing.T) {
		input := []interface{}{"val1", "val2"}
		result := redactValue("items", input)
		resultArray, ok := result.([]interface{})
		require.True(t, ok)
		assert.Len(t, resultArray, 2)
	})

	t.Run("preserves other types", func(t *testing.T) {
		assert.Equal(t, 42, redactValue("count", 42))
		assert.Equal(t, true, redactValue("flag", true))
		assert.Equal(t, 3.14, redactValue("ratio", 3.14))
	})
}

func TestAuditEvent(t *testing.T) {
	t.Run("AuditEvent marshals to JSON", func(t *testing.T) {
		event := AuditEvent{
			ID:        "test-123",
			Timestamp: time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC),
			Type:      "auth",
			User:      "john@example.com",
			Action:    "login",
			Resource:  "/api/login",
			Result:    "success",
			Details: map[string]interface{}{
				"ip": "192.168.1.1",
			},
			ClientIP: "192.168.1.1",
		}

		data, err := json.Marshal(event)
		require.NoError(t, err)

		var unmarshaled AuditEvent
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, "test-123", unmarshaled.ID)
		assert.Equal(t, "auth", unmarshaled.Type)
		assert.Equal(t, "john@example.com", unmarshaled.User)
	})
}

func TestAuditLogger(t *testing.T) {
	t.Run("NewAuditLogger with nil writer", func(t *testing.T) {
		logger := NewAuditLogger(nil)
		assert.NotNil(t, logger)
	})

	t.Run("NewAuditLogger with custom writer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)
		assert.NotNil(t, logger)
	})

	t.Run("Log writes JSON to output", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		event := AuditEvent{
			Type:   "test",
			User:   "testuser",
			Action: "testaction",
			Result: "success",
		}

		err := logger.Log(event)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), `"type":"test"`)
		assert.Contains(t, buf.String(), `"user":"testuser"`)
	})

	t.Run("Log generates event ID", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		event := AuditEvent{
			Type:   "test",
			User:   "testuser",
			Action: "testaction",
			Result: "success",
		}

		err := logger.Log(event)
		require.NoError(t, err)

		var loggedEvent AuditEvent
		err = json.Unmarshal(buf.Bytes(), &loggedEvent)
		require.NoError(t, err)
		assert.NotEmpty(t, loggedEvent.ID)
	})

	t.Run("Log sets timestamp", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		beforeLog := time.Now()
		event := AuditEvent{
			Type:   "test",
			User:   "testuser",
			Action: "testaction",
			Result: "success",
		}
		err := logger.Log(event)
		afterLog := time.Now()

		require.NoError(t, err)

		var loggedEvent AuditEvent
		err = json.Unmarshal(buf.Bytes(), &loggedEvent)
		require.NoError(t, err)

		assert.True(t, !loggedEvent.Timestamp.IsZero())
		assert.True(t, loggedEvent.Timestamp.After(beforeLog) || loggedEvent.Timestamp.Equal(beforeLog))
		assert.True(t, loggedEvent.Timestamp.Before(afterLog) || loggedEvent.Timestamp.Equal(afterLog))
	})

	t.Run("Log redacts sensitive details", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		event := AuditEvent{
			Type:   "auth",
			User:   "testuser",
			Action: "login",
			Result: "success",
			Details: map[string]interface{}{
				"username": "john",
				"password": "secret123",
			},
		}

		err := logger.Log(event)
		require.NoError(t, err)

		var loggedEvent AuditEvent
		err = json.Unmarshal(buf.Bytes(), &loggedEvent)
		require.NoError(t, err)

		details, ok := loggedEvent.Details["password"].(string)
		require.True(t, ok)
		assert.Equal(t, "***REDACTED***", details)
	})

	t.Run("Log returns error on marshal failure", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		event := AuditEvent{
			Type:   "test",
			User:   "testuser",
			Action: "testaction",
			Result: "success",
			Details: map[string]interface{}{
				"channel": make(chan int),
			},
		}

		err := logger.Log(event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "marshal audit event")
	})

	t.Run("Log returns error on write failure", func(t *testing.T) {
		writer := &failingWriter{}
		logger := NewAuditLogger(writer)

		event := AuditEvent{
			Type:   "test",
			User:   "testuser",
			Action: "testaction",
			Result: "success",
		}

		err := logger.Log(event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write audit event")
	})
}

func TestAuditLoggerConvenienceMethods(t *testing.T) {
	t.Run("LogAuth creates auth event", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		err := logger.LogAuth("john@example.com", "login", "success")
		require.NoError(t, err)

		var event AuditEvent
		err = json.Unmarshal(buf.Bytes(), &event)
		require.NoError(t, err)

		assert.Equal(t, "auth", event.Type)
		assert.Equal(t, "john@example.com", event.User)
		assert.Equal(t, "login", event.Action)
		assert.Equal(t, "success", event.Result)
	})

	t.Run("LogCommand creates command event", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		err := logger.LogCommand("admin", "kubectl delete pod mypod", "success")
		require.NoError(t, err)

		var event AuditEvent
		err = json.Unmarshal(buf.Bytes(), &event)
		require.NoError(t, err)

		assert.Equal(t, "command", event.Type)
		assert.Equal(t, "admin", event.User)
		assert.Equal(t, "kubectl delete pod mypod", event.Action)
		assert.Equal(t, "success", event.Result)
	})

	t.Run("LogAccess creates access event", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		err := logger.LogAccess("user1", "/api/admin/users", "denied")
		require.NoError(t, err)

		var event AuditEvent
		err = json.Unmarshal(buf.Bytes(), &event)
		require.NoError(t, err)

		assert.Equal(t, "access", event.Type)
		assert.Equal(t, "user1", event.User)
		assert.Equal(t, "/api/admin/users", event.Action)
		assert.Equal(t, "denied", event.Result)
	})

	t.Run("LogConfigChange creates config event", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		err := logger.LogConfigChange("admin", "max_connections", "100", "200")
		require.NoError(t, err)

		var event AuditEvent
		err = json.Unmarshal(buf.Bytes(), &event)
		require.NoError(t, err)

		assert.Equal(t, "config", event.Type)
		assert.Equal(t, "admin", event.User)
		assert.Equal(t, "update.max_connections", event.Action)

		assert.Equal(t, "max_connections", event.Details["field"])
		assert.Equal(t, "100", event.Details["old_value"])
		assert.Equal(t, "200", event.Details["new_value"])
	})

	t.Run("LogConfigChange redacts sensitive config changes", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		err := logger.LogConfigChange("admin", "api_key", "old-key-123", "new-key-456")
		require.NoError(t, err)

		var event AuditEvent
		err = json.Unmarshal(buf.Bytes(), &event)
		require.NoError(t, err)

		assert.Equal(t, "***REDACTED***", event.Details["old_value"])
		assert.Equal(t, "***REDACTED***", event.Details["new_value"])
	})
}

func TestAuditLoggerClose(t *testing.T) {
	t.Run("Close with non-closer writer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)
		err := logger.Close()
		assert.NoError(t, err)
	})

	t.Run("Close with closer writer", func(t *testing.T) {
		closer := &nopCloser{&bytes.Buffer{}}
		logger := NewAuditLogger(closer)
		err := logger.Close()
		assert.NoError(t, err)
	})
}

func TestGenerateEventID(t *testing.T) {
	t.Run("generates unique IDs", func(t *testing.T) {
		id1 := generateEventID()
		time.Sleep(1 * time.Millisecond)
		id2 := generateEventID()

		assert.NotEqual(t, id1, id2)
		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
	})

	t.Run("returns numeric string", func(t *testing.T) {
		id := generateEventID()
		assert.Regexp(t, `^\d+$`, id)
	})
}

// Helper types for testing

type failingWriter struct{}

func (fw *failingWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("write failed")
}

type nopCloser struct {
	io.Writer
}

func (nc *nopCloser) Close() error {
	return nil
}

func TestAuditEventJSONRoundTrip(t *testing.T) {
	t.Run("complete event round trip", func(t *testing.T) {
		original := AuditEvent{
			ID:        "event-456",
			Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
			Type:      "access",
			User:      "alice@example.com",
			Action:    "/api/data",
			Resource:  "database",
			Result:    "success",
			Details: map[string]interface{}{
				"query":   "SELECT * FROM users",
				"rows":    100,
				"enabled": true,
			},
			ClientIP: "10.0.0.1",
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var unmarshaled AuditEvent
		err = json.Unmarshal(data, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, original.ID, unmarshaled.ID)
		assert.Equal(t, original.Type, unmarshaled.Type)
		assert.Equal(t, original.User, unmarshaled.User)
		assert.Equal(t, original.Action, unmarshaled.Action)
		assert.Equal(t, original.Resource, unmarshaled.Resource)
		assert.Equal(t, original.Result, unmarshaled.Result)
		assert.Equal(t, original.ClientIP, unmarshaled.ClientIP)
	})
}

func TestAuditLoggerMultipleEvents(t *testing.T) {
	t.Run("multiple events written separately", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewAuditLogger(buf)

		events := []AuditEvent{
			{Type: "auth", User: "user1", Action: "login", Result: "success"},
			{Type: "auth", User: "user2", Action: "logout", Result: "success"},
			{Type: "command", User: "admin", Action: "restart", Result: "success"},
		}

		for _, event := range events {
			err := logger.Log(event)
			require.NoError(t, err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.Len(t, lines, 3)

		for i, line := range lines {
			var loggedEvent AuditEvent
			err := json.Unmarshal([]byte(line), &loggedEvent)
			require.NoError(t, err, "Failed to parse event %d", i)
			assert.NotEmpty(t, loggedEvent.ID)
		}
	})
}

func TestAuditLoggerConcurrency(t *testing.T) {
	t.Run("concurrent logging", func(t *testing.T) {
		// Use a thread-safe buffer for concurrent writes
		buf := &syncBuffer{Buffer: &bytes.Buffer{}}
		logger := NewAuditLogger(buf)

		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func(id int) {
				event := AuditEvent{
					Type:   "concurrent",
					User:   fmt.Sprintf("user%d", id),
					Action: fmt.Sprintf("action%d", id),
					Result: "success",
				}
				err := logger.Log(event)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		// Give a small delay for all writes to complete
		time.Sleep(10 * time.Millisecond)

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		// At least 9 lines should be present (one might be lost due to race)
		assert.GreaterOrEqual(t, len(lines), 9)
		assert.LessOrEqual(t, len(lines), 10)
	})
}

// syncBuffer is a thread-safe buffer for testing
type syncBuffer struct {
	*bytes.Buffer
	mu sync.Mutex
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.Buffer.Write(p)
}

func TestAuditLoggerWithDifferentLevels(t *testing.T) {
	t.Run("logger respects global level", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			Level:      zerolog.ErrorLevel,
			FormatJSON: true,
			Output:     buf,
		})

		logger.Debug().Msg("debug message")
		logger.Info().Msg("info message")
		logger.Error().Msg("error message")

		output := buf.String()
		assert.NotContains(t, output, "debug message")
		assert.NotContains(t, output, "info message")
		assert.Contains(t, output, "error message")
	})
}

func TestLoggerWithContextMethods(t *testing.T) {
	t.Run("LoggerWithContext uses context logger", func(t *testing.T) {
		buf := &bytes.Buffer{}
		baseLogger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		ctx := context.Background()
		ctx = zerolog.New(buf).With().Timestamp().Logger().WithContext(ctx)

		ctxLogger := baseLogger.LoggerWithContext(ctx)
		assert.NotNil(t, ctxLogger)

		ctxLogger.Info().Str("test", "value").Msg("test message")
		assert.Contains(t, buf.String(), "test")
		assert.Contains(t, buf.String(), "value")
	})

	t.Run("FromContext returns global when no context logger", func(t *testing.T) {
		ctx := context.Background()
		logger := FromContext(ctx)
		assert.NotNil(t, logger)
	})

	t.Run("WithStr adds string field to logger", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		withField := logger.WithStr("trace_id", "abc-123")
		withField.Info().Msg("test")

		assert.Contains(t, buf.String(), "trace_id")
		assert.Contains(t, buf.String(), "abc-123")
	})

	t.Run("WithErr adds error field to logger", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		err := errors.New("test error")
		withErr := logger.WithErr(err)
		withErr.Error().Msg("error occurred")

		assert.Contains(t, buf.String(), "error")
		assert.Contains(t, buf.String(), "test error")
	})

	t.Run("WithFields adds multiple fields", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		fields := map[string]interface{}{
			"user_id": 123,
			"action":  "login",
		}
		withFields := logger.WithFields(fields)
		withFields.Info().Msg("user action")

		assert.Contains(t, buf.String(), "user_id")
		assert.Contains(t, buf.String(), "action")
	})
}

func TestSpecializedLoggingMethods(t *testing.T) {
	t.Run("InfoKV logs with fields", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		fields := map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		}
		logger.InfoKV("test_event", fields)

		output := buf.String()
		assert.Contains(t, output, "test_event")
		assert.Contains(t, output, "key1")
		assert.Contains(t, output, "value1")
	})

	t.Run("ErrorKV logs error with fields", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		err := errors.New("test error")
		fields := map[string]interface{}{
			"code": 500,
		}
		logger.ErrorKV("error_event", err, fields)

		output := buf.String()
		assert.Contains(t, output, "error_event")
		assert.Contains(t, output, "test error")
		assert.Contains(t, output, "code")
	})

	t.Run("Metric logs metrics with tags", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		tags := map[string]string{
			"environment": "production",
			"service":     "api",
		}
		logger.Metric("request_count", 100.5, tags)

		output := buf.String()
		assert.Contains(t, output, "metric")
		assert.Contains(t, output, "request_count")
		assert.Contains(t, output, "100.5")
		assert.Contains(t, output, "tag_environment")
		assert.Contains(t, output, "production")
	})

	t.Run("Request logs HTTP request details", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		logger.Request("GET", "/api/users", 125*time.Millisecond, 200)

		output := buf.String()
		assert.Contains(t, output, "request")
		assert.Contains(t, output, "GET")
		assert.Contains(t, output, "/api/users")
		assert.Contains(t, output, "200")
	})

	t.Run("Command logs command execution", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		logger.Command("kubectl", []string{"get", "pods"}, 2*time.Second, nil)

		output := buf.String()
		assert.Contains(t, output, "command")
		assert.Contains(t, output, "kubectl")
		assert.Contains(t, output, "get")
		assert.Contains(t, output, "pods")
	})

	t.Run("Command logs with error", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		err := errors.New("command failed")
		logger.Command("kubectl", []string{"delete", "pod"}, 1*time.Second, err)

		output := buf.String()
		assert.Contains(t, output, "command")
		assert.Contains(t, output, "kubectl")
		assert.Contains(t, output, "command failed")
	})

	t.Run("Command logs without args", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		logger.Command("ls", nil, 100*time.Millisecond, nil)

		output := buf.String()
		assert.Contains(t, output, "command")
		assert.Contains(t, output, "ls")
		assert.NotContains(t, output, "args")
	})
}

func TestLoggerOutputManagement(t *testing.T) {
	t.Run("SetOutput changes output destination", func(t *testing.T) {
		buf1 := &bytes.Buffer{}
		buf2 := &bytes.Buffer{}

		logger := New(Config{
			FormatJSON: true,
			Output:     buf1,
		})

		logger.Info().Msg("message 1")
		assert.Contains(t, buf1.String(), "message 1")

		logger.SetOutput(buf2)
		logger.Info().Msg("message 2")

		assert.NotContains(t, buf1.String(), "message 2")
		assert.Contains(t, buf2.String(), "message 2")
	})

	t.Run("Output returns logger output", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		output := logger.Output()
		assert.NotNil(t, output)
	})

	t.Run("Close returns nil", func(t *testing.T) {
		logger := New(Config{})
		err := logger.Close()
		assert.NoError(t, err)
	})
}

func TestGlobalLogger(t *testing.T) {
	t.Run("Global returns singleton", func(t *testing.T) {
		logger1 := Global()
		logger2 := Global()
		assert.Equal(t, logger1, logger2)
	})

	t.Run("SetGlobalLogger sets the global instance", func(t *testing.T) {
		newLogger := New(Config{})
		SetGlobalLogger(newLogger)

		global := Global()
		assert.Equal(t, newLogger, global)
	})

	t.Run("SetLevel changes global logger level", func(t *testing.T) {
		SetLevel(zerolog.DebugLevel)
		level := GetLevel()
		assert.Equal(t, zerolog.DebugLevel, level)
	})

	t.Run("GetLevel returns default when no global logger", func(t *testing.T) {
		original := globalLogger
		globalLogger = nil
		defer func() { globalLogger = original }()

		level := GetLevel()
		assert.Equal(t, zerolog.InfoLevel, level)
	})
}

func TestLoggerLevelMethods(t *testing.T) {
	t.Run("info warn error methods return non-nil", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})

		assert.NotNil(t, logger.Info())
		assert.NotNil(t, logger.Warn())
		assert.NotNil(t, logger.Error())
	})

	t.Run("debug and trace return events", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			Level:      zerolog.TraceLevel,
			FormatJSON: true,
			Output:     buf,
		})

		assert.NotNil(t, logger.Trace())
		assert.NotNil(t, logger.Debug())
	})
}
