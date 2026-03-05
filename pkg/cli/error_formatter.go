package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openapi-cli/openapi-cli/pkg/errors"
)

// ErrorFormatter formats AppError for CLI output.
type ErrorFormatter struct {
	colors       bool
	verbose      bool
	noPadding    bool
	forceColor   bool
	forceNoColor bool
}

// NewErrorFormatter creates a new error formatter.
func NewErrorFormatter() *ErrorFormatter {
	return &ErrorFormatter{
		colors:    shouldColorize(),
		verbose:   false,
		noPadding: false,
	}
}

// NewVerboseErrorFormatter creates a verbose error formatter.
func NewVerboseErrorFormatter() *ErrorFormatter {
	return &ErrorFormatter{
		colors:    shouldColorize(),
		verbose:   true,
		noPadding: false,
	}
}

// SetColor enables or disables color output.
func (f *ErrorFormatter) SetColor(enabled bool) {
	f.colors = enabled
	f.forceColor = enabled
	f.forceNoColor = !enabled
}

// SetVerbose enables or disables verbose output.
func (f *ErrorFormatter) SetVerbose(enabled bool) {
	f.verbose = enabled
}

// SetNoPadding enables or disables padding.
func (f *ErrorFormatter) SetNoPadding(enabled bool) {
	f.noPadding = enabled
}

// Format writes formatted error to writer.
func (f *ErrorFormatter) Format(w io.Writer, err error) error {
	if err == nil {
		fmt.Fprintln(w, "No error")
		return nil
	}

	appErr, ok := err.(*errors.AppError)
	if !ok {
		fmt.Fprintf(w, "%s\n", f.colorize(err.Error(), colorRed))
		if f.verbose {
			fmt.Fprintf(w, "Type: %T\n", err)
		}
		return nil
	}

	var sb strings.Builder

	if !f.noPadding {
		sb.WriteString(f.colorize("Error", colorRed))
		if string(appErr.Code()) != "" {
			sb.WriteString(" ")
			sb.WriteString(f.colorize(fmt.Sprintf("[%s]", string(appErr.Code())), colorYellow))
		}
		sb.WriteString(": ")
	} else {
		if string(appErr.Code()) != "" {
			sb.WriteString(string(appErr.Code()))
			sb.WriteString(": ")
		}
	}

	sb.WriteString(appErr.Message())
	sb.WriteString("\n")

	if f.verbose {
		f.writeVerbose(&sb, appErr)
	}

	fmt.Fprint(w, sb.String())
	return nil
}

// FormatBytes returns formatted error as bytes.
func (f *ErrorFormatter) FormatBytes(err error) ([]byte, error) {
	var buf strings.Builder
	if err := f.Format(&buf, err); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// FormatString returns formatted error as string.
func (f *ErrorFormatter) FormatString(err error) (string, error) {
	bytes, err := f.FormatBytes(err)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// FormatJSON formats error as JSON.
func (f *ErrorFormatter) FormatJSON(w io.Writer, err error) error {
	if err == nil {
		return json.NewEncoder(w).Encode(map[string]interface{}{"error": nil})
	}

	appErr, ok := err.(*errors.AppError)
	if !ok {
		return json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
	}

	return json.NewEncoder(w).Encode(map[string]interface{}{
		"code":     string(appErr.Code()),
		"message":  appErr.Message(),
		"severity": string(appErr.Severity()),
		"details":  appErr.Details(),
		"cause":    errCause(appErr),
	})
}

// writeVerbose writes verbose error information.
func (f *ErrorFormatter) writeVerbose(sb *strings.Builder, err *errors.AppError) {
	prefix := "    "

	if err.Severity() != "" {
		sb.WriteString(prefix)
		sb.WriteString("Severity: ")
		sb.WriteString(f.colorizeSeverity(err.Severity()))
		sb.WriteString("\n")
	}

	if len(err.Details()) > 0 {
		sb.WriteString(prefix)
		sb.WriteString("Details:\n")
		for key, value := range err.Details() {
			sb.WriteString(prefix)
			sb.WriteString("  ")
			sb.WriteString(key)
			sb.WriteString(": ")
			sb.WriteString(fmt.Sprintf("%v", value))
			sb.WriteString("\n")
		}
	}

	if cause := err.Unwrap(); cause != nil {
		sb.WriteString(prefix)
		sb.WriteString("Cause: ")
		sb.WriteString(cause.Error())
		sb.WriteString("\n")
	}
}

// colorizeSeverity colors the severity level.
func (f *ErrorFormatter) colorizeSeverity(severity errors.Severity) string {
	switch severity {
	case errors.SeverityCritical:
		return f.colorize(string(severity), colorRed, colorBold)
	case errors.SeverityHigh:
		return f.colorize(string(severity), colorRed)
	case errors.SeverityMedium:
		return f.colorize(string(severity), colorYellow)
	case errors.SeverityLow:
		return f.colorize(string(severity), colorBlue)
	case errors.SeverityInfo:
		return f.colorize(string(severity), colorCyan)
	default:
		return string(severity)
	}
}

// colorize applies color codes.
func (f *ErrorFormatter) colorize(text string, colors ...string) string {
	if !f.shouldColorize() {
		return text
	}
	return colorizeColors(text, colors...)
}

// shouldColorize checks if colors should be enabled.
func (f *ErrorFormatter) shouldColorize() bool {
	if f.forceNoColor {
		return false
	}
	if f.forceColor {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return isTerminal(os.Stdout)
}

// errCause returns the underlying error message.
func errCause(err *errors.AppError) string {
	if unwrapped := err.Unwrap(); unwrapped != nil {
		return unwrapped.Error()
	}
	return ""
}

// shouldColorize checks if colors should be enabled.
func shouldColorize() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return isTerminal(os.Stdout)
}

// isTerminal checks if writer is a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isTerminalFile(f)
}

// isTerminalFile checks if file is a terminal.
func isTerminalFile(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorBold   = "\033[1m"
)

// colorizeColors applies multiple color codes.
func colorizeColors(text string, colors ...string) string {
	if len(colors) == 0 {
		return text
	}
	var sb strings.Builder
	for _, color := range colors {
		sb.WriteString(color)
	}
	sb.WriteString(text)
	sb.WriteString(colorReset)
	return sb.String()
}
