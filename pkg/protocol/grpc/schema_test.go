package grpc

import (
	"context"
	"os"
	"testing"

	apperrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// This file is the G-SCHEMA gate of PROTO-013 §2.7: the two surfaces (operations
// and types) are distinct, the depth-1 fidelity the model used to drop is now
// carried, the three discovery inputs agree, and the negative controls keep
// their class.

// TestSchemaAndDiscover_TwoSurfacesOneFile is gate item 1: the service-less
// fixture yields ZERO operations (extraction stays services-only) AND four
// types, both asserted in one test so the numbers cannot drift apart into two
// tests that no longer describe the same file.
func TestSchemaAndDiscover_TwoSurfacesOneFile(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), "testdata/msgs_only.proto")
	if err != nil {
		t.Fatalf("discover service-less source: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("operations = %d, want 0 (a file with no service has no RPCs)", len(ops))
	}

	set, err := adapter.Schema(context.Background(), "testdata/msgs_only.proto")
	if err != nil {
		t.Fatalf("schema of service-less source: %v", err)
	}
	got := set.Names()
	want := []string{"only.Flavor", "only.Other", "only.Thing", "only.Third"}
	if len(got) != len(want) {
		t.Fatalf("types = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("types = %v, want %v", got, want)
		}
	}
	if set.Source[SourceDescriptors] != "proto" {
		t.Fatalf("source stamp = %q, want proto", set.Source[SourceDescriptors])
	}

	// The descriptor-set form of the SAME file declares the same types: the
	// schema surface is not a source-form-specific walk.
	setFromSet, err := adapter.Schema(context.Background(), "testdata/msgs_only.pb")
	if err != nil {
		t.Fatalf("schema of service-less set: %v", err)
	}
	if !sameTypeNames(set, setFromSet) {
		t.Fatalf("proto types %v != descriptor-set types %v", set.Names(), setFromSet.Names())
	}
}

// TestSchema_RichFixtureDepth1Fidelity is gate item 2 against the measured
// baseline (PROTO-011 §4.2): nested Ref, repeated item type, map key/value,
// enum value NAMES (measured baseline: 0), the oneof group, and the JSON form of
// a well-known type.
func TestSchema_RichFixtureDepth1Fidelity(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	set, err := adapter.Schema(context.Background(), "testdata/schema.proto")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	outer, ok := set.Type("schema.Outer")
	if !ok {
		t.Fatalf("schema.Outer missing from %v", set.Names())
	}
	fields := map[string]protocol.SchemaField{}
	for _, f := range outer.Fields {
		fields[f.Name] = f
	}

	inner, ok := fields["inner"]
	if !ok {
		t.Fatal("field inner missing")
	}
	if inner.Ref != "schema.Inner" {
		t.Errorf("inner.Ref = %q, want schema.Inner", inner.Ref)
	}
	if _, ok := set.Type(inner.Ref); !ok {
		t.Errorf("inner.Ref %q does not resolve in the set", inner.Ref)
	}

	list, ok := fields["list"]
	if !ok {
		t.Fatal("field list missing")
	}
	if !list.Repeated || list.ItemRef != "schema.Inner" || list.ItemType != "object" {
		t.Errorf("list = %+v, want repeated items of schema.Inner", list)
	}

	attrs, ok := fields["attrs"]
	if !ok {
		t.Fatal("field attrs missing")
	}
	if !attrs.Map || attrs.MapKeyType != "string" || attrs.MapValueType != "string" {
		t.Errorf("attrs = %+v, want map<string,string>", attrs)
	}

	color, ok := fields["color"]
	if !ok {
		t.Fatal("field color missing")
	}
	if len(color.Enum) != 2 || color.Enum[0] != "COLOR_UNSPECIFIED" || color.Enum[1] != "RED" {
		t.Errorf("color.Enum = %v, want [COLOR_UNSPECIFIED RED] (baseline measured 0)", color.Enum)
	}

	created, ok := fields["created"]
	if !ok {
		t.Fatal("field created missing")
	}
	if created.Type != "string" || created.Format != "date-time" {
		t.Errorf("created = %+v, want the Timestamp JSON form (string/date-time), not object", created)
	}

	x, ok := fields["x"]
	if !ok {
		t.Fatal("field x missing")
	}
	y, ok := fields["y"]
	if !ok {
		t.Fatal("field y missing")
	}
	if x.Oneof != "choice" || y.Oneof != "choice" {
		t.Errorf("oneof members = %q/%q, want both in group choice", x.Oneof, y.Oneof)
	}
	if opt := fields["opt"]; opt.Oneof != "" {
		t.Errorf("proto3 optional field opt reported as oneof %q (synthetic presence is not a group)", opt.Oneof)
	}

	if enum, ok := set.Type("schema.Color"); !ok {
		t.Error("schema.Color missing")
	} else if len(enum.EnumValues) != 2 {
		t.Errorf("schema.Color values = %v, want 2", enum.EnumValues)
	}
}

// TestOperationRequestBody_Depth1Fidelity proves the SAME facts reach the
// operation path — the agent-facing surface — not only the schema set. Before
// PROTO-016 the operation carried 9 flat fields with 0 enum values, 0 refs and
// 0 oneof groups (PROTO-011 §4.2).
func TestOperationRequestBody_Depth1Fidelity(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), "testdata/schema.proto")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("operations = %d, want 1", len(ops))
	}
	fields := map[string]protocol.RequestBodyProperty{}
	for _, f := range ops[0].RequestBody {
		fields[f.Name] = f
	}
	if len(fields) != 9 {
		t.Fatalf("request body carries %d fields, want 9 (baseline)", len(fields))
	}
	if fields["inner"].Ref != "schema.Inner" {
		t.Errorf("inner.Ref = %q, want schema.Inner", fields["inner"].Ref)
	}
	if fields["list"].ItemRef != "schema.Inner" || fields["list"].ItemType != "object" {
		t.Errorf("list = %+v, want item type schema.Inner", fields["list"])
	}
	if fields["attrs"].MapValueType != "string" || fields["attrs"].MapKeyType != "string" {
		t.Errorf("attrs = %+v, want map key/value types", fields["attrs"])
	}
	if len(fields["color"].Enum) != 2 {
		t.Errorf("color.Enum = %v, want 2 value names (baseline measured 0)", fields["color"].Enum)
	}
	if fields["created"].Format != "date-time" || fields["created"].Type != "string" {
		t.Errorf("created = %+v, want Timestamp JSON form", fields["created"])
	}
	if fields["x"].Oneof != "choice" || fields["y"].Oneof != "choice" {
		t.Errorf("oneof members = %q/%q, want group choice", fields["x"].Oneof, fields["y"].Oneof)
	}
}

// TestSchema_SourceSetReflectionEquivalence is gate item 4: the same fixture
// discovered as .proto source, as an --include_imports descriptor set, and over
// reflection (bufconn) produces the same type graph. The reflection leg reuses
// the shipped dynamic-service fixture, so it is in-process and network-free.
func TestSchema_SourceSetReflectionEquivalence(t *testing.T) {
	fromSource := schemaOf(t, NewAdapter(), protoSourceToTemp(t))
	fromSet := schemaOf(t, NewAdapter(), descriptorSetToTemp(t))

	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()
	cc := connFor(t, lis)
	reflectAdapter := NewAdapter().WithConn(cc)
	defer func() { _ = reflectAdapter.Close() }()
	fromReflection := schemaOf(t, reflectAdapter, "grpc://bufnet")

	if !sameTypeNames(fromSource, fromSet) {
		t.Errorf("source types %v != descriptor-set types %v", fromSource.Names(), fromSet.Names())
	}
	if !sameTypeNames(fromSource, fromReflection) {
		t.Errorf("source types %v != reflection types %v", fromSource.Names(), fromReflection.Names())
	}
	for _, name := range fromSource.Names() {
		want, _ := fromSource.Type(name)
		got, ok := fromReflection.Type(name)
		if !ok {
			t.Errorf("reflection set is missing %s", name)
			continue
		}
		if len(want.Fields) != len(got.Fields) || want.Kind != got.Kind {
			t.Errorf("%s: reflection %+v != source %+v", name, got, want)
		}
	}
	if fromReflection.Source[SourceDescriptors] != "reflection" {
		t.Errorf("reflection schema source stamp = %q", fromReflection.Source[SourceDescriptors])
	}
	if fromReflection.Source[SourceHost] != "bufnet" {
		t.Errorf("reflection schema host stamp = %q", fromReflection.Source[SourceHost])
	}
}

// TestSchema_ServiceLessDescriptorSetIsAUnionOverFiles: a set is a discovery
// source for every file it contains (documented union semantics), which for the
// schema surface means both files' types are present.
func TestSchema_ServiceLessDescriptorSetIsAUnionOverFiles(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	set, err := adapter.Schema(context.Background(), fixtureSchemaAll)
	if err != nil {
		t.Fatalf("schema of set: %v", err)
	}
	for _, want := range []string{"schema.Outer", "schema.Inner", "google.protobuf.Timestamp"} {
		if _, ok := set.Type(want); !ok {
			t.Errorf("type %s missing from %v", want, set.Names())
		}
	}
}

// TestSchema_NegativeControlsKeepTheirClass is gate item 5: an unparseable
// source and a missing file still fail with CodeValidation (the schema verb
// shares the loaders, so it must not soften them).
func TestSchema_NegativeControlsKeepTheirClass(t *testing.T) {
	broken := t.TempDir() + "/broken.proto"
	if err := os.WriteFile(broken, []byte("syntax = total nonsense;"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Schema(context.Background(), broken); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("syntax-error schema error = %v, want CodeValidation", err)
	}
	if _, err := adapter.Schema(context.Background(), "/definitely/not/here.proto"); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("missing-file schema error = %v, want CodeValidation", err)
	}
}

// schemaOf discovers a schema set from a URI and fails the test on error.
func schemaOf(t *testing.T, adapter *Adapter, uri string) *protocol.SchemaSet {
	t.Helper()
	set, err := adapter.Schema(context.Background(), uri)
	if err != nil {
		t.Fatalf("schema(%s): %v", uri, err)
	}
	return set
}

// sameTypeNames reports whether two sets declare exactly the same type names.
func sameTypeNames(a, b *protocol.SchemaSet) bool {
	an, bn := a.Names(), b.Names()
	if len(an) != len(bn) {
		return false
	}
	for i := range an {
		if an[i] != bn[i] {
			return false
		}
	}
	return true
}
