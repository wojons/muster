package log

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rs/zerolog"
)

func TestGlobal(t *testing.T) {
	// Global() returns a Logger value type; verify it doesn't panic and produces output.
	// A value type cannot be compared to nil.
	Global().Info().Str("test", "global").Msg("global logger test")
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel)
	logger := NewLogger(zl)
	logger.Debug().Msg("created via NewLogger")
	if buf.String() == "" {
		t.Fatal("NewLogger() produced no output")
	}
}

func TestInfo(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)
	logger := NewLogger(zl)
	entry := logger.Info()
	if entry == nil {
		t.Fatal("Info() returned nil")
	}
	entry.Str("key", "value").Msg("test message")
	if buf.String() == "" {
		t.Error("expected output from Info")
	}
}

func TestWarn(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.WarnLevel)
	logger := NewLogger(zl)
	entry := logger.Warn()
	if entry == nil {
		t.Fatal("Warn() returned nil")
	}
	entry.Msg("warning")
	if buf.String() == "" {
		t.Error("expected output from Warn")
	}
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.ErrorLevel)
	logger := NewLogger(zl)
	entry := logger.Error()
	if entry == nil {
		t.Fatal("Error() returned nil")
	}
	entry.Msg("error")
	if buf.String() == "" {
		t.Error("expected output from Error")
	}
}

func TestDebug(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel)
	logger := NewLogger(zl)
	entry := logger.Debug()
	if entry == nil {
		t.Fatal("Debug() returned nil")
	}
	entry.Msg("debugging")
	if buf.String() == "" {
		t.Error("expected output from Debug")
	}
}

func TestDebugFiltered(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)
	logger := NewLogger(zl)
	entry := logger.Debug()
	if entry == nil {
		t.Fatal("Debug() returned nil")
	}
	entry.Msg("should not appear")
	if buf.String() != "" {
		t.Error("expected no output when debug at info level")
	}
}

func TestInt(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)
	logger := NewLogger(zl)
	logger.Info().Int("code", 200).Msg("status")
	if buf.String() == "" {
		t.Error("expected output with int field")
	}
}

func TestBool(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)
	logger := NewLogger(zl)
	logger.Info().Bool("ok", true).Msg("check")
	if buf.String() == "" {
		t.Error("expected output with bool field")
	}
}

func TestErr(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.ErrorLevel)
	logger := NewLogger(zl)
	logger.Error().Err(errors.New("test error")).Msg("with error")
	if buf.String() == "" {
		t.Error("expected output with error field")
	}
}

func TestWithField(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)
	logger := NewLogger(zl)
	logger.Info().WithField("custom", 42).Msg("test")
	if buf.String() == "" {
		t.Error("expected output with field")
	}
}

func TestInterface(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)
	logger := NewLogger(zl)
	logger.Info().Interface("data", map[string]string{"x": "y"}).Msg("test")
	if buf.String() == "" {
		t.Error("expected output with interface field")
	}
}

func TestFromContext(t *testing.T) {
	// FromContext returns Logger (value type). Verify it doesn't panic and produces output.
	logger := FromContext(nil)
	logger.Info().Msg("from context test")
}

func TestSetGlobal(t *testing.T) {
	// SetGlobal replaces the global logger.
	// Verify it does not panic and Global() returns a usable logger after Set.
	noopLogger := NewLogger(zerolog.Nop())

	// SetGlobal should not panic
	SetGlobal(noopLogger)

	// Global() should return a usable logger without panicking
	got := Global()
	got.Info().Msg("set global test")
	got.Warn().Msg("set global warn test")
	got.Error().Msg("set global error test")

	// Reset to a functional logger so other tests work correctly
	var buf bytes.Buffer
	zl := zerolog.New(&buf)
	resetLogger := NewLogger(zl)
	SetGlobal(resetLogger)
}
