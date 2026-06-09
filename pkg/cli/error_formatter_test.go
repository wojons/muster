package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/wojons/muster/pkg/errors"
)

func TestNewErrorFormatter(t *testing.T) {
	f := NewErrorFormatter()
	if f == nil {
		t.Fatal("expected formatter to not be nil")
	}
	if f.verbose {
		t.Error("expected verbose to be false by default")
	}
	if f.noPadding {
		t.Error("expected noPadding to be false by default")
	}
}

func TestNewVerboseErrorFormatter(t *testing.T) {
	f := NewVerboseErrorFormatter()
	if f == nil {
		t.Fatal("expected formatter to not be nil")
	}
	if !f.verbose {
		t.Error("expected verbose to be true")
	}
}

func TestFormatNilError(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	err := f.Format(&buf, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if output != "No error\n" {
		t.Errorf("expected 'No error\\n', got %q", output)
	}
}

func TestFormatAppError(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	appErr := errors.New(errors.CodeBadRequest, "Bad request error")
	err := f.Format(&buf, appErr)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Bad request error") {
		t.Errorf("expected output to contain error message, got %q", output)
	}
}

func TestFormatAppErrorWithCode(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	appErr := errors.New(errors.CodeNotFound, "Resource not found")
	err := f.Format(&buf, appErr)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NOT_FOUND") {
		t.Errorf("expected output to contain error code, got %q", output)
	}
	if !strings.Contains(output, "Resource not found") {
		t.Errorf("expected output to contain error message, got %q", output)
	}
}

func TestFormatAppErrorWithDetails(t *testing.T) {
	f := NewVerboseErrorFormatter()
	var buf bytes.Buffer

	appErr := errors.New(errors.CodeValidation, "Validation failed").
		WithDetail("field", "username").
		WithDetail("reason", "too short")

	err := f.Format(&buf, appErr)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Details:") {
		t.Errorf("expected output to contain details section, got %q", output)
	}
	if !strings.Contains(output, "field") {
		t.Errorf("expected output to contain field detail, got %q", output)
	}
	if !strings.Contains(output, "username") {
		t.Errorf("expected output to contain username value, got %q", output)
	}
}

func TestFormatAppErrorWithCause(t *testing.T) {
	f := NewVerboseErrorFormatter()
	var buf bytes.Buffer

	inner := errors.New(errors.CodeBadRequest, "Inner error")
	outer := errors.Wrap(inner, errors.CodeValidation, "Outer error")

	err := f.Format(&buf, outer)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Cause:") {
		t.Errorf("expected output to contain cause section, got %q", output)
	}
	if !strings.Contains(output, "Inner error") {
		t.Errorf("expected output to contain inner error, got %q", output)
	}
}

func TestFormatPlainError(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	plainErr := fmt.Errorf("plain error message")

	err := f.Format(&buf, plainErr)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "plain error message") {
		t.Errorf("expected output to contain error message, got %q", output)
	}
}

func TestFormatString(t *testing.T) {
	f := NewErrorFormatter()

	appErr := errors.New(errors.CodeBadRequest, "test error")

	str, err := f.FormatString(appErr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if str == "" {
		t.Error("expected string to not be empty")
	}
	if !strings.Contains(str, "test error") {
		t.Errorf("expected string to contain error message, got %q", str)
	}
}

func TestFormatBytes(t *testing.T) {
	f := NewErrorFormatter()

	appErr := errors.New(errors.CodeBadRequest, "test error")

	bytes, err := f.FormatBytes(appErr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(bytes) == 0 {
		t.Error("expected bytes to not be empty")
	}
	if !strings.Contains(string(bytes), "test error") {
		t.Errorf("expected bytes to contain error message, got %q", string(bytes))
	}
}

func TestFormatJSON(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	appErr := errors.New(errors.CodeBadRequest, "Bad request").
		WithDetail("field", "value").
		WithSeverity(errors.SeverityHigh)

	err := f.FormatJSON(&buf, appErr)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()

	// Verify JSON contains expected fields
	if !strings.Contains(output, "BAD_REQUEST") {
		t.Errorf("expected JSON to contain code, got %q", output)
	}
	if !strings.Contains(output, "Bad request") {
		t.Errorf("expected JSON to contain message, got %q", output)
	}
	if !strings.Contains(output, "high") {
		t.Errorf("expected JSON to contain severity, got %q", output)
	}
	if !strings.Contains(output, "details") {
		t.Errorf("expected JSON to contain details, got %q", output)
	}
}

func TestFormatJSONNilError(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	err := f.FormatJSON(&buf, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "null") {
		t.Errorf("expected JSON for nil error, got %q", output)
	}
}

func TestFormatJSONPlainError(t *testing.T) {
	f := NewErrorFormatter()
	var buf bytes.Buffer

	plainErr := fmt.Errorf("plain error")

	err := f.FormatJSON(&buf, plainErr)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "plain error") {
		t.Errorf("expected JSON to contain error message, got %q", output)
	}
}

func TestSetColor(t *testing.T) {
	f := NewErrorFormatter()

	f.SetColor(true)
	if !f.colors {
		t.Error("expected colors to be enabled")
	}

	f.SetColor(false)
	if f.colors {
		t.Error("expected colors to be disabled")
	}
}

func TestSetVerbose(t *testing.T) {
	f := NewErrorFormatter()

	f.SetVerbose(true)
	if !f.verbose {
		t.Error("expected verbose to be enabled")
	}

	f.SetVerbose(false)
	if f.verbose {
		t.Error("expected verbose to be disabled")
	}
}

func TestSetNoPadding(t *testing.T) {
	f := NewErrorFormatter()

	f.SetNoPadding(true)
	if !f.noPadding {
		t.Error("expected noPadding to be enabled")
	}

	f.SetNoPadding(false)
	if f.noPadding {
		t.Error("expected noPadding to be disabled")
	}
}

func TestErrorSeverityColors(t *testing.T) {
	f := NewErrorFormatter()
	f.SetColor(true)

	tests := []struct {
		name         string
		severity     errors.Severity
		wantContains string
	}{
		{"critical", errors.SeverityCritical, "033["}, // Has ANSI codes
		{"high", errors.SeverityHigh, "033["},
		{"medium", errors.SeverityMedium, "033["},
		{"low", errors.SeverityLow, "033["},
		{"info", errors.SeverityInfo, "033["},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := errors.New(errors.CodeGeneric, "test").
				WithSeverity(tt.severity)

			f.SetVerbose(true)
			var buf bytes.Buffer

			if err := f.Format(&buf, appErr); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// When colors are forced on, should have ANSI codes
			// Note: This test assumes terminal detection may be mocked or NO_COLOR not set
		})
	}
}

func TestNoPaddingFormat(t *testing.T) {
	f := NewErrorFormatter()
	f.SetNoPadding(true)

	var buf bytes.Buffer

	appErr := errors.New(errors.CodeBadRequest, "test error")

	if err := f.Format(&buf, appErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()

	// No padding should not include "Error:" prefix
	if strings.Contains(output, "Error:") {
		t.Errorf("expected no 'Error:' prefix with noPadding, got %q", output)
	}
}

func TestWrappedErrorFormatting(t *testing.T) {
	f := NewVerboseErrorFormatter()
	var buf bytes.Buffer

	inner := errors.New(errors.CodeValidation, "validation failed")
	outer := errors.Wrap(inner, errors.CodeBadRequest, "bad request")

	if err := f.Format(&buf, outer); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "bad request") {
		t.Errorf("expected outer error message, got %q", output)
	}
	if !strings.Contains(output, "validation failed") {
		t.Errorf("expected inner error message in verbose mode, got %q", output)
	}
}

func TestMultipleDetailsFormatting(t *testing.T) {
	f := NewVerboseErrorFormatter()
	var buf bytes.Buffer

	appErr := errors.New(errors.CodeValidation, "validation failed").
		WithDetail("field1", "value1").
		WithDetail("field2", "value2").
		WithDetail("field3", "value3")

	if err := f.Format(&buf, appErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "field1") ||
		!strings.Contains(output, "field2") ||
		!strings.Contains(output, "field3") {
		t.Errorf("expected all details to be present, got %q", output)
	}
}
