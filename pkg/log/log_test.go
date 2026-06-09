package log

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
)

func TestGlobal(t *testing.T) {
	if Global() == nil {
		t.Fatal("Global() returned nil")
	}
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.DebugLevel)
	if logger == nil {
		t.Fatal("New() returned nil")
	}
}

func TestInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.InfoLevel)
	entry := logger.Info()
	if entry == nil {
		t.Fatal("Info() returned nil")
	}
	entry.Str("key", "value").Msg("test message")
	output := buf.String()
	if output == "" {
		t.Error("expected output from Info")
	}
}

func TestWarn(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.WarnLevel)
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
	logger := New(&buf, zerolog.ErrorLevel)
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
	logger := New(&buf, zerolog.DebugLevel)
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
	logger := New(&buf, zerolog.InfoLevel)
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
	logger := New(&buf, zerolog.InfoLevel)
	logger.Info().Int("code", 200).Msg("status")
	if buf.String() == "" {
		t.Error("expected output with int field")
	}
}

func TestBool(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.InfoLevel)
	logger.Info().Bool("ok", true).Msg("check")
	if buf.String() == "" {
		t.Error("expected output with bool field")
	}
}

func TestErr(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.ErrorLevel)
	logger.Error().Err(zerolog.ErrNilLog).Msg("with error") // ErrNilLog is a harmless sentinel
	if buf.String() == "" {
		t.Error("expected output with error field")
	}
}

func TestWithField(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.InfoLevel)
	logger.Info().WithField("custom", 42).Msg("test")
	if buf.String() == "" {
		t.Error("expected output with field")
	}
}

func TestInterface(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.InfoLevel)
	logger.Info().Interface("data", map[string]string{"x": "y"}).Msg("test")
	if buf.String() == "" {
		t.Error("expected output with interface field")
	}
}

func TestFromContext(t *testing.T) {
	// Without context, should fall back to global
	logger := FromContext(nil)
	if logger == nil {
		t.Fatal("FromContext(nil) returned nil")
	}
}

func TestWithContext(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, zerolog.DebugLevel)
	ctx := WithContext(nil, logger)
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("FromContext returned nil")
	}
}
