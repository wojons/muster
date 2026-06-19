// Package errors provides structured error types for the Muster application.
// It implements the AppError pattern with error codes, severity levels,
// contextual details, and error wrapping support.
package errors

import (
	"fmt"
	"strings"
)

// Code represents an error code categorization.
type Code string

// Common error codes.
const (
	CodeBadRequest   Code = "BAD_REQUEST"
	CodeNotFound     Code = "NOT_FOUND"
	CodeValidation   Code = "VALIDATION_ERROR"
	CodeGeneric      Code = "GENERIC_ERROR"
	CodeDatabase     Code = "DATABASE_ERROR"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeInternal     Code = "INTERNAL_ERROR"
	CodeTimeout      Code = "TIMEOUT"
	CodeRateLimit    Code = "RATE_LIMITED"
)

// Severity indicates the seriousness of an error.
type Severity string

// Severity levels.
const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// AppError is a structured application error with code, message, severity,
// contextual details, and optional wrapped cause.
type AppError struct {
	code     Code
	message  string
	severity Severity
	details  map[string]interface{}
	cause    error
}

// New creates a new AppError with the given code and message.
func New(code Code, message string) *AppError {
	return &AppError{
		code:    code,
		message: message,
		details: make(map[string]interface{}),
	}
}

// Wrap wraps an existing error with a new code and message.
// The original error becomes the cause.
func Wrap(err error, code Code, message string) *AppError {
	appErr := New(code, message)
	appErr.cause = err
	return appErr
}

// Code returns the error code.
func (e *AppError) Code() Code {
	if e == nil {
		return ""
	}
	return e.code
}

// Message returns the error message.
func (e *AppError) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Severity returns the error severity level.
func (e *AppError) Severity() Severity {
	if e == nil {
		return ""
	}
	return e.severity
}

// Details returns the error's contextual details map.
func (e *AppError) Details() map[string]interface{} {
	if e == nil {
		return nil
	}
	return e.details
}

// Unwrap returns the wrapped cause error, implementing the standard
// errors.Unwrap interface for use with errors.Is and errors.As.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(string(e.code))
	sb.WriteString(": ")
	sb.WriteString(e.message)
	if e.cause != nil {
		sb.WriteString(": ")
		sb.WriteString(e.cause.Error())
	}
	return sb.String()
}

// WithDetail adds a contextual detail to the error.
// Returns the error for method chaining.
func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e == nil {
		return nil
	}
	if e.details == nil {
		e.details = make(map[string]interface{})
	}
	e.details[key] = value
	return e
}

// WithSeverity sets the severity level of the error.
// Returns the error for method chaining.
func (e *AppError) WithSeverity(severity Severity) *AppError {
	if e == nil {
		return nil
	}
	e.severity = severity
	return e
}

// IsError checks whether the given error (or any error in its chain)
// has the specified code. Returns false if err is nil.
func IsError(err error, code Code) bool {
	if err == nil {
		return false
	}
	if appErr, ok := err.(*AppError); ok {
		if appErr.Code() == code {
			return true
		}
	}
	// Walk the error chain
	for {
		unwrapped := err
		if u, ok := err.(interface{ Unwrap() error }); ok {
			unwrapped = u.Unwrap()
			if unwrapped == nil {
				break
			}
			if appErr, ok := unwrapped.(*AppError); ok {
				if appErr.Code() == code {
					return true
				}
			}
			err = unwrapped
		} else {
			break
		}
	}
	return false
}

// Errorf creates a new AppError with a formatted message.
func Errorf(code Code, format string, args ...interface{}) *AppError {
	return New(code, fmt.Sprintf(format, args...))
}
