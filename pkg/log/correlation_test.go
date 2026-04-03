package log

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCorrelationID(t *testing.T) {
	// Test that generated IDs are unique
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateCorrelationID()
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "Correlation ID should be unique")
		ids[id] = true
	}
}

func TestCorrelationIDHeader(t *testing.T) {
	// Test default header
	assert.Equal(t, "X-Correlation-ID", GetCorrelationIDHeader())

	// Test custom header
	SetCorrelationIDHeader("X-Request-ID")
	assert.Equal(t, "X-Request-ID", GetCorrelationIDHeader())

	// Reset for other tests
	SetCorrelationIDHeader("X-Correlation-ID")
}

func TestWithCorrelationID(t *testing.T) {
	ctx := context.Background()

	// Test with empty ID generates one
	ctx = WithCorrelationID(ctx, "")
	id := GetCorrelationID(ctx)
	assert.NotEmpty(t, id)
	assert.Len(t, id, 32) // hex encoded 16 bytes

	// Test with specific ID
	ctx = WithCorrelationID(ctx, "test-correlation-id")
	assert.Equal(t, "test-correlation-id", GetCorrelationID(ctx))
}

func TestGetCorrelationID(t *testing.T) {
	// Test nil context
	assert.Equal(t, "", GetCorrelationID(nil))

	// Test empty context
	ctx := context.Background()
	assert.Equal(t, "", GetCorrelationID(ctx))

	// Test with correlation ID
	ctx = WithCorrelationID(ctx, "test-id")
	assert.Equal(t, "test-id", GetCorrelationID(ctx))
}

func TestGetCorrelationIDOrGenerate(t *testing.T) {
	// Test with existing ID
	ctx := WithCorrelationID(context.Background(), "existing-id")
	assert.Equal(t, "existing-id", GetCorrelationIDOrGenerate(ctx))

	// Test without ID
	ctx = context.Background()
	id := GetCorrelationIDOrGenerate(ctx)
	assert.NotEmpty(t, id)
}

func TestLoggerWithCorrelationID(t *testing.T) {
	logger := Default()

	// Test without correlation ID
	ctx := context.Background()
	l := LoggerWithCorrelationID(ctx, logger)
	// Should return same logger (no correlation_id field added)
	assert.NotNil(t, l)

	// Test with correlation ID
	ctx = WithCorrelationID(ctx, "test-cid")
	l = LoggerWithCorrelationID(ctx, logger)
	assert.NotNil(t, l)
}

func TestRequestContext(t *testing.T) {
	rc := RequestContext{
		CorrelationID: "cid-123",
		TraceID:       "trace-456",
		SpanID:        "span-789",
		RequestID:     "req-abc",
		TenantID:      "tenant-def",
		UserID:        "user-ghi",
		SessionID:     "session-jkl",
		ClientIP:      "192.168.1.1",
		UserAgent:     "Test-Agent/1.0",
	}

	// Test ToFields
	fields := rc.ToFields()
	assert.Equal(t, "cid-123", fields["correlation_id"])
	assert.Equal(t, "trace-456", fields["trace_id"])
	assert.Equal(t, "span-789", fields["span_id"])
	assert.Equal(t, "req-abc", fields["request_id"])
	assert.Equal(t, "tenant-def", fields["tenant_id"])
	assert.Equal(t, "user-ghi", fields["user_id"])
	assert.Equal(t, "session-jkl", fields["session_id"])
	assert.Equal(t, "192.168.1.1", fields["client_ip"])
	assert.Equal(t, "Test-Agent/1.0", fields["user_agent"])

	// Test empty fields not included
	rc2 := RequestContext{CorrelationID: "only-cid"}
	fields2 := rc2.ToFields()
	assert.Equal(t, 1, len(fields2))
	assert.Equal(t, "only-cid", fields2["correlation_id"])
}

func TestWithRequestContext(t *testing.T) {
	ctx := context.Background()
	rc := RequestContext{
		CorrelationID: "cid-123",
		TraceID:       "trace-456",
		RequestID:     "req-abc",
		TenantID:      "tenant-def",
		UserID:        "user-ghi",
	}

	ctx = WithRequestContext(ctx, rc)

	assert.Equal(t, "cid-123", GetCorrelationID(ctx))
	assert.Equal(t, "trace-456", GetTraceID(ctx))
	assert.Equal(t, "req-abc", GetRequestID(ctx))
	assert.Equal(t, "tenant-def", GetTenantID(ctx))
	assert.Equal(t, "user-ghi", GetUserID(ctx))
}

func TestTraceID(t *testing.T) {
	ctx := context.Background()

	// Test GetTraceID returns empty by default
	assert.Equal(t, "", GetTraceID(ctx))

	// Test WithTraceID
	ctx = WithTraceID(ctx, "trace-123")
	assert.Equal(t, "trace-123", GetTraceID(ctx))
}

func TestSpanID(t *testing.T) {
	ctx := context.Background()

	// Test GetSpanID returns empty by default
	assert.Equal(t, "", GetSpanID(ctx))

	// Test WithSpanID
	ctx = WithSpanID(ctx, "span-456")
	assert.Equal(t, "span-456", GetSpanID(ctx))
}

func TestRequestID(t *testing.T) {
	ctx := context.Background()

	// Test GetRequestID returns empty by default
	assert.Equal(t, "", GetRequestID(ctx))

	// Test WithRequestID
	ctx = WithRequestID(ctx, "req-789")
	assert.Equal(t, "req-789", GetRequestID(ctx))
}

func TestTenantID(t *testing.T) {
	ctx := context.Background()

	// Test GetTenantID returns empty by default
	assert.Equal(t, "", GetTenantID(ctx))

	// Test WithTenantID
	ctx = WithTenantID(ctx, "tenant-abc")
	assert.Equal(t, "tenant-abc", GetTenantID(ctx))
}

func TestUserID(t *testing.T) {
	ctx := context.Background()

	// Test GetUserID returns empty by default
	assert.Equal(t, "", GetUserID(ctx))

	// Test WithUserID
	ctx = WithUserID(ctx, "user-def")
	assert.Equal(t, "user-def", GetUserID(ctx))
}

func TestSessionID(t *testing.T) {
	ctx := context.Background()

	// Test GetSessionID returns empty by default
	assert.Equal(t, "", GetSessionID(ctx))

	// Test WithSessionID
	ctx = WithSessionID(ctx, "session-ghi")
	assert.Equal(t, "session-ghi", GetSessionID(ctx))
}

func TestClientIP(t *testing.T) {
	ctx := context.Background()

	// Test GetClientIP returns empty by default
	assert.Equal(t, "", GetClientIP(ctx))

	// Test WithClientIP
	ctx = WithClientIP(ctx, "192.168.1.1")
	assert.Equal(t, "192.168.1.1", GetClientIP(ctx))
}

func TestUserAgent(t *testing.T) {
	ctx := context.Background()

	// Test GetUserAgent returns empty by default
	assert.Equal(t, "", GetUserAgent(ctx))

	// Test WithUserAgent
	ctx = WithUserAgent(ctx, "TestAgent/1.0")
	assert.Equal(t, "TestAgent/1.0", GetUserAgent(ctx))
}

func TestGetAllContextValues(t *testing.T) {
	ctx := context.Background()

	// Test empty context
	values := GetAllContextValues(ctx)
	assert.Empty(t, values)

	// Test with all values set
	ctx = WithCorrelationID(ctx, "cid-123")
	ctx = WithTraceID(ctx, "trace-456")
	ctx = WithSpanID(ctx, "span-789")
	ctx = WithRequestID(ctx, "req-abc")
	ctx = WithTenantID(ctx, "tenant-def")
	ctx = WithUserID(ctx, "user-ghi")
	ctx = WithSessionID(ctx, "session-jkl")
	ctx = WithClientIP(ctx, "192.168.1.1")
	ctx = WithUserAgent(ctx, "TestAgent/1.0")

	values = GetAllContextValues(ctx)
	assert.Equal(t, 9, len(values))
	assert.Equal(t, "cid-123", values["correlation_id"])
	assert.Equal(t, "trace-456", values["trace_id"])
	assert.Equal(t, "span-789", values["span_id"])
	assert.Equal(t, "req-abc", values["request_id"])
	assert.Equal(t, "tenant-def", values["tenant_id"])
	assert.Equal(t, "user-ghi", values["user_id"])
	assert.Equal(t, "session-jkl", values["session_id"])
	assert.Equal(t, "192.168.1.1", values["client_ip"])
	assert.Equal(t, "TestAgent/1.0", values["user_agent"])
}

func TestCorrelationIDMiddleware(t *testing.T) {
	// Test that middleware adds correlation ID if not present
	var capturedCtx context.Context
	handler := func(ctx context.Context) error {
		capturedCtx = ctx
		return nil
	}

	middleware := CorrelationIDMiddleware(handler)
	ctx := context.Background()
	err := middleware(ctx)
	require.NoError(t, err)

	cid := GetCorrelationID(capturedCtx)
	assert.NotEmpty(t, cid)

	// Test that existing correlation ID is preserved
	ctx = WithCorrelationID(context.Background(), "existing-cid")
	err = middleware(ctx)
	require.NoError(t, err)
	assert.Equal(t, "existing-cid", GetCorrelationID(capturedCtx))
}

func TestGenerateCorrelationID_Uniqueness(t *testing.T) {
	// Generate many IDs and ensure uniqueness
	const count = 10000
	ids := make(map[string]bool)

	for i := 0; i < count; i++ {
		id := GenerateCorrelationID()
		if ids[id] {
			t.Fatalf("Duplicate correlation ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestCorrelationID_Concurrency(t *testing.T) {
	// Test concurrent generation
	const goroutines = 100
	const idsPerGoroutine = 100

	ids := make(chan string, goroutines*idsPerGoroutine)
	done := make(chan bool)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < idsPerGoroutine; j++ {
				ids <- GenerateCorrelationID()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}
	close(ids)

	// Check for duplicates
	seen := make(map[string]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("Duplicate correlation ID in concurrent generation: %s", id)
		}
		seen[id] = true
	}

	assert.Equal(t, goroutines*idsPerGoroutine, len(seen))
}

func TestRequestContext_Partial(t *testing.T) {
	// Test that partial request context still works
	rc := RequestContext{
		CorrelationID: "cid-123",
		// Leave other fields empty
	}

	fields := rc.ToFields()
	assert.Equal(t, 1, len(fields))
	assert.Equal(t, "cid-123", fields["correlation_id"])
}

func BenchmarkGenerateCorrelationID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateCorrelationID()
	}
}

func BenchmarkWithCorrelationID(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WithCorrelationID(ctx, "test-id")
	}
}

func BenchmarkGetCorrelationID(b *testing.B) {
	ctx := WithCorrelationID(context.Background(), "test-id")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetCorrelationID(ctx)
	}
}

func TestCorrelationID_Timeout(t *testing.T) {
	// Test that correlation ID works with context timeout
	ctx, cancel := context.WithTimeout(
		WithCorrelationID(context.Background(), "test-id"),
		100*time.Millisecond,
	)
	defer cancel()

	assert.Equal(t, "test-id", GetCorrelationID(ctx))

	// Wait for timeout
	time.Sleep(200 * time.Millisecond)

	// Correlation ID should still be accessible after timeout
	assert.Equal(t, "test-id", GetCorrelationID(ctx))
}

func TestCorrelationID_Cancellation(t *testing.T) {
	// Test that correlation ID works with context cancellation
	ctx, cancel := context.WithCancel(
		WithCorrelationID(context.Background(), "test-id"),
	)

	assert.Equal(t, "test-id", GetCorrelationID(ctx))

	cancel()

	// Correlation ID should still be accessible after cancellation
	assert.Equal(t, "test-id", GetCorrelationID(ctx))
}

func TestGetAllContextValues_NilContext(t *testing.T) {
	// Test with nil context
	values := GetAllContextValues(nil)
	assert.NotNil(t, values)
	assert.Empty(t, values)
}
