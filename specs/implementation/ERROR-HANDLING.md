# Error Handling Specification

## Overview

Comprehensive error handling strategy including error types, codes, retry logic, circuit breaker, and recovery mechanisms.

## 1. Error Hierarchy

```go
// internal/core/errors/errors.go

package errors

import (
    "errors"
    "fmt"
)

// Base error interface
type OpenAPICliError interface {
    error
    Code() string
    Severity() Severity
    IsRetryable() bool
    Details() map[string]interface{}
}

// Severity levels
type Severity string

const (
    SeverityDebug   Severity = "debug"
    SeverityInfo    Severity = "info"
    SeverityWarning Severity = "warning"
    SeverityError   Severity = "error"
    SeverityFatal   Severity = "fatal"
)

// Base error struct
type BaseError struct {
    Code_     string
    Message   string
    Severity_ Severity
    Retryable bool
    Cause     error
    Details_  map[string]interface{}
}

func (e *BaseError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (caused by: %v)", e.Code_, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s", e.Code_, e.Message)
}

func (e *BaseError) Code() string { return e.Code_ }
func (e *BaseError) Severity() Severity { return e.Severity_ }
func (e *BaseError) IsRetryable() bool { return e.Retryable }
func (e *BaseError) Details() map[string]interface{} { return e.Details_ }

func (e *BaseError) Unwrap() error { return e.Cause }

// --- Specific Error Types ---

// InventoryError for inventory-related failures
type InventoryError struct {
    *BaseError
    Host    string
    Version string
}

func NewInventoryError(code, message string, host string, cause error) *InventoryError {
    return &InventoryError{
        BaseError: &BaseError{
            Code_:     code,
            Message:   message,
            Severity_: SeverityError,
            Retryable: true, // Network errors are retryable
            Cause:     cause,
            Details_:  map[string]interface{}{"host": host},
        },
        Host: host,
    }
}

// HTTPError for HTTP request failures
type HTTPError struct {
    *BaseError
    StatusCode int
    URL        string
    Method     string
}

func NewHTTPError(statusCode int, url, method string, cause error) *HTTPError {
    retryable := statusCode >= 500 || statusCode == 429 || statusCode == 0 // Network errors
    
    return &HTTPError{
        BaseError: &BaseError{
            Code_:     "HTTP_ERROR",
            Message:   fmt.Sprintf("HTTP %d %s %s failed", statusCode, method, url),
            Severity_: SeverityError,
            Retryable: retryable,
            Cause:     cause,
            Details_: map[string]interface{}{
                "status_code": statusCode,
                "url":         url,
                "method":      method,
            },
        },
        StatusCode: statusCode,
        URL:        url,
        Method:     method,
    }
}

// AuthError for authentication failures
type AuthError struct {
    *BaseError
    AuthType string
}

func NewAuthError(authType, message string, cause error) *AuthError {
    return &AuthError{
        BaseError: &BaseError{
            Code_:     "AUTH_FAILED",
            Message:   message,
            Severity_: SeverityError,
            Retryable: false, // Auth errors usually not retryable
            Cause:     cause,
            Details_:  map[string]interface{}{"auth_type": authType},
        },
        AuthType: authType,
    }
}

// CacheError for cache failures
type CacheError struct {
    *BaseError
    Operation string
    Key       string
}

func NewCacheError(operation, key string, cause error) *CacheError {
    return &CacheError{
        BaseError: &BaseError{
            Code_:     "CACHE_ERROR",
            Message:   fmt.Sprintf("Cache %s failed for key %s", operation, key),
            Severity_: SeverityWarning, // Cache errors are warnings (can fallback)
            Retryable: true,
            Cause:     cause,
            Details_: map[string]interface{}{
                "operation": operation,
                "key":       key,
            },
        },
        Operation: operation,
        Key:       key,
    }
}

// WASMError for WASM execution failures
type WASMError struct {
    *BaseError
    Module string
    Function string
}

func NewWASMError(module, function, message string, cause error) *WASMError {
    return &WASMError{
        BaseError: &BaseError{
            Code_:     "WASM_ERROR",
            Message:   fmt.Sprintf("WASM execution failed: %s", message),
            Severity_: SeverityError,
            Retryable: false, // WASM errors usually deterministic
            Cause:     cause,
            Details_: map[string]interface{}{
                "module":   module,
                "function": function,
            },
        },
        Module:   module,
        Function: function,
    }
}

// DSLError for DSL execution failures
type DSLError struct {
    *BaseError
    Line   int
    Column int
}

func NewDSLError(message string, line, column int, cause error) *DSLError {
    return &DSLError{
        BaseError: &BaseError{
            Code_:     "DSL_ERROR",
            Message:   fmt.Sprintf("DSL error at line %d, column %d: %s", line, column, message),
            Severity_: SeverityError,
            Retryable: false,
            Cause:     cause,
            Details_: map[string]interface{}{
                "line":   line,
                "column": column,
            },
        },
        Line:   line,
        Column: column,
    }
}

// ConfigError for configuration errors
type ConfigError struct {
    *BaseError
    Key string
}

func NewConfigError(key, message string, cause error) *ConfigError {
    return &ConfigError{
        BaseError: &BaseError{
            Code_:     "CONFIG_ERROR",
            Message:   fmt.Sprintf("Configuration error for %s: %s", key, message),
            Severity_: SeverityFatal,
            Retryable: false,
            Cause:     cause,
            Details_:  map[string]interface{}{"key": key},
        },
        Key: key,
    }
}

// SignatureError for signature verification failures
type SignatureError struct {
    *BaseError
    KeyID string
}

func NewSignatureError(keyID, message string, cause error) *SignatureError {
    return &SignatureError{
        BaseError: &BaseError{
            Code_:     "SIGNATURE_INVALID",
            Message:   message,
            Severity_: SeverityFatal,
            Retryable: false, // Signature errors not retryable
            Cause:     cause,
            Details_:  map[string]interface{}{"key_id": keyID},
        },
        KeyID: keyID,
    }
}

// RevocationError for revoked inventories/modules
type RevocationError struct {
    *BaseError
    RevokedID   string
    RevokedType string
    Reason      string
    Severity_   Severity
}

func NewRevocationError(revokedType, id, reason string, severity Severity) *RevocationError {
    return &RevocationError{
        BaseError: &BaseError{
            Code_:     "REVOKED",
            Message:   fmt.Sprintf("%s %s has been revoked: %s", revokedType, id, reason),
            Severity_: severity,
            Retryable: false,
            Cause:     nil,
            Details_: map[string]interface{}{
                "type":   revokedType,
                "id":     id,
                "reason": reason,
            },
        },
        RevokedID:   id,
        RevokedType: revokedType,
        Reason:      reason,
        Severity_:   severity,
    }
}
```

## 2. Error Codes

```go
// Error code constants
const (
    // Inventory errors
    ErrInventoryFetchFailed     = "INV_FETCH_FAILED"
    ErrInventoryNotFound        = "INV_NOT_FOUND"
    ErrInventoryVersionMismatch = "INV_VERSION_MISMATCH"
    ErrInventoryExpired         = "INV_EXPIRED"
    ErrInventoryRevoked         = "INV_REVOKED"
    
    // HTTP errors
    ErrHTTPTimeout              = "HTTP_TIMEOUT"
    ErrHTTPNetwork              = "HTTP_NETWORK"
    ErrHTTP5xx                  = "HTTP_5XX"
    ErrHTTP429                  = "HTTP_RATE_LIMITED"
    ErrHTTP401                  = "HTTP_UNAUTHORIZED"
    ErrHTTP403                  = "HTTP_FORBIDDEN"
    ErrHTTP404                  = "HTTP_NOT_FOUND"
    
    // Auth errors
    ErrAuthTokenExpired         = "AUTH_TOKEN_EXPIRED"
    ErrAuthTokenInvalid         = "AUTH_TOKEN_INVALID"
    ErrAuthRefreshFailed        = "AUTH_REFRESH_FAILED"
    ErrAuthMTLSFailed           = "AUTH_MTLS_FAILED"
    ErrAuthOAuthFailed          = "AUTH_OAUTH_FAILED"
    
    // Cache errors
    ErrCacheCorrupted           = "CACHE_CORRUPTED"
    ErrCacheFull                = "CACHE_FULL"
    ErrCacheMiss                = "CACHE_MISS"
    
    // WASM errors
    ErrWASMLoadFailed           = "WASM_LOAD_FAILED"
    ErrWASMExecutionFailed      = "WASM_EXEC_FAILED"
    ErrWASMTimeout              = "WASM_TIMEOUT"
    ErrWASMMemoryExceeded       = "WASM_MEMORY_EXCEEDED"
    ErrWASMInvalidSignature       = "WASM_INVALID_SIGNATURE"
    
    // DSL errors
    ErrDSLParseFailed           = "DSL_PARSE_FAILED"
    ErrDSLExecutionFailed       = "DSL_EXEC_FAILED"
    ErrDSLTimeout               = "DSL_TIMEOUT"
    ErrDSLSecurityViolation     = "DSL_SECURITY_VIOLATION"
    
    // Config errors
    ErrConfigNotFound           = "CONFIG_NOT_FOUND"
    ErrConfigInvalid            = "CONFIG_INVALID"
    ErrConfigVersionMismatch    = "CONFIG_VERSION_MISMATCH"
    
    // Module errors
    ErrModuleNotFound           = "MODULE_NOT_FOUND"
    ErrModuleDownloadFailed     = "MODULE_DOWNLOAD_FAILED"
    ErrModuleHashMismatch       = "MODULE_HASH_MISMATCH"
    
    // Signature errors
    ErrSignatureInvalid         = "SIGNATURE_INVALID"
    ErrSignatureVerification    = "SIG_VERIFY_FAILED"
    ErrKeyNotFound              = "KEY_NOT_FOUND"
    
    // Fingerprint errors
    ErrFingerprintRegeneration   = "FP_REGEN_FAILED"
    ErrFingerprintRevoked       = "FP_REVOKED"
    
    // General errors
    ErrContextCanceled          = "CONTEXT_CANCELED"
    ErrContextDeadlineExceeded  = "CONTEXT_DEADLINE_EXCEEDED"
    ErrNotImplemented           = "NOT_IMPLEMENTED"
    ErrInternal                 = "INTERNAL_ERROR"
)
```

## 3. Retry Logic

```go
// internal/core/retry/retry.go

package retry

import (
    "context"
    "math"
    "math/rand"
    "time"
)

// Config for retry behavior
type Config struct {
    MaxAttempts     int
    InitialDelay    time.Duration
    MaxDelay        time.Duration
    BackoffStrategy string // "linear", "exponential", "exponential_jitter"
    RetryableErrors []string
}

// Default config
var DefaultConfig = Config{
    MaxAttempts:     3,
    InitialDelay:    1 * time.Second,
    MaxDelay:        30 * time.Second,
    BackoffStrategy: "exponential_jitter",
    RetryableErrors: []string{
        ErrHTTP5xx,
        ErrHTTP429,
        ErrHTTPNetwork,
        ErrHTTPTimeout,
        ErrCacheCorrupted,
    },
}

// Retry executes fn with retry logic
func Retry(ctx context.Context, config Config, fn func() error) error {
    var lastErr error
    
    for attempt := 0; attempt < config.MaxAttempts; attempt++ {
        if attempt > 0 {
            delay := calculateDelay(config, attempt)
            
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
        
        err := fn()
        if err == nil {
            return nil
        }
        
        lastErr = err
        
        // Check if error is retryable
        if !isRetryable(err, config.RetryableErrors) {
            return err // Don't retry non-retryable errors
        }
        
        // Check if we should retry based on error specifics
        if cliErr, ok := err.(OpenAPICliError); ok {
            if !cliErr.IsRetryable() {
                return err
            }
        }
    }
    
    return fmt.Errorf("max retries exceeded (attempts: %d): %w", config.MaxAttempts, lastErr)
}

func calculateDelay(config Config, attempt int) time.Duration {
    switch config.BackoffStrategy {
    case "linear":
        return config.InitialDelay * time.Duration(attempt)
        
    case "exponential":
        delay := config.InitialDelay * time.Duration(math.Pow(2, float64(attempt-1)))
        if delay > config.MaxDelay {
            delay = config.MaxDelay
        }
        return delay
        
    case "exponential_jitter":
        // Full jitter: random between 0 and exponential delay
        expDelay := config.InitialDelay * time.Duration(math.Pow(2, float64(attempt-1)))
        if expDelay > config.MaxDelay {
            expDelay = config.MaxDelay
        }
        jitter := time.Duration(rand.Float64() * float64(expDelay))
        return jitter
        
    default:
        return config.InitialDelay
    }
}

func isRetryable(err error, codes []string) bool {
    if cliErr, ok := err.(OpenAPICliError); ok {
        code := cliErr.Code()
        for _, c := range codes {
            if c == code {
                return true
            }
        }
    }
    return false
}

// RetryHTTP wraps HTTP client with retry
func RetryHTTP(client HTTPClient, config Config) HTTPClient {
    return &retryHTTPClient{
        client: client,
        config: config,
    }
}

type retryHTTPClient struct {
    client HTTPClient
    config Config
}

func (c *retryHTTPClient) Do(ctx context.Context, req *Request) (*Response, error) {
    var resp *Response
    var err error
    
    retryErr := Retry(ctx, c.config, func() error {
        resp, err = c.client.Do(ctx, req)
        if err != nil {
            return err
        }
        
        // Check if HTTP status warrants retry
        if resp.StatusCode >= 500 || resp.StatusCode == 429 {
            return NewHTTPError(resp.StatusCode, req.URL, req.Method, nil)
        }
        
        return nil
    })
    
    if retryErr != nil {
        return nil, retryErr
    }
    
    return resp, nil
}
```

## 4. Circuit Breaker

```go
// internal/core/circuitbreaker/breaker.go

package circuitbreaker

import (
    "sync"
    "time"
)

// State of circuit breaker
type State int

const (
    StateClosed    State = iota // Normal operation
    StateOpen                   // Failing, rejecting requests
    StateHalfOpen               // Testing if service recovered
)

// Config for circuit breaker
type Config struct {
    FailureThreshold    int           // Trip after N failures
    SuccessThreshold    int           // Close after N successes in half-open
    Timeout             time.Duration // Reset timeout
    HalfOpenMaxRequests int           // Max requests in half-open
}

// Default config
var DefaultConfig = Config{
    FailureThreshold:    5,
    SuccessThreshold:  3,
    Timeout:             30 * time.Second,
    HalfOpenMaxRequests: 3,
}

// Breaker implements circuit breaker pattern
type Breaker struct {
    config Config
    
    mu                sync.RWMutex
    state             State
    failures          int
    successes         int
    lastFailureTime   time.Time
    halfOpenRequests  int
}

// New creates a new circuit breaker
func New(config Config) *Breaker {
    return &Breaker{
        config: config,
        state:  StateClosed,
    }
}

// Execute runs fn with circuit breaker protection
func (b *Breaker) Execute(fn func() error) error {
    // Check if we can proceed
    if err := b.canExecute(); err != nil {
        return err
    }
    
    err := fn()
    b.recordResult(err)
    
    return err
}

func (b *Breaker) canExecute() error {
    b.mu.RLock()
    state := b.state
    b.mu.RUnlock()
    
    switch state {
    case StateClosed:
        return nil
        
    case StateOpen:
        // Check if timeout elapsed
        if time.Since(b.lastFailureTime) > b.config.Timeout {
            // Transition to half-open
            b.mu.Lock()
            b.state = StateHalfOpen
            b.halfOpenRequests = 0
            b.mu.Unlock()
            return nil
        }
        return ErrCircuitOpen
        
    case StateHalfOpen:
        // Allow limited requests
        b.mu.Lock()
        defer b.mu.Unlock()
        if b.halfOpenRequests < b.config.HalfOpenMaxRequests {
            b.halfOpenRequests++
            return nil
        }
        return ErrCircuitOpen
    }
    
    return nil
}

func (b *Breaker) recordResult(err error) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    if err != nil {
        b.failures++
        b.lastFailureTime = time.Now()
        
        if b.state == StateHalfOpen {
            // Back to open
            b.state = StateOpen
            b.halfOpenRequests = 0
        } else if b.failures >= b.config.FailureThreshold {
            // Trip the circuit
            b.state = StateOpen
        }
    } else {
        if b.state == StateHalfOpen {
            b.successes++
            if b.successes >= b.config.SuccessThreshold {
                // Close the circuit
                b.state = StateClosed
                b.failures = 0
                b.successes = 0
                b.halfOpenRequests = 0
            }
        } else {
            // Reset failures on success
            b.failures = 0
        }
    }
}

// State returns current state
func (b *Breaker) State() State {
    b.mu.RLock()
    defer b.mu.RUnlock()
    return b.state
}

// Errors
var ErrCircuitOpen = errors.New("circuit breaker is open")
```

## 5. Error Recovery

```go
// internal/core/recovery/recovery.go

package recovery

import (
    "fmt"
    "runtime/debug"
)

// Recover catches panics and converts to errors
func Recover(fn func() error) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("panic recovered: %v\n%s", r, debug.Stack())
        }
    }()
    
    return fn()
}

// RecoveryStrategy defines how to recover from specific errors
type RecoveryStrategy interface {
    CanRecover(err error) bool
    Recover(ctx context.Context, err error) (Result, error)
}

// Result of recovery attempt
type Result struct {
    Recovered    bool
    FallbackUsed bool
    Message      string
}

// FallbackStrategy uses fallback values on error
type FallbackStrategy struct {
    FallbackFn func() (interface{}, error)
}

func (f *FallbackStrategy) CanRecover(err error) bool {
    // Can recover from any error
    return true
}

func (f *FallbackStrategy) Recover(ctx context.Context, err error) (Result, error) {
    fallback, err := f.FallbackFn()
    if err != nil {
        return Result{Recovered: false}, err
    }
    
    return Result{
        Recovered:    true,
        FallbackUsed: true,
        Message:      fmt.Sprintf("using fallback: %v", fallback),
    }, nil
}

// CacheFallbackStrategy uses cache on network errors
type CacheFallbackStrategy struct {
    Cache CacheProvider
}

func (c *CacheFallbackStrategy) CanRecover(err error) bool {
    // Only recover from network errors
    if httpErr, ok := err.(*HTTPError); ok {
        return httpErr.StatusCode == 0 || httpErr.StatusCode >= 500
    }
    return false
}

func (c *CacheFallbackStrategy) Recover(ctx context.Context, err error) (Result, error) {
    // Try to use stale cache
    if c.Cache != nil {
        // Implementation depends on cache interface
        return Result{
            Recovered:    true,
            FallbackUsed: true,
            Message:      "using stale cache",
        }, nil
    }
    
    return Result{Recovered: false}, err
}
```

## 6. User-Facing Error Messages

```go
// internal/core/errors/messages.go

package errors

import (
    "fmt"
    "strings"
)

// FormatError returns user-friendly error message
func FormatError(err error, verbose bool) string {
    if err == nil {
        return ""
    }
    
    var messages []string
    
    // Main error
    if cliErr, ok := err.(OpenAPICliError); ok {
        messages = append(messages, formatCliError(cliErr, verbose))
        
        // Suggestions based on error code
        if suggestion := getSuggestion(cliErr.Code()); suggestion != "" {
            messages = append(messages, "")
            messages = append(messages, "Suggestion: "+suggestion)
        }
    } else {
        messages = append(messages, err.Error())
    }
    
    // Cause chain
    if verbose {
        cause := errors.Unwrap(err)
        if cause != nil {
            messages = append(messages, "")
            messages = append(messages, "Caused by:")
            messages = append(messages, "  "+cause.Error())
        }
    }
    
    return strings.Join(messages, "\n")
}

func formatCliError(err OpenAPICliError, verbose bool) string {
    code := err.Code()
    message := err.Error()
    details := err.Details()
    
    switch code {
    case ErrInventoryFetchFailed:
        return fmt.Sprintf("Failed to fetch inventory from %s\n\n%s", 
            details["host"], message)
            
    case ErrHTTPTimeout:
        return fmt.Sprintf("Request timed out\n\n%s", message)
        
    case ErrHTTP429:
        return fmt.Sprintf("Rate limited. Please wait before retrying.\n\n%s", message)
        
    case ErrAuthTokenExpired:
        return fmt.Sprintf("Authentication expired. Please run:\n\n  openapi-cli auth login", message)
        
    case ErrSignatureInvalid:
        return fmt.Sprintf("Security warning: Inventory signature verification failed.\n\n%s\n\nThis could indicate a security issue or misconfiguration.", message)
        
    case ErrInventoryRevoked:
        return fmt.Sprintf("CRITICAL: This inventory has been revoked.\nReason: %s\n\nContact your administrator immediately.", details["reason"])
        
    default:
        if verbose {
            return fmt.Sprintf("Error [%s]: %s", code, message)
        }
        return message
    }
}

func getSuggestion(code string) string {
    suggestions := map[string]string{
        ErrInventoryFetchFailed:   "Check your network connection and try again. Use --offline to use cached version.",
        ErrHTTPTimeout:           "The server is taking too long to respond. Try again later or increase timeout with --timeout.",
        ErrAuthTokenExpired:      "Run 'openapi-cli auth login' to refresh your credentials.",
        ErrCacheCorrupted:        "Run 'openapi-cli cache clear' to clear the corrupted cache.",
        ErrConfigInvalid:         "Check your configuration file for errors using 'openapi-cli config validate'.",
        ErrWASMExecutionFailed:   "The module execution failed. Check the module capabilities and retry.",
    }
    
    if suggestion, ok := suggestions[code]; ok {
        return suggestion
    }
    return ""
}
```

## 7. Error Logging

```go
// LogError logs an error with appropriate severity
func LogError(logger Logger, err error) {
    if err == nil {
        return
    }
    
    if cliErr, ok := err.(OpenAPICliError); ok {
        switch cliErr.Severity() {
        case SeverityDebug:
            logger.Debug(err.Error(), "code", cliErr.Code())
        case SeverityInfo:
            logger.Info(err.Error(), "code", cliErr.Code())
        case SeverityWarning:
            logger.Warn(err.Error(), "code", cliErr.Code())
        case SeverityError:
            logger.Error(err.Error(), 
                "code", cliErr.Code(),
                "retryable", cliErr.IsRetryable(),
                "details", cliErr.Details())
        case SeverityFatal:
            logger.Fatal(err.Error(), "code", cliErr.Code())
        }
    } else {
        logger.Error(err.Error())
    }
}
```

---

This error handling specification provides complete error types, retry logic, circuit breaker, recovery strategies, and user-facing error messages for Ralph Loop.
