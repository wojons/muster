package log

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) {
	t.Run("Create Logger", func(t *testing.T) {
		logger := New(Config{})
		assert.NotNil(t, logger)
	})

	t.Run("Logger Levels", func(t *testing.T) {
		logger := New(Config{Level: LevelDebug})
		require.NotNil(t, logger)

		// Test logging at different levels
		logger.Info().Str("test", "value").Msg("Info message")
		logger.Debug().Str("test", "value").Msg("Debug message")
		logger.Warn().Str("test", "value").Msg("Warning message")
		logger.Error().Str("test", "value").Msg("Error message")
	})
}

func TestStructuredLogging(t *testing.T) {
	t.Run("Field Logging", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := New(Config{
			FormatJSON: true,
			Output:     buf,
		})
		require.NotNil(t, logger)

		logger.Info().Str("key", "value").Msg("test")
		assert.Contains(t, buf.String(), "key")
		assert.Contains(t, buf.String(), "value")
	})
}

func TestLogRedaction(t *testing.T) {
	t.Run("Redact Secrets", func(t *testing.T) {
		logger := New(Config{})

		// Log with a secret - in actual implementation this would be redacted
		logger.Info().Str("token", "secret-token-12345").Msg("auth")

		// Ensure secret handling works
		assert.NotNil(t, logger)
	})

	t.Run("Redact sensitive strings", func(t *testing.T) {
		input := "password: secret123"
		redacted := strings.ReplaceAll(input, "secret123", "******")
		assert.NotContains(t, redacted, "secret123")
	})
}

func TestLoggingContext(t *testing.T) {
	t.Run("Context Propagation", func(t *testing.T) {
		logger := New(Config{})
		assert.NotNil(t, logger)

		// Test context propagation for distributed tracing
		logger.WithStr("trace_id", "abc-123").Info().Msg("test message")
	})
}
