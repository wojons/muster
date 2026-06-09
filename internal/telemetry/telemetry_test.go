package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTracer(t *testing.T) {
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))
	if tracer == nil {
		t.Fatal("NewTracer returned nil")
	}
}

func TestStartAndEnd(t *testing.T) {
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))

	ctx, span := tracer.Start(context.Background(), "test-span")
	if ctx == nil {
		t.Error("expected non-nil context")
	}
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	span.End()
}

func TestSetAttribute(t *testing.T) {
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))

	_, span := tracer.Start(context.Background(), "attr-test")
	span.SetAttribute("key", "value")
	span.SetAttributeInt("count", int64(42))
	span.SetAttributeBool("flag", true)
	span.End()
}

func TestRecordError(t *testing.T) {
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))

	_, span := tracer.Start(context.Background(), "error-test")
	span.RecordError(errors.New("test error"))
	span.End()
}

func TestSetOK(t *testing.T) {
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))

	_, span := tracer.Start(context.Background(), "ok-test")
	span.SetOK()
	span.End()
}

func TestWithSpanKind(t *testing.T) {
	tp := noop.NewTracerProvider()
	tracer := NewTracer(tp.Tracer("test"))

	ctx, span := tracer.Start(context.Background(), "kind-test")
	if ctx == nil {
		t.Error("expected non-nil context")
	}
	if span == nil {
		t.Error("expected non-nil span")
	}
	span.End()
}

func TestDefaultTracingConfig(t *testing.T) {
	cfg := DefaultTracingConfig()
	if cfg.ServiceName != "muster" {
		t.Errorf("expected ServiceName 'muster', got %s", cfg.ServiceName)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
}

func TestNewTracerProvider(t *testing.T) {
	// Disabled config should return a no-op provider
	config := DefaultTracingConfig()
	config.Enabled = false
	provider, err := NewTracerProvider(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}

	// Tracer should work even with disabled config
	tracer := provider.Tracer()
	if tracer == nil {
		t.Error("tracer is nil for disabled provider")
	}

	// Shutdown should not error
	err = provider.Shutdown(context.Background())
	if err != nil {
		t.Errorf("unexpected shutdown error: %v", err)
	}
}

func TestNewTracerProvider_Enabled(t *testing.T) {
	config := DefaultTracingConfig()
	config.Enabled = true
	provider, err := NewTracerProvider(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}

	tracer := provider.Tracer()
	if tracer == nil {
		t.Error("tracer is nil")
	}

	err = provider.Shutdown(context.Background())
	if err != nil {
		t.Errorf("unexpected shutdown error: %v", err)
	}
}
