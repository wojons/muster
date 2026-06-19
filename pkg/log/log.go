// Package log provides structured logging using zerolog.
// It wraps zerolog to provide the specific interface expected by the Muster codebase.
package log

import (
	"os"
	"sync"

	"github.com/rs/zerolog"
)

var (
	globalLogger Logger
	once         sync.Once
)

// Logger wraps zerolog.Logger to provide the expected interface.
type Logger struct {
	zl zerolog.Logger
}

// Entry wraps a zerolog event for fluent field construction.
type Entry struct {
	event *zerolog.Event
}

// Global returns the global logger instance, initializing it on first call.
func Global() Logger {
	once.Do(func() {
		output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}
		globalLogger = Logger{
			zl: zerolog.New(output).With().Timestamp().Logger(),
		}
	})
	return globalLogger
}

// SetGlobal replaces the global logger instance.
func SetGlobal(l Logger) {
	globalLogger = l
}

// Info starts an info-level log entry.
func (l Logger) Info() *Entry {
	return &Entry{event: l.zl.Info()}
}

// Warn starts a warning-level log entry.
func (l Logger) Warn() *Entry {
	return &Entry{event: l.zl.Warn()}
}

// Error starts an error-level log entry.
func (l Logger) Error() *Entry {
	return &Entry{event: l.zl.Error()}
}

// Debug starts a debug-level log entry.
func (l Logger) Debug() *Entry {
	return &Entry{event: l.zl.Debug()}
}

// Str adds a string field to the log entry.
func (e *Entry) Str(key, val string) *Entry {
	if e != nil && e.event != nil {
		e.event.Str(key, val)
	}
	return e
}

// Int adds an integer field to the log entry.
func (e *Entry) Int(key string, val int) *Entry {
	if e != nil && e.event != nil {
		e.event.Int(key, val)
	}
	return e
}

// Bool adds a boolean field to the log entry.
func (e *Entry) Bool(key string, val bool) *Entry {
	if e != nil && e.event != nil {
		e.event.Bool(key, val)
	}
	return e
}

// Err adds an error field to the log entry.
func (e *Entry) Err(err error) *Entry {
	if e != nil && e.event != nil && err != nil {
		e.event.Err(err)
	}
	return e
}

// Msg finalizes and writes the log entry with the given message.
func (e *Entry) Msg(msg string) {
	if e != nil && e.event != nil {
		e.event.Msg(msg)
	}
}

// WithField adds an arbitrary field to the log entry.
func (e *Entry) WithField(key string, val interface{}) *Entry {
	if e != nil && e.event != nil {
		e.event.Interface(key, val)
	}
	return e
}

// Interface adds an arbitrary interface field to the log entry.
func (e *Entry) Interface(key string, val interface{}) *Entry {
	if e != nil && e.event != nil {
		e.event.Interface(key, val)
	}
	return e
}

// NewLogger creates a new Logger from a zerolog.Logger.
func NewLogger(zl zerolog.Logger) Logger {
	return Logger{zl: zl}
}

// FromContext extracts a logger from a context.
// If no logger is found, returns the global logger.
func FromContext(ctx interface{}) Logger {
	// For now, always return global logger
	// Full implementation would extract from context.Context
	return Global()
}
