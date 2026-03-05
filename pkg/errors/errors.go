package errors

import (
	"fmt"
)

type Code string

const (
	CodeGeneric            Code = "GENERIC"
	CodeBadRequest         Code = "BAD_REQUEST"
	CodeUnauthorized       Code = "UNAUTHORIZED"
	CodeForbidden          Code = "FORBIDDEN"
	CodeNotFound           Code = "NOT_FOUND"
	CodeConflict           Code = "CONFLICT"
	CodeInternal           Code = "INTERNAL_ERROR"
	CodeServiceUnavailable Code = "SERVICE_UNAVAILABLE"
	CodeTimeout            Code = "TIMEOUT"
	CodeRateLimitExceeded  Code = "RATE_LIMIT_EXCEEDED"
	CodeInvalidInput       Code = "INVALID_INPUT"
	CodeValidation         Code = "VALIDATION_ERROR"
	CodeConfig             Code = "CONFIGURATION_ERROR"
	CodeNetwork            Code = "NETWORK_ERROR"
	CodeDatabase           Code = "DATABASE_ERROR"
	CodeResource           Code = "RESOURCE_ERROR"
	CodeAuthentication     Code = "AUTHENTICATION_ERROR"
	CodeAuthorization      Code = "AUTHORIZATION_ERROR"
	CodeTokenExpired       Code = "TOKEN_EXPIRED"
	CodeTokenInvalid       Code = "TOKEN_INVALID"
	CodeKeychainError      Code = "KEYCHAIN_ERROR"
	CodeOAuth              Code = "OAUTH_ERROR"
	CodeMTLS               Code = "MTLS_ERROR"
	CodeInventory          Code = "INVENTORY_ERROR"
	CodeOpenAPI            Code = "OPENAPI_ERROR"
	CodeParsing            Code = "PARSING_ERROR"
	CodeWASM               Code = "WASM_ERROR"
	CodeExecution          Code = "EXECUTION_ERROR"
	CodeCapability         Code = "CAPABILITY_ERROR"
	CodeCache              Code = "CACHE_ERROR"
	CodeLock               Code = "LOCK_ERROR"
	CodeConcurrent         Code = "CONCURRENT_ERROR"
	CodeSerialization      Code = "SERIALIZATION_ERROR"
	CodeDeserialization    Code = "DESERIALIZATION_ERROR"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type AppError struct {
	code     Code
	message  string
	cause    error
	details  map[string]interface{}
	severity Severity
}

func (e *AppError) Error() string {
	msg := e.message
	if cause := e.cause; cause != nil {
		msg = fmt.Sprintf("%s: %v", msg, cause)
	}
	return msg
}

func (e *AppError) Unwrap() error {
	return e.cause
}

func (e *AppError) Code() Code {
	return e.code
}

func (e *AppError) Message() string {
	return e.message
}

func (e *AppError) Details() map[string]interface{} {
	return e.details
}

func (e *AppError) Severity() Severity {
	return e.severity
}

func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.code == t.code
}

func New(code Code, message string) *AppError {
	return &AppError{
		code:     code,
		message:  message,
		details:  make(map[string]interface{}),
		severity: SeverityMedium,
	}
}

func Wrap(err error, code Code, message string) *AppError {
	return &AppError{
		code:     code,
		message:  message,
		cause:    err,
		details:  make(map[string]interface{}),
		severity: SeverityMedium,
	}
}

func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	e.details[key] = value
	return e
}

func (e *AppError) WithDetails(details map[string]interface{}) *AppError {
	for k, v := range details {
		e.details[k] = v
	}
	return e
}

func (e *AppError) WithCause(err error) *AppError {
	e.cause = err
	return e
}

func (e *AppError) WithSeverity(severity Severity) *AppError {
	e.severity = severity
	return e
}

func IsError(err error, code Code) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*AppError)
	if !ok {
		return false
	}
	return e.code == code
}

func GetCode(err error) Code {
	if err == nil {
		return CodeGeneric
	}
	e, ok := err.(*AppError)
	if !ok {
		return CodeGeneric
	}
	return e.code
}

func Format(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

var (
	ErrGeneric            = New(CodeGeneric, "An error occurred")
	ErrBadRequest         = New(CodeBadRequest, "Bad request")
	ErrUnauthorized       = New(CodeUnauthorized, "Unauthorized")
	ErrForbidden          = New(CodeForbidden, "Forbidden")
	ErrNotFound           = New(CodeNotFound, "Resource not found")
	ErrConflict           = New(CodeConflict, "Conflict")
	ErrInternal           = New(CodeInternal, "Internal error")
	ErrServiceUnavailable = New(CodeServiceUnavailable, "Service unavailable")
	ErrTimeout            = New(CodeTimeout, "Request timeout")
	ErrRateLimitExceeded  = New(CodeRateLimitExceeded, "Rate limit exceeded")
	ErrInvalidInput       = New(CodeInvalidInput, "Invalid input")
	ErrValidation         = New(CodeValidation, "Validation error")
	ErrConfig             = New(CodeConfig, "Configuration error")
	ErrNetwork            = New(CodeNetwork, "Network error")
	ErrDatabase           = New(CodeDatabase, "Database error")
	ErrResource           = New(CodeResource, "Resource error")
	ErrAuthentication     = New(CodeAuthentication, "Authentication error")
	ErrAuthorization      = New(CodeAuthorization, "Authorization error")
	ErrTokenExpired       = New(CodeTokenExpired, "Token expired")
	ErrTokenInvalid       = New(CodeTokenInvalid, "Token invalid")
	ErrKeychain           = New(CodeKeychainError, "Keychain error")
	ErrOAuth              = New(CodeOAuth, "OAuth error")
	ErrMTLS               = New(CodeMTLS, "mTLS error")
	ErrInventory          = New(CodeInventory, "Inventory error")
	ErrOpenAPI            = New(CodeOpenAPI, "OpenAPI error")
	ErrParsing            = New(CodeParsing, "Parsing error")
	ErrWASM               = New(CodeWASM, "WASM error")
	ErrExecution          = New(CodeExecution, "Execution error")
	ErrCapability         = New(CodeCapability, "Capability error")
	ErrCache              = New(CodeCache, "Cache error")
	ErrLock               = New(CodeLock, "Lock error")
	ErrConcurrent         = New(CodeConcurrent, "Concurrent error")
	ErrSerialization      = New(CodeSerialization, "Serialization error")
	ErrDeserialization    = New(CodeDeserialization, "Deserialization error")
)

type Error interface {
	Error() string
	Code() Code
	Message() string
	Severity() Severity
	Details() map[string]interface{}
	Is(target error) bool
}
