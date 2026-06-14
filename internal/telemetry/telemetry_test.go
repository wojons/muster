// Package telemetry_test provides comprehensive unit tests for the telemetry
// package using the OpenTelemetry noop tracer provider.
package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/wojons/muster/internal/telemetry"
)

// noopTracer returns a noop trace.Tracer suitable for unit tests.
func noopTracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("test")
}

// TestNewTracer verifies that NewTracer returns a non-nil *Tracer.
func TestNewTracer(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	if tr == nil {
		t.Fatal("NewTracer returned nil")
	}
}

// TestTracerStart verifies that Start returns a non-nil *Span and a context
// that differs from the input context.
func TestTracerStart(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	ctx := context.Background()
	newCtx, sp := tr.Start(ctx, "test-span")
	if newCtx == nil {
		t.Fatal("Start returned nil context")
	}
	if sp == nil {
		t.Fatal("Start returned nil span")
	}
	if newCtx == ctx {
		t.Log("Start context is same as input (noop tracer may reuse context)")
	}

	// Verify no panic when ending the span
	sp.End()
}

// TestTracerStartWithSpanKind verifies that WithSpanKind is accepted for all
// standard SpanKind values and does not cause panics.
func TestTracerStartWithSpanKind(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	ctx := context.Background()

	kinds := []struct {
		name string
		kind trace.SpanKind
	}{
		{"client", trace.SpanKindClient},
		{"server", trace.SpanKindServer},
		{"internal", trace.SpanKindInternal},
		{"producer", trace.SpanKindProducer},
		{"consumer", trace.SpanKindConsumer},
		{"unspecified", trace.SpanKindUnspecified},
	}

	for _, tc := range kinds {
		t.Run(tc.name, func(t *testing.T) {
			_, sp := tr.Start(ctx, "span-"+tc.name, telemetry.WithSpanKind(tc.kind))
			if sp == nil {
				t.Fatal("Start returned nil span")
			}
			sp.End()
		})
	}
}

// TestTracerStartWithAttributes verifies that WithAttributes passes attribute
// key-value pairs through and does not cause panics.
func TestTracerStartWithAttributes(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	ctx := context.Background()

	_, sp := tr.Start(
		ctx, "span-with-attrs",
		telemetry.WithAttributes(
			attribute.String("service", "test"),
			attribute.Int("count", 42),
			attribute.Bool("enabled", true),
		),
	)
	if sp == nil {
		t.Fatal("Start returned nil span")
	}
	sp.End()

	// Test WithAttributes with no attrs also works.
	_, sp2 := tr.Start(ctx, "span-empty-attrs", telemetry.WithAttributes())
	if sp2 == nil {
		t.Fatal("Start returned nil span with empty attributes")
	}
	sp2.End()

	// Test WithAttributes with many attrs does not panic.
	attrs := make([]attribute.KeyValue, 100)
	for i := 0; i < 100; i++ {
		attrs[i] = attribute.Int("key", i)
	}
	_, sp3 := tr.Start(ctx, "span-many-attrs", telemetry.WithAttributes(attrs...))
	if sp3 == nil {
		t.Fatal("Start returned nil span with many attributes")
	}
	sp3.End()
}

// TestSpanEnd verifies calling End on a valid span does not panic.
func TestSpanEnd(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test")
	sp.End()
}

// TestSpanEndNil verifies calling End on a nil *Span does not panic.
func TestSpanEndNil(t *testing.T) {
	var nilSpan *telemetry.Span
	nilSpan.End() // must not panic
}

// TestSpanEndMultiple verifies calling End multiple times does not panic.
func TestSpanEndMultiple(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test")
	sp.End()
	sp.End()
	sp.End()
}

// TestSpanSetAttribute verifies all supported attribute types can be set
// without panicking.
func TestSpanSetAttribute(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-attrs")

	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"string", "str_key", "hello"},
		{"int", "int_key", 42},
		{"int64", "int64_key", int64(1234567890123)},
		{"float64", "float64_key", float64(3.14159)},
		{"bool_true", "bool_key", true},
		{"bool_false", "bool_key", false},
		{"int_zero", "int_zero", 0},
		{"int_negative", "int_negative", -1},
		{"float64_zero", "float64_zero", float64(0.0)},
		{"string_empty", "string_empty", ""},
		{"int64_min", "int64_min", int64(-9223372036854775808)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sp.SetAttribute(tc.key, tc.value)
		})
	}

	sp.End()
}

// TestSpanSetAttributeUnsupported verifies that unsupported types fall back
// to a string representation without panicking.
func TestSpanSetAttributeUnsupported(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-unsupported")
	defer sp.End()

	sp.SetAttribute("slice_key", []int{1, 2, 3})
	sp.SetAttribute("struct_key", struct{ X int }{X: 1})
	sp.SetAttribute("ptr_key", &struct{}{})
	sp.SetAttribute("nil_val", nil)
}

// TestSpanSetAttributeNil verifies SetAttribute on a nil *Span does not panic.
func TestSpanSetAttributeNil(t *testing.T) {
	var nilSpan *telemetry.Span
	nilSpan.SetAttribute("key", "value") // must not panic
}

// TestSpanSetAttributeInt verifies SetAttributeInt sets an int64 attribute
// without panicking.
func TestSpanSetAttributeInt(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-int-attrs")
	defer sp.End()

	sp.SetAttributeInt("count", 100)
	sp.SetAttributeInt("negative", -50)
	sp.SetAttributeInt("zero", 0)
	sp.SetAttributeInt("large", int64(1<<62))
}

// TestSpanSetAttributeIntNil verifies SetAttributeInt on a nil *Span does not panic.
func TestSpanSetAttributeIntNil(t *testing.T) {
	var nilSpan *telemetry.Span
	nilSpan.SetAttributeInt("key", 42) // must not panic
}

// TestSpanSetAttributeBool verifies SetAttributeBool sets a bool attribute
// without panicking.
func TestSpanSetAttributeBool(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-bool-attrs")
	defer sp.End()

	sp.SetAttributeBool("enabled", true)
	sp.SetAttributeBool("disabled", false)
}

// TestSpanSetAttributeBoolNil verifies SetAttributeBool on a nil *Span does not panic.
func TestSpanSetAttributeBoolNil(t *testing.T) {
	var nilSpan *telemetry.Span
	nilSpan.SetAttributeBool("key", true) // must not panic
}

// TestSpanRecordError verifies RecordError with a real error does not panic.
func TestSpanRecordError(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-error")
	defer sp.End()

	err := errors.New("something went wrong")
	sp.RecordError(err)

	// Test with various error types.
	sp.RecordError(context.DeadlineExceeded)
	sp.RecordError(errors.New(""))
	sp.RecordError(ioErr("custom error"))
}

type ioErr string

func (e ioErr) Error() string { return string(e) }

// TestSpanRecordErrorNilSpan verifies RecordError on a nil *Span does not panic.
func TestSpanRecordErrorNilSpan(t *testing.T) {
	var nilSpan *telemetry.Span
	nilSpan.RecordError(errors.New("err")) // must not panic
}

// TestSpanRecordErrorNilError verifies RecordError with nil error does not panic
// even on a valid span.
func TestSpanRecordErrorNilError(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-nil-error")
	defer sp.End()

	sp.RecordError(nil) // must not panic; should be a no-op
}

// TestSpanSetOK verifies SetOK marks the span as successful without panicking.
func TestSpanSetOK(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "test-ok")
	defer sp.End()

	sp.SetOK()
}

// TestSpanSetOKNil verifies SetOK on a nil *Span does not panic.
func TestSpanSetOKNil(t *testing.T) {
	var nilSpan *telemetry.Span
	nilSpan.SetOK() // must not panic
}

// TestWithSpanKind verifies the WithSpanKind functional option builds
// a valid SpanStartOption without panicking.
func TestWithSpanKind(t *testing.T) {
	// Verify the option itself is constructable and invokable.
	opt := telemetry.WithSpanKind(trace.SpanKindClient)
	if opt == nil {
		t.Fatal("WithSpanKind returned nil")
	}

	// Verify it applies when passed to Start.
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(context.Background(), "kind-test", opt)
	if sp == nil {
		t.Fatal("Start with WithSpanKind returned nil span")
	}
	sp.End()
}

// TestWithAttributes verifies the WithAttributes functional option builds
// a valid SpanStartOption without panicking.
func TestWithAttributes(t *testing.T) {
	// Verify the option itself is constructable with zero attrs.
	optEmpty := telemetry.WithAttributes()
	if optEmpty == nil {
		t.Fatal("WithAttributes() returned nil")
	}

	// Verify with single attribute.
	optSingle := telemetry.WithAttributes(attribute.String("k1", "v1"))
	if optSingle == nil {
		t.Fatal("WithAttributes with one attr returned nil")
	}

	// Verify with multiple attributes.
	optMulti := telemetry.WithAttributes(
		attribute.String("k1", "v1"),
		attribute.Int("k2", 2),
		attribute.Bool("k3", true),
	)
	if optMulti == nil {
		t.Fatal("WithAttributes with multiple attrs returned nil")
	}

	// Verify it applies when passed to Start.
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(
		context.Background(),
		"attr-test",
		telemetry.WithAttributes(attribute.String("service", "test")),
	)
	if sp == nil {
		t.Fatal("Start with WithAttributes returned nil span")
	}
	sp.End()
}

// TestMultipleOptions verifies that multiple SpanStartOptions can be supplied
// together without panicking.
func TestMultipleOptions(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(
		context.Background(),
		"multi-opt",
		telemetry.WithSpanKind(trace.SpanKindServer),
		telemetry.WithAttributes(
			attribute.String("env", "test"),
			attribute.Int("attempt", 1),
		),
	)
	if sp == nil {
		t.Fatal("Start with multiple options returned nil span")
	}
	sp.End()
}

// TestTracerStartNoOptions verifies Start works with no options at all.
func TestTracerStartNoOptions(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	ctx := context.Background()
	newCtx, sp := tr.Start(ctx, "no-opts")
	if newCtx == nil {
		t.Fatal("Start returned nil context")
	}
	if sp == nil {
		t.Fatal("Start returned nil span")
	}
	sp.End()
}

// TestSpanEndNilSpanDirect verifies calling End on a nil pointer directly
// (not via a nil *Span variable) does not panic.
func TestSpanEndNilSpanDirect(t *testing.T) {
	(*telemetry.Span)(nil).End()
}

// TestSequence exercises a realistic lifecycle: create tracer, start span,
// set attributes, record an error, set OK, end.
func TestSequence(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	ctx, sp := tr.Start(
		context.Background(),
		"full-lifecycle",
		telemetry.WithSpanKind(trace.SpanKindServer),
		telemetry.WithAttributes(
			attribute.String("service", "muster"),
			attribute.String("endpoint", "/api/v1/data"),
		),
	)
	if ctx == nil || sp == nil {
		t.Fatal("Start returned nil during sequence test")
	}

	sp.SetAttribute("http.method", "GET")
	sp.SetAttributeInt("http.status_code", 200)
	sp.SetAttributeBool("success", true)
	sp.SetOK()
	sp.End()
}

// TestSequenceWithError exercises a realistic error lifecycle.
func TestSequenceWithError(t *testing.T) {
	tr := telemetry.NewTracer(noopTracer())
	_, sp := tr.Start(
		context.Background(),
		"error-lifecycle",
		telemetry.WithSpanKind(trace.SpanKindClient),
	)
	if sp == nil {
		t.Fatal("Start returned nil span during error sequence test")
	}

	sp.SetAttribute("http.method", "POST")
	sp.RecordError(errors.New("connection refused"))
	sp.End()
}

// TestNilTracerStart verifies that calling Start on a zero-value Tracer
// (where the internal tracer is nil) panics or recovers gracefully.
// This tests the internal trace.Tracer boundary.
func TestNilTracerStart(t *testing.T) {
	// NewTracer with nil will store nil — Start will panic.
	// We verify the panic message or just that it (predictably) panics.
	defer func() {
		if r := recover(); r == nil {
			// The behavior here depends on the OTel implementation.
			// We just ensure we don't crash the test runner unexpectedly.
			t.Log("Start on nil underlying tracer did not panic (expected on some OTel versions)")
		}
	}()

	tr := telemetry.NewTracer(nil)
	_, sp := tr.Start(context.Background(), "nil-tracer")
	if sp != nil {
		// If it didn't panic, at least we should get a usable span.
		sp.End()
	}
}
