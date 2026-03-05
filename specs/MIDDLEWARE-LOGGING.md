# Middleware: Request Logging

## Overview

Request logging middleware captures HTTP request/response details with configurable log levels, structured formatting, and sensitive data redaction for security compliance.

## Configuration

```go
type LoggingConfig struct {
    Enabled           bool     `json:"enabled"`
    Level             string   `json:"level" validate:"oneof=debug info warn error"`
    Format            string   `json:"format" validate:"oneof=json text"`
    IncludeHeaders    bool     `json:"include_headers"`
    IncludeBody       bool     `json:"include_body"`
    MaxBodySize       int      `json:"max_body_size" validate:"gte=0"`
    RedactHeaders     []string `json:"redact_headers"`
    RedactBodyFields  []string `json:"redact_body_fields"`
    SkipPaths         []string `json:"skip_paths"`
    RequestIDHeader   string   `json:"request_id_header"`
}

var DefaultLoggingConfig = LoggingConfig{
    Enabled:          true,
    Level:            "info",
    Format:           "json",
    IncludeHeaders:   true,
    IncludeBody:      false,
    MaxBodySize:      1024,
    RedactHeaders:    []string{"authorization", "cookie", "x-api-key"},
    RedactBodyFields: []string{"password", "token", "secret", "api_key"},
    RequestIDHeader:  "X-Request-ID",
}
```

## Log Entry Structure

```go
type LogEntry struct {
    Timestamp    time.Time         `json:"timestamp"`
    Level        string            `json:"level"`
    RequestID    string            `json:"request_id"`
    Method       string            `json:"method"`
    Path         string            `json:"path"`
    Query        string            `json:"query,omitempty"`
    RemoteAddr   string            `json:"remote_addr"`
    UserAgent    string            `json:"user_agent"`
    StatusCode   int               `json:"status_code"`
    Duration     time.Duration     `json:"duration_ms"`
    ResponseSize int64             `json:"response_size"`
    Headers      map[string]string `json:"headers,omitempty"`
    Error        string            `json:"error,omitempty"`
}
```

## Middleware Implementation

```go
package middleware

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "regexp"
    "strings"
    "time"

    "go.uber.org/zap"
)

type responseRecorder struct {
    http.ResponseWriter
    statusCode   int
    responseSize int64
    body         *bytes.Buffer
}

func (rr *responseRecorder) WriteHeader(code int) {
    rr.statusCode = code
    rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
    n, err := rr.ResponseWriter.Write(b)
    rr.responseSize += int64(n)
    if rr.body != nil {
        rr.body.Write(b)
    }
    return n, err
}

type LoggingMiddleware struct {
    config LoggingConfig
    logger *zap.Logger
}

func NewLoggingMiddleware(config LoggingConfig, logger *zap.Logger) *LoggingMiddleware {
    return &LoggingMiddleware{
        config: config,
        logger: logger,
    }
}

func (m *LoggingMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !m.config.Enabled {
            next.ServeHTTP(w, r)
            return
        }

        for _, path := range m.config.SkipPaths {
            if r.URL.Path == path {
                next.ServeHTTP(w, r)
                return
            }
        }

        start := time.Now()
        requestID := r.Header.Get(m.config.RequestIDHeader)
        if requestID == "" {
            requestID = generateRequestID()
        }

        w.Header().Set(m.config.RequestIDHeader, requestID)

        var reqBody []byte
        if m.config.IncludeBody && r.Body != nil {
            reqBody, _ = io.ReadAll(r.Body)
            r.Body = io.NopCloser(bytes.NewReader(reqBody))
            reqBody = m.redactBody(reqBody)
        }

        recorder := &responseRecorder{
            ResponseWriter: w,
            statusCode:     http.StatusOK,
            body:           &bytes.Buffer{},
        }

        defer func() {
            duration := time.Since(start)
            entry := LogEntry{
                Timestamp:    time.Now(),
                RequestID:    requestID,
                Method:       r.Method,
                Path:         r.URL.Path,
                Query:        r.URL.RawQuery,
                RemoteAddr:   m.maskIP(r.RemoteAddr),
                UserAgent:    r.UserAgent(),
                StatusCode:   recorder.statusCode,
                Duration:     duration,
                ResponseSize: recorder.responseSize,
            }

            if m.config.IncludeHeaders {
                entry.Headers = m.redactHeaders(r.Header)
            }

            m.logEntry(entry)
        }()

        next.ServeHTTP(recorder, r)
    })
}

func (m *LoggingMiddleware) redactHeaders(headers http.Header) map[string]string {
    result := make(map[string]string)
    redactPattern := make(map[string]bool)
    for _, h := range m.config.RedactHeaders {
        redactPattern[strings.ToLower(h)] = true
    }

    for key, values := range headers {
        lowerKey := strings.ToLower(key)
        if redactPattern[lowerKey] {
            result[key] = "[REDACTED]"
        } else if len(values) > 0 {
            result[key] = values[0]
        }
    }
    return result
}

func (m *LoggingMiddleware) redactBody(body []byte) []byte {
    if len(body) == 0 || len(m.config.RedactBodyFields) == 0 {
        return body
    }

    var data map[string]interface{}
    if err := json.Unmarshal(body, &data); err != nil {
        return body
    }

    for _, field := range m.config.RedactBodyFields {
        if _, exists := data[field]; exists {
            data[field] = "[REDACTED]"
        }
    }

    result, _ := json.Marshal(data)
    if len(result) > m.config.MaxBodySize {
        return append(result[:m.config.MaxBodySize], []byte("...[truncated]")...)
    }
    return result
}

func (m *LoggingMiddleware) maskIP(addr string) string {
    if idx := strings.LastIndex(addr, ":"); idx != -1 {
        addr = addr[:idx]
    }
    if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
        addr = addr[1 : len(addr)-1]
    }
    parts := strings.Split(addr, ".")
    if len(parts) == 4 {
        return parts[0] + "." + parts[1] + ".***.***"
    }
    return addr
}

func (m *LoggingMiddleware) logEntry(entry LogEntry) {
    fields := []zap.Field{
        zap.String("request_id", entry.RequestID),
        zap.String("method", entry.Method),
        zap.String("path", entry.Path),
        zap.Int("status", entry.StatusCode),
        zap.Duration("duration", entry.Duration),
    }

    switch m.config.Level {
    case "debug":
        m.logger.Debug("http request", fields...)
    case "info":
        if entry.StatusCode >= 500 {
            m.logger.Error("http request failed", append(fields, zap.Int("status", entry.StatusCode))...)
        } else if entry.StatusCode >= 400 {
            m.logger.Warn("http request warning", fields...)
        } else {
            m.logger.Info("http request", fields...)
        }
    case "warn":
        if entry.StatusCode >= 400 {
            m.logger.Warn("http request", fields...)
        }
    case "error":
        if entry.StatusCode >= 500 {
            m.logger.Error("http request", fields...)
        }
    }
}

func generateRequestID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}
```
