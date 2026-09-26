package grpc

import (
	"context"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// Fixtures for the descriptor-set dependency-closure gate (PROTO-013 §3.4,
// gate G-CLOSURE). They are committed BYTES because a set built without
// `protoc --include_imports` cannot be produced at test time without protoc:
//
//	testdata/schema.proto       rich message + one service, imports a WKT
//	testdata/schema_nodeps.pb   `protoc --descriptor_set_out` (NO imports) — the
//	                            input measured to hard-fail with a library error
//	testdata/schema_all.pb      `protoc --include_imports` — the success baseline
//	testdata/main.proto         imports dep.proto (NOT a well-known type)
//	testdata/main_nodeps.pb     bare set for main.proto: unresolvable dependency
//	testdata/multi_all.pb       --include_imports set over two service files
//	testdata/msgs_only.pb       service-less file's set — 0 operations
//
// Sizes match PROTO-013's Appendix B (schema_nodeps.pb 542 B, schema_all.pb 800 B).
const (
	fixtureSchemaNoDeps = "testdata/schema_nodeps.pb"
	fixtureSchemaAll    = "testdata/schema_all.pb"
	fixtureSchemaProto  = "testdata/schema.proto"
	fixtureMainNoDeps   = "testdata/main_nodeps.pb"
	fixtureMultiAll     = "testdata/multi_all.pb"
	fixtureMsgsOnlyPB   = "testdata/msgs_only.pb"
)

// TestDiscover_DescriptorSetWithoutImports_ResolvesWellKnownTypes is the RED
// proof of the closure fix: this exact input — a set produced by a bare
// `protoc --descriptor_set_out` over a file importing
// google/protobuf/timestamp.proto — failed before the fix with
//
//	VALIDATION_ERROR: link descriptors from ...: no such file: "google/protobuf/timestamp.proto"
//
// because desc.CreateFileDescriptors resolves imports only from the supplied
// set. It now discovers the same operation as the --include_imports set.
func TestDiscover_DescriptorSetWithoutImports_ResolvesWellKnownTypes(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), fixtureSchemaNoDeps)
	if err != nil {
		t.Fatalf("discover set without imports: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("operations = %d, want 1 (schema.Desc-Do)", len(ops))
	}
	op := ops[0]
	if op.ID != "schema.Desc-Do" {
		t.Fatalf("operation ID = %q, want schema.Desc-Do", op.ID)
	}
	if op.Source[SourceFullMethod] != "/schema.Desc/Do" {
		t.Fatalf("full_method = %q, want /schema.Desc/Do", op.Source[SourceFullMethod])
	}
	if op.Source[SourceDescriptors] != "descriptorset" {
		t.Fatalf("source stamp = %q, want descriptorset", op.Source[SourceDescriptors])
	}
}

// TestDiscover_DescriptorSetParity_WithWithoutIncludeImports extends the
// shipped parity test (TestDiscover_DescriptorSetParity) so the parities are
// source ↔ set-with-imports ↔ set-without-imports: all three discovery inputs
// for the SAME file must yield identical operation IDs and full methods.
func TestDiscover_DescriptorSetParity_WithWithoutIncludeImports(t *testing.T) {
	inputs := map[string]string{
		"proto":                fixtureSchemaProto,
		"descriptorset":        fixtureSchemaAll,
		"descriptorset-nodeps": fixtureSchemaNoDeps,
	}
	want := map[string]string{"schema.Desc-Do": "/schema.Desc/Do"}
	for name, uri := range inputs {
		adapter := NewAdapter()
		ops, err := adapter.Discover(context.Background(), uri)
		_ = adapter.Close()
		if err != nil {
			t.Fatalf("discover %s (%s): %v", name, uri, err)
		}
		got := make(map[string]string, len(ops))
		for _, op := range ops {
			got[op.ID] = op.Source[SourceFullMethod]
		}
		if len(got) != len(want) {
			t.Fatalf("%s discovered %v, want %v", name, got, want)
		}
		for id, full := range want {
			if got[id] != full {
				t.Fatalf("%s: %s → %q, want %q", name, id, got[id], full)
			}
		}
	}
}

// TestDecodeDescriptorSet_ClosureAppendsRegistryFile proves the closure pass
// behaviourally, in both directions: the fixture really does violate
// self-containment (1 file), and the decoded descriptor list contains the
// imported well-known type the set was missing (2 files), so a consumer that
// walks the descriptors sees a complete graph rather than only a passing link.
func TestDecodeDescriptorSet_ClosureAppendsRegistryFile(t *testing.T) {
	raw := readFixture(t, fixtureSchemaNoDeps)
	if n := countSetFiles(t, raw); n != 1 {
		t.Fatalf("fixture %s holds %d files, want 1 (a self-containment violation)", fixtureSchemaNoDeps, n)
	}

	fds, err := decodeDescriptorSet(raw, fixtureSchemaNoDeps)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	names := map[string]bool{}
	for _, fd := range fds {
		names[fd.GetName()] = true
	}
	if !names["schema.proto"] {
		t.Fatalf("decoded files %v, want schema.proto present", names)
	}
	if !names["google/protobuf/timestamp.proto"] {
		t.Fatalf("decoded files %v, want the imported well-known type synthesized into the set", names)
	}
}

// TestDecodeDescriptorSet_UnresolvableDependencyFailsEarly is the negative
// branch: a set missing a dependency that is NOT a well-known type cannot be
// closed, so it fails, names the missing file, names the remedy, and returns no
// descriptors at all.
func TestDecodeDescriptorSet_UnresolvableDependencyFailsEarly(t *testing.T) {
	raw := readFixture(t, fixtureMainNoDeps)
	if n := countSetFiles(t, raw); n != 1 {
		t.Fatalf("fixture %s holds %d files, want 1 (dep.proto is absent)", fixtureMainNoDeps, n)
	}

	fds, err := decodeDescriptorSet(raw, fixtureMainNoDeps)
	if err == nil {
		t.Fatal("set with an unresolvable import must fail")
	}
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("error = %v, want AppError CodeValidation", err)
	}
	if len(fds) != 0 {
		t.Fatalf("descriptors = %d, want 0 on a failed closure", len(fds))
	}
	msg := err.Error()
	if !strings.Contains(msg, "dep.proto") {
		t.Fatalf("error must name the missing file; got %q", msg)
	}
	if !strings.Contains(msg, "--include_imports") {
		t.Fatalf("error must name the remedy; got %q", msg)
	}
}

// TestDiscover_UnresolvableDependencyReturnsNoOperations pins the CLI-visible
// half of the fail-early rule: the adapter returns an error and no operations,
// so an operator never sees a partial operation list next to a failure.
func TestDiscover_UnresolvableDependencyReturnsNoOperations(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), fixtureMainNoDeps)
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("error = %v, want AppError CodeValidation", err)
	}
	if len(ops) != 0 {
		t.Fatalf("operations = %d, want 0 on a failed link", len(ops))
	}
}

// TestDiscover_ServiceLessDescriptorSetStillZeroOperations is the no-regression
// control: closing dependencies must not invent operations for a file with no
// service (extraction stays services-only).
func TestDiscover_ServiceLessDescriptorSetStillZeroOperations(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), fixtureMsgsOnlyPB)
	if err != nil {
		t.Fatalf("discover service-less set: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("operations = %d, want 0 for a service-less file", len(ops))
	}
}

// TestDiscover_DescriptorSetUnionOverAllFiles pins the documented union
// semantics (PROTO-013 §3.3): a set built with --include_imports over a file
// that imports a file declaring its own service discovers both files'
// operations.
func TestDiscover_DescriptorSetUnionOverAllFiles(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), fixtureMultiAll)
	if err != nil {
		t.Fatalf("discover union set: %v", err)
	}
	ids := map[string]bool{}
	for _, op := range ops {
		ids[op.ID] = true
	}
	for _, want := range []string{"mainpkg.Main-Do", "dep.Extra-Ping"} {
		if !ids[want] {
			t.Fatalf("union set ids = %v, want %s", ids, want)
		}
	}
}

// readFixture reads a committed fixture file.
func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}

// countSetFiles reports how many FileDescriptorProto entries a descriptor set
// holds — the non-vacuity check for the closure tests.
func countSetFiles(t *testing.T, raw []byte) int {
	t.Helper()
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatalf("unmarshal descriptor set: %v", err)
	}
	return len(set.GetFile())
}
