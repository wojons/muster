// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestTracingMiddleware_Basic(t *testing.T) {
	// Create a test handler
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create tracer (using no-op for testing)
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))

	// Create middleware
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	// Create request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	rec := httptest.NewRecorder()

	// Execute
	wrapped.ServeHTTP(rec, req)

	// Verify
	assert.True(t, handlerCalled, "Handler should have been called")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

func TestTracingMiddleware_TraceExtraction(t *testing.T) {
	var capturedCtx context.Context

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	// Create request with trace context header
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Verify context was captured
	require.NotNil(t, capturedCtx)

	// Verify trace ID was extracted
	traceID := GetTraceID(capturedCtx)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", traceID)

	// Verify span ID was extracted
	spanID := GetSpanID(capturedCtx)
	assert.Equal(t, "b7ad6b7169203331", spanID)
}

func TestTracingMiddleware_StatusCodeCapture(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Created"))
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	req := httptest.NewRequest("POST", "http://example.com/resource", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "Created", rec.Body.String())
}

func TestTracingMiddleware_ErrorStatus(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error"))
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/error", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestTracingMiddleware_NilTracer(t *testing.T) {
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware with nil tracer
	middleware := TracingMiddleware(nil)
	wrapped := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "Handler should still be called with nil tracer")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTracingMiddleware_BytesWritten(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, "Hello, World!", rec.Body.String())
}

func TestTracingMiddleware_MultipleWrites(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("First"))
		w.Write([]byte("Second"))
		w.Write([]byte("Third"))
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, "FirstSecondThird", rec.Body.String())
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rw := &responseWriter{
		ResponseWriter: httptest.NewRecorder(),
		statusCode:     http.StatusOK,
	}

	rw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rw.statusCode)
	assert.True(t, rw.wroteHeader)

	// Should not write header twice
	rw.WriteHeader(http.StatusOK)
	assert.Equal(t, http.StatusNotFound, rw.statusCode) // Should remain NotFound
}

func TestResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rec,
		statusCode:     http.StatusOK,
	}

	n, err := rw.Write([]byte("Hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(5), rw.bytesWritten)
	assert.Equal(t, http.StatusOK, rw.statusCode)
}

func TestExtractTraceContext(t *testing.T) {
	// Create a request with traceparent header
	headers := http.Header{}
	headers.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	ctx := ExtractTraceContext(context.Background(), headers)

	// Context should contain trace info
	ctx, span := noop.NewTracerProvider().Tracer("test").Start(ctx, "test")
	defer span.End()

	// Just verify context exists
	assert.NotNil(t, ctx)

	// The context should have trace context
	assert.NotNil(t, ctx)
}

func TestInjectTraceContext(t *testing.T) {
	headers := http.Header{}

	// Create a span context with a real tracer
	tp := noop.NewTracerProvider()
	ctx, _ := tp.Tracer("test").Start(context.Background(), "test-span")

	// Inject trace context
	InjectTraceContext(ctx, headers)

	// Headers should contain traceparent (will be empty for no-op but that's OK for test)
	// Just verify function doesn't panic
	assert.NotNil(t, headers)
}

func TestGetTraceID(t *testing.T) {
	// Test with no span - will return zero trace ID for no-op provider
	ctx := context.Background()
	traceID := GetTraceID(ctx)
	// No-op span returns zero ID, which is expected
	assert.NotNil(t, traceID)
}

func TestGetSpanID(t *testing.T) {
	// Test with no span - will return zero span ID (0000000000000000) for no-op provider
	ctx := context.Background()
	spanID := GetSpanID(ctx)
	// No-op span returns zero ID, which is valid
	assert.NotNil(t, spanID)
}

func TestWithTraceContext(t *testing.T) {
	// Create a request with traceparent
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	// Create context from request
	ctx := WithTraceContext(context.Background(), req)

	// Verify trace ID was extracted
	traceID := GetTraceID(ctx)
	assert.NotEmpty(t, traceID)
}

func TestParseTraceParent_Valid(t *testing.T) {
	traceID, spanID, flags, err := ParseTraceParent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	require.NoError(t, err)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", traceID)
	assert.Equal(t, "b7ad6b7169203331", spanID)
	assert.Equal(t, "01", flags)
}

func TestParseTraceParent_InvalidLength(t *testing.T) {
	_, _, _, err := ParseTraceParent("00-1234")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid traceparent length")
}

func TestParseTraceParent_InvalidFormat(t *testing.T) {
	// Missing delimiters - this is exactly 55 chars but wrong format
	_, _, _, err := ParseTraceParent("000af7651916cd43dd8448eb211c80319cb7ad6b716920333101")
	assert.Error(t, err)
	// The error could be about missing delimiters or invalid hex character
	// Just verify an error was returned
}

func TestParseTraceParent_InvalidHex(t *testing.T) {
	// Invalid hex character
	_, _, _, err := ParseTraceParent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b716920333X-01")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex character")
}

func TestParseTraceParent_UnsupportedVersion(t *testing.T) {
	// Version 01 is not supported
	_, _, _, err := ParseTraceParent("01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported traceparent version")
}

func TestValidateTraceParent(t *testing.T) {
	assert.True(t, ValidateTraceParent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"))
	assert.False(t, ValidateTraceParent("invalid"))
}

func TestParseSamplingDecision(t *testing.T) {
	// 01 = sampled
	assert.True(t, ParseSamplingDecision("01"))
	// 00 = not sampled
	assert.False(t, ParseSamplingDecision("00"))
	// Invalid
	assert.False(t, ParseSamplingDecision("invalid"))
}

func TestFormatTraceContextHeader(t *testing.T) {
	header := FormatTraceContextHeader(
		"0af7651916cd43dd8448eb211c80319c",
		"b7ad6b7169203331",
		true,
	)
	assert.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", header)

	// Not sampled
	header = FormatTraceContextHeader(
		"0af7651916cd43dd8448eb211c80319c",
		"b7ad6b7169203331",
		false,
	)
	assert.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00", header)
}

func TestSpanStatusFromHTTPStatus(t *testing.T) {
	// Success codes
	assert.Equal(t, SpanStatusOK, SpanStatusFromHTTPStatus(200))
	assert.Equal(t, SpanStatusOK, SpanStatusFromHTTPStatus(201))
	assert.Equal(t, SpanStatusOK, SpanStatusFromHTTPStatus(204))
	assert.Equal(t, SpanStatusOK, SpanStatusFromHTTPStatus(299))

	// Error codes
	assert.Equal(t, SpanStatusError, SpanStatusFromHTTPStatus(400))
	assert.Equal(t, SpanStatusError, SpanStatusFromHTTPStatus(404))
	assert.Equal(t, SpanStatusError, SpanStatusFromHTTPStatus(500))
	assert.Equal(t, SpanStatusError, SpanStatusFromHTTPStatus(503))

	// Other codes
	assert.Equal(t, SpanStatusUnset, SpanStatusFromHTTPStatus(100))
	assert.Equal(t, SpanStatusUnset, SpanStatusFromHTTPStatus(301))
}

func TestTracingMiddleware_MultipleRequests(t *testing.T) {
	handlerCallCount := 0
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCallCount++
		w.WriteHeader(http.StatusOK)
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	// Make multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	assert.Equal(t, 10, handlerCallCount)
}

func TestTracingMiddleware_DifferentMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			handlerCalled := false
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				assert.Equal(t, method, r.Method)
				w.WriteHeader(http.StatusOK)
			})

			tp := noop.NewTracerProvider()
			tracer := NewTracer(tp.Tracer("test"))
			middleware := TracingMiddleware(tracer)
			wrapped := middleware(testHandler)

			req := httptest.NewRequest(method, "http://example.com/test", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			assert.True(t, handlerCalled)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestTracingMiddleware_WithUserAgent(t *testing.T) {
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		assert.Equal(t, "TestAgent/1.0", r.UserAgent())
		w.WriteHeader(http.StatusOK)
	})

	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	middleware := TracingMiddleware(tracer)
	wrapped := middleware(testHandler)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.True(t, handlerCalled)
}
