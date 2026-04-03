// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddleware creates an HTTP middleware that automatically traces incoming
// requests and propagates trace context. It extracts trace context from incoming
// requests, creates spans with HTTP attributes, and propagates context to handlers.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.Handle("/api/", TracingMiddleware(tracer)(apiHandler))
//
// The middleware will:
//   - Extract trace context from W3C TraceContext headers
//   - Create a span named after the HTTP method and path
//   - Record HTTP attributes (method, URL, status code, response size)
//   - Record errors for non-2xx responses
//   - Propagate the trace context to the handler via request context
func TracingMiddleware(tracer *Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tracer == nil || !tracer.IsValid() {
				next.ServeHTTP(w, r)
				return
			}

			// Extract trace context from incoming request headers
			ctx := r.Context()
			propagator := propagation.TraceContext{}
			ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))

			// Create span name using method and route pattern
			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

			// Start a new span with the extracted context
			ctx, span := tracer.StartFromContext(ctx, spanName,
				WithSpanKind(trace.SpanKindServer),
				WithAttributes(
					semconv.HTTPMethodKey.String(r.Method),
					semconv.HTTPSchemeKey.String(r.URL.Scheme),
					semconv.HTTPTargetKey.String(r.URL.String()),
					semconv.HTTPURLKey.String(r.URL.String()),
					semconv.UserAgentOriginal(r.UserAgent()),
					semconv.HTTPRequestContentLengthKey.Int64(r.ContentLength),
				),
			)
			defer span.End()

			// Create response writer wrapper to capture status code and bytes written
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Update request with the new context containing the span
			r = r.WithContext(ctx)

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Record response attributes
			span.SetAttributes(
				semconv.HTTPStatusCodeKey.Int(wrapped.statusCode),
				semconv.HTTPResponseContentLengthKey.Int64(wrapped.bytesWritten),
			)

			// Record error status for non-2xx responses
			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrapped.statusCode))
			}

			// Record request route
			span.SetAttributes(
				semconv.HTTPRoute(r.URL.Path),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

// WriteHeader captures the status code before calling the underlying ResponseWriter
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// Write captures the bytes written before calling the underlying ResponseWriter
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// ExtractTraceContext extracts trace context from an HTTP request and returns
// a context with the extracted trace information. This is useful when you need
// to manually propagate trace context without using the middleware.
//
// Example:
//
//	ctx := ExtractTraceContext(r.Context(), r.Header)
//	// Use ctx for downstream calls
func ExtractTraceContext(ctx context.Context, header http.Header) context.Context {
	propagator := propagation.TraceContext{}
	return propagator.Extract(ctx, propagation.HeaderCarrier(header))
}

// InjectTraceContext injects trace context into HTTP headers. This is useful
// when making outgoing HTTP requests and you need to propagate the trace
// context to downstream services.
//
// Example:
//
//	req, _ := http.NewRequest("GET", "http://api.example.com/data", nil)
//	InjectTraceContext(ctx, req.Header)
//	client.Do(req)
func InjectTraceContext(ctx context.Context, header http.Header) {
	propagator := propagation.TraceContext{}
	propagator.Inject(ctx, propagation.HeaderCarrier(header))
}

// GetTraceID extracts the trace ID from a context. Returns empty string if
// no trace is present in the context.
//
// Example:
//
//	traceID := GetTraceID(ctx)
//	if traceID != "" {
//	    log.Printf("Processing request with trace ID: %s", traceID)
//	}
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// GetSpanID extracts the span ID from a context. Returns empty string if
// no span is present in the context.
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	return span.SpanContext().SpanID().String()
}

// WithTraceContext returns a copy of the parent context with the trace context
// from the provided request. This is useful for creating background goroutines
// that need to inherit trace context from an HTTP request.
//
// Example:
//
//	ctx := WithTraceContext(context.Background(), r)
//	go processInBackground(ctx, data)
func WithTraceContext(parent context.Context, r *http.Request) context.Context {
	return ExtractTraceContext(parent, r.Header)
}

// ParseTraceParent parses a traceparent header value and returns trace ID,
// span ID, and trace flags. Returns error if the header is malformed.
//
// The traceparent header format is:
// version-traceId-spanId-flags
//
// Example:
//
//	traceID, spanID, flags, err := ParseTraceParent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
//	if err != nil {
//	    log.Printf("Invalid traceparent: %v", err)
//	}
func ParseTraceParent(traceParent string) (traceID, spanID, flags string, err error) {
	// W3C TraceContext format: version-traceId-parentId-flags
	// Example: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
	if len(traceParent) != 55 {
		return "", "", "", fmt.Errorf("invalid traceparent length: %d, expected 55", len(traceParent))
	}

	// Extract parts
	if traceParent[2] != '-' || traceParent[35] != '-' || traceParent[52] != '-' {
		return "", "", "", fmt.Errorf("invalid traceparent format: missing delimiters")
	}

	version := traceParent[0:2]
	traceID = traceParent[3:35]
	spanID = traceParent[36:52]
	flags = traceParent[53:55]

	// Validate version
	if version != "00" {
		return "", "", "", fmt.Errorf("unsupported traceparent version: %s", version)
	}

	// Validate hex format
	hexChars := "0123456789abcdefABCDEF"
	for _, c := range traceID + spanID + flags {
		found := false
		for _, h := range hexChars {
			if c == h {
				found = true
				break
			}
		}
		if !found {
			return "", "", "", fmt.Errorf("invalid hex character in traceparent: %c", c)
		}
	}

	return traceID, spanID, flags, nil
}

// ValidateTraceParent validates a traceparent header value without parsing it.
// Returns true if the header is valid W3C TraceContext format.
func ValidateTraceParent(traceParent string) bool {
	_, _, _, err := ParseTraceParent(traceParent)
	return err == nil
}

// SpanStatus represents the status of a span
type SpanStatus int

const (
	// SpanStatusUnset is the default status
	SpanStatusUnset SpanStatus = iota
	// SpanStatusOK indicates success
	SpanStatusOK
	// SpanStatusError indicates an error
	SpanStatusError
)

// SpanStatusFromHTTPStatus converts an HTTP status code to a SpanStatus
func SpanStatusFromHTTPStatus(statusCode int) SpanStatus {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return SpanStatusOK
	case statusCode >= 400:
		return SpanStatusError
	default:
		return SpanStatusUnset
	}
}

// FormatTraceContextHeader formats trace context values into a W3C TraceContext
// header value.
//
// Example:
//
//	headerValue := FormatTraceContextHeader("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", true)
//	// Returns: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
func FormatTraceContextHeader(traceID, spanID string, sampled bool) string {
	flags := "00"
	if sampled {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", traceID, spanID, flags)
}

// ParseSamplingDecision parses the sampling decision from trace flags.
// Returns true if the least significant bit is set (sampled).
func ParseSamplingDecision(flags string) bool {
	if len(flags) != 2 {
		return false
	}
	// Parse the hex flags value
	val, err := strconv.ParseInt(flags, 16, 64)
	if err != nil {
		return false
	}
	// Check the least significant bit (sampled flag)
	return val&1 == 1
}
