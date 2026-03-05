# Middleware: Panic Recovery

## Overview

Panic recovery middleware catches unhandled panics, logs stack traces, and returns graceful HTTP 500 responses without exposing internal details to clients.

## Configuration

```go
type RecoveryConfig struct {
    Enabled              bool     `json:"enabled"`
    LogStackTrace        bool     `json:"log_stack_trace"`
    StackTraceSize       int      `json:"stack_trace_size" validate:"gte=0"`
    EnablePanicLog       bool     `json:"enable_panic_log"`
    CustomErrorMessage   string   `json:"custom_error_message"`
    NotifyHandler        func(error, []byte) `json:"-"`
    ExcludeErrors        []string `json:"exclude_errors"`
}

var DefaultRecoveryConfig = RecoveryConfig{
    Enabled:            true,
    LogStackTrace:      true,
    StackTraceSize:     4096,
    EnablePanicLog:     true,
    CustomErrorMessage: "Internal Server Error",
}
```

## Recovery Middleware Implementation

```go
package middleware

import (
    "fmt"
    "net/http"
    "runtime/debug"
    "strings"

    "go.uber.org/zap"
)

type RecoveryMiddleware struct {
    config RecoveryConfig
    logger *zap.Logger
}

type PanicError struct {
    Message    string
    Stack      []byte
    RequestID  string
    Method     string
    Path       string
    RemoteAddr string
    UserAgent  string
}

func (e *PanicError) Error() string {
    return fmt.Sprintf("panic: %s", e.Message)
}

func NewRecoveryMiddleware(config RecoveryConfig, logger *zap.Logger) *RecoveryMiddleware {
    return &RecoveryMiddleware{
        config: config,
        logger: logger,
    }
}

func (m *RecoveryMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !m.config.Enabled {
            next.ServeHTTP(w, r)
            return
        }

        defer func() {
            if rec := recover(); rec != nil {
                m.handlePanic(w, r, rec)
            }
        }()

        next.ServeHTTP(w, r)
    })
}

func (m *RecoveryMiddleware) handlePanic(w http.ResponseWriter, r *http.Request, rec interface{}) {
    var err error
    switch v := rec.(type) {
    case error:
        err = v
    case string:
        err = fmt.Errorf("%s", v)
    default:
        err = fmt.Errorf("%v", v)
    }

    for _, exclude := range m.config.ExcludeErrors {
        if strings.Contains(err.Error(), exclude) {
            return
        }
    }

    stack := debug.Stack()
    if m.config.StackTraceSize > 0 && len(stack) > m.config.StackTraceSize {
        stack = stack[:m.config.StackTraceSize]
    }

    panicErr := &PanicError{
        Message:    err.Error(),
        Stack:      stack,
        RequestID:  r.Header.Get("X-Request-ID"),
        Method:     r.Method,
        Path:       r.URL.Path,
        RemoteAddr: r.RemoteAddr,
        UserAgent:  r.UserAgent(),
    }

    if m.config.EnablePanicLog && m.logger != nil {
        fields := []zap.Field{
            zap.String("error", panicErr.Message),
            zap.String("request_id", panicErr.RequestID),
            zap.String("method", panicErr.Method),
            zap.String("path", panicErr.Path),
            zap.String("remote_addr", panicErr.RemoteAddr),
        }
        if m.config.LogStackTrace {
            fields = append(fields, zap.String("stack_trace", string(panicErr.Stack)))
        }
        m.logger.Error("panic recovered", fields...)
    }

    if m.config.NotifyHandler != nil {
        go m.config.NotifyHandler(err, stack)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusInternalServerError)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error":   m.config.CustomErrorMessage,
        "request_id": panicErr.RequestID,
    })
}

func SafeHandler(fn http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                err := fmt.Errorf("panic in handler: %v", rec)
                stack := debug.Stack()
                fmt.Fprintf(os.Stderr, "Panic: %v\n%s\n", err, stack)
                
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusInternalServerError)
                json.NewEncoder(w).Encode(map[string]string{
                    "error": "Internal Server Error",
                })
            }
        }()
        fn(w, r)
    }
}
```
