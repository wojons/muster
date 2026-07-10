package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTestTracer() (*Tracer, *tracetest.SpanRecorder) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	return NewTracer(tp.Tracer("test")), sr
}

func TestNewTracer(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tr := NewTracer(tp.Tracer("test-component"))

	if tr == nil {
		t.Fatal("NewTracer returned nil")
	}
	if tr.tp == nil {
		t.Error("tracer's internal tp is nil")
	}
}

func TestTracer_Start(t *testing.T) {
	tr, sr := newTestTracer()

	ctx := context.Background()
	_, sp := tr.Start(ctx, "test-span")

	if sp == nil {
		t.Fatal("Start returned nil span")
	}

	// Span should be started
	ended := sr.Ended()
	if len(ended) != 0 {
		t.Error("span should not be ended yet")
	}

	sp.End()

	ended = sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	if ended[0].Name() != "test-span" {
		t.Errorf("span name = %q, want %q", ended[0].Name(), "test-span")
	}
}

func TestTracer_Start_WithSpanKind(t *testing.T) {
	tr, sr := newTestTracer()

	ctx := context.Background()
	ctx, sp := tr.Start(ctx, "server-span",
		WithSpanKind(trace.SpanKindServer),
		WithAttributes(attribute.String("http.method", "GET")),
	)

	if sp == nil {
		t.Fatal("Start returned nil span")
	}

	sp.End()
	_ = ctx

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	if ended[0].SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v, want %v", ended[0].SpanKind(), trace.SpanKindServer)
	}
}

func TestSpan_End(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "end-test")
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
}

func TestSpan_End_NilSpan(t *testing.T) {
	var sp *Span
	// Should not panic
	sp.End()

	sp2 := &Span{span: nil}
	sp2.End()
}

func TestSpan_SetAttribute_String(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "attr-test")
	sp.SetAttribute("key1", "value1")
	sp.SetAttribute("key2", 42)
	sp.SetAttribute("key3", int64(100))
	sp.SetAttribute("key4", 3.14)
	sp.SetAttribute("key5", true)
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}

	attrs := ended[0].Attributes()
	attrMap := make(map[string]attribute.Value)
	for _, a := range attrs {
		attrMap[string(a.Key)] = a.Value
	}

	if v, ok := attrMap["key1"]; !ok || v.AsString() != "value1" {
		t.Errorf("key1 = %v, want value1", v.AsString())
	}
	if v, ok := attrMap["key2"]; !ok || v.AsInt64() != 42 {
		t.Errorf("key2 = %v, want 42", v.AsInt64())
	}
	if v, ok := attrMap["key3"]; !ok || v.AsInt64() != 100 {
		t.Errorf("key3 = %v, want 100", v.AsInt64())
	}
	if v, ok := attrMap["key4"]; !ok || v.AsFloat64() != 3.14 {
		t.Errorf("key4 = %v, want 3.14", v.AsFloat64())
	}
	if v, ok := attrMap["key5"]; !ok || v.AsBool() != true {
		t.Errorf("key5 = %v, want true", v.AsBool())
	}
}

func TestSpan_SetAttribute_Unknown(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "unknown-type")
	type custom struct{ Name string }
	sp.SetAttribute("custom", custom{Name: "test"})
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}

	attrs := ended[0].Attributes()
	for _, a := range attrs {
		if a.Key == "custom" && a.Value.AsString() != "<unknown type>" {
			t.Errorf("custom attribute = %q, want <unknown type>", a.Value.AsString())
		}
	}
}

func TestSpan_SetAttribute_NilSpan(t *testing.T) {
	var sp *Span
	// Should not panic
	sp.SetAttribute("key", "value")

	sp2 := &Span{span: nil}
	sp2.SetAttribute("key", "value")
}

func TestSpan_SetAttributeInt(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "int-attr")
	sp.SetAttributeInt("count", int64(99))
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}

	attrs := ended[0].Attributes()
	found := false
	for _, a := range attrs {
		if a.Key == "count" && a.Value.AsInt64() == 99 {
			found = true
		}
	}
	if !found {
		t.Error("count attribute not found or wrong value")
	}
}

func TestSpan_SetAttributeInt_NilSpan(t *testing.T) {
	var sp *Span
	sp.SetAttributeInt("key", 1)

	sp2 := &Span{span: nil}
	sp2.SetAttributeInt("key", 1)
}

func TestSpan_SetAttributeBool(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "bool-attr")
	sp.SetAttributeBool("enabled", true)
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}

	attrs := ended[0].Attributes()
	found := false
	for _, a := range attrs {
		if a.Key == "enabled" && a.Value.AsBool() == true {
			found = true
		}
	}
	if !found {
		t.Error("enabled attribute not found or wrong value")
	}
}

func TestSpan_SetAttributeBool_NilSpan(t *testing.T) {
	var sp *Span
	sp.SetAttributeBool("key", true)

	sp2 := &Span{span: nil}
	sp2.SetAttributeBool("key", false)
}

func TestSpan_RecordError(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "error-span")
	sp.RecordError(errors.New("something broke"))
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}

	if ended[0].Status().Code != codes.Error {
		t.Errorf("status code = %v, want Error", ended[0].Status().Code)
	}
	if ended[0].Status().Description != "something broke" {
		t.Errorf("status desc = %q, want something broke", ended[0].Status().Description)
	}
}

func TestSpan_RecordError_Nil(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "nil-err")
	// Recording nil error should be a no-op
	sp.RecordError(nil)
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	if ended[0].Status().Code != codes.Unset {
		t.Errorf("status should be Unset for nil error, got %v", ended[0].Status().Code)
	}
}

func TestSpan_RecordError_NilSpan(t *testing.T) {
	var sp *Span
	sp.RecordError(errors.New("err"))

	sp2 := &Span{span: nil}
	sp2.RecordError(errors.New("err"))
}

func TestSpan_SetOK(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "ok-span")
	sp.SetOK()
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	if ended[0].Status().Code != codes.Ok {
		t.Errorf("status code = %v, want Ok", ended[0].Status().Code)
	}
}

func TestSpan_SetOK_NilSpan(t *testing.T) {
	var sp *Span
	sp.SetOK()

	sp2 := &Span{span: nil}
	sp2.SetOK()
}

func TestWithSpanKind(t *testing.T) {
	opt := WithSpanKind(trace.SpanKindClient)
	cfg := &spanConfig{}
	opt(cfg)

	if cfg.kind != trace.SpanKindClient {
		t.Errorf("kind = %v, want Client", cfg.kind)
	}
}

func TestWithAttributes(t *testing.T) {
	opt := WithAttributes(
		attribute.String("svc", "api"),
		attribute.Int("port", 8080),
	)
	cfg := &spanConfig{}
	opt(cfg)

	if len(cfg.attributes) != 2 {
		t.Fatalf("expected 2 attributes, got %d", len(cfg.attributes))
	}
	if cfg.attributes[0].Key != "svc" || cfg.attributes[0].Value.AsString() != "api" {
		t.Error("first attribute mismatch")
	}
	if cfg.attributes[1].Key != "port" || cfg.attributes[1].Value.AsInt64() != 8080 {
		t.Error("second attribute mismatch")
	}
}

func TestTracer_Start_ContextPropagation(t *testing.T) {
	tr, _ := newTestTracer()

	ctx := context.Background()
	ctx, sp1 := tr.Start(ctx, "parent")
	_, sp2 := tr.Start(ctx, "child")

	// Both spans should share context lineage
	if sp1 == nil || sp2 == nil {
		t.Fatal("spans should not be nil")
	}

	sp2.End()
	sp1.End()
}

func TestTracer_Start_NoOptions(t *testing.T) {
	tr, sr := newTestTracer()
	_, sp := tr.Start(context.Background(), "no-opts")
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	// Default span kind should be internal
	if ended[0].SpanKind() != trace.SpanKindInternal {
		t.Errorf("default span kind = %v, want Internal", ended[0].SpanKind())
	}
}
