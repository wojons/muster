package grpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	apperrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// This file is PROTO-018: the ROUND-TRIP and CLOSURE gate over PROTO-016's
// `.proto` support surface (PROTO-013 §2.6 S1–S3, gates G-SCHEMA/G-CLOSURE in
// specs/PROTOCOLS.md §6 rows 2a/2b).
//
// The row exists because the shipped tests assert each surface where it lives:
// schema_test.go reads a schema straight out of a loader, closure_test.go reads
// a committed descriptor set. Neither one proves the property an operator
// depends on — that a source and the descriptor set produced FROM it describe
// the SAME schema. A schema in → descriptor set → schema out trip that drops
// field numbers, nesting or an import is a silent loss: every individual test
// stays green while the artifact does not round-trip.
//
// The trip asserted here is:
//
//	.proto source --(loadProtoSources)--> linked descriptors
//	  --(descriptorSetBytes: the package's own FileDescriptorSet producer)-->
//	  FileDescriptorSet bytes --(decodeDescriptorSet)--> descriptors --> SchemaSet
//
// and every cell asserts CONTENT (type names, per-field name/type/format/ref/
// cardinality/oneof/enum values/order, field NUMBERS, synthetic nested types,
// extensions), never "no error". Field numbers are asserted on the descriptor
// graph on both sides of the trip: the stage-2 `protocol.SchemaField` model is
// PROTO-013 §2.4's (type/cardinality/ref/map/WKT/description) and carries no
// number, so the graph is where a number can be observed — and the fixture's
// declared numbers are also pinned literally, so two ends agreeing on a WRONG
// number cannot pass.
//
// Closure is asserted three ways: the produced set is self-contained (every
// file it references travels inside it), every Ref in the schema graph resolves
// in that graph, and the guard is proven ABLE TO FAIL — the same committed
// fixture with one imported FileDescriptorProto removed goes red, naming the
// file and the remedy, and goes green again when it is restored.

// roundTripInput is one row of the round-trip table: a discovery source in,
// the schema it declares, and the literal facts the trip must preserve.
type roundTripInput struct {
	name string
	// uri is the discovery URI (the `dir=file.proto` form is used where an
	// import must resolve; see TestClosureGuard_SourceImportRemovalGoesRed).
	uri string
	// wantFieldNumbers pins each message's declared field numbers, by
	// fully-qualified message name. Literal, so the trip cannot pass by
	// agreeing with itself.
	wantFieldNumbers map[string]map[string]int32
	// wantEnumValues pins each enum type's value names in declaration order.
	wantEnumValues map[string][]string
	// wantFieldCount pins the field count of each named message.
	wantFieldCount map[string]int
	// wantImportFiles are the file names the source imports that the produced
	// set must therefore carry (transitive closure), by set-file name.
	wantImportFiles []string
	// wantExtraTypes are exactly the type names the produced set carries
	// BEYOND the source's own declarations — the imported files' types. The
	// source path reports only the files it was named (measured, and pinned in
	// TestSchema_SourcePathDoesNotCarryImportedDeclarations), so the produced
	// set is a strict superset; this asserts which types the closure adds.
	wantExtraTypes []string
}

// roundTripSchema parses a source, produces the descriptor set, decodes it back
// and returns the schema on both ends of the trip plus the produced bytes.
func roundTripSchema(t *testing.T, uri string) (in, out *protocol.SchemaSet, inFds, outFds []*desc.FileDescriptor, raw []byte) {
	t.Helper()
	var err error
	inFds, err = loadProtoSources(uri)
	if err != nil {
		t.Fatalf("parse source %s: %v", uri, err)
	}
	if len(inFds) == 0 {
		t.Fatalf("parse source %s: 0 file descriptors", uri)
	}
	in = MapFileDescriptorsToSchema(inFds)

	raw, err = descriptorSetBytes(inFds)
	if err != nil {
		t.Fatalf("produce descriptor set from %s: %v", uri, err)
	}
	if len(raw) == 0 {
		t.Fatalf("produce descriptor set from %s: 0 bytes", uri)
	}
	outFds, err = decodeDescriptorSet(raw, "<round-trip of "+uri+">")
	if err != nil {
		t.Fatalf("read the produced descriptor set back: %v", err)
	}
	out = MapFileDescriptorsToSchema(outFds)
	return in, out, inFds, outFds, raw
}

// writeTempSet writes a produced descriptor set to disk so the ADAPTER (the
// wired entry point `grpc schema`/`grpc discover` call) can ingest it.
func writeTempSet(t *testing.T, raw []byte, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write produced descriptor set: %v", err)
	}
	return path
}

// TestRoundTrip_ContentSurvivesDescriptorSet is the round-trip gate: for each
// fixture, the schema read back out of the produced descriptor set is the same
// schema the source declared, and the set carries its imports with it.
func TestRoundTrip_ContentSurvivesDescriptorSet(t *testing.T) {
	inputs := []roundTripInput{
		{
			name: "rich (nested/repeated/map/enum/WKT/oneof/optional) — schema.proto",
			uri:  "testdata/schema.proto",
			wantFieldNumbers: map[string]map[string]int32{
				"schema.Outer":            {"id": 1, "inner": 2, "list": 3, "attrs": 4, "color": 5, "created": 6, "x": 7, "y": 8, "opt": 9},
				"schema.Inner":            {"a": 1},
				"schema.Outer.AttrsEntry": {"key": 1, "value": 2},
			},
			wantEnumValues:  map[string][]string{"schema.Color": {"COLOR_UNSPECIFIED", "RED"}},
			wantFieldCount:  map[string]int{"schema.Outer": 9, "schema.Inner": 1, "schema.Outer.AttrsEntry": 2},
			wantImportFiles: []string{"google/protobuf/timestamp.proto"},
			wantExtraTypes:  []string{"google.protobuf.Timestamp"},
		},
		{
			name: "proto2 extensions — extension.proto",
			uri:  "testdata/extension.proto",
			wantFieldNumbers: map[string]map[string]int32{
				"ext.Base":   {"id": 1},
				"ext.Extra":  {"n": 1},
				"ext.Holder": {"base": 1},
			},
			wantFieldCount: map[string]int{"ext.Base": 1, "ext.Extra": 1, "ext.Holder": 1},
		},
		{
			name: "messages only, no service — msgs_only.proto",
			uri:  "testdata/msgs_only.proto",
			wantFieldNumbers: map[string]map[string]int32{
				"only.Thing": {"id": 1, "n": 2},
				"only.Other": {"name": 1},
				"only.Third": {"flag": 1},
			},
			wantEnumValues: map[string][]string{"only.Flavor": {"FLAVOR_UNSPECIFIED", "SWEET"}},
			wantFieldCount: map[string]int{"only.Thing": 2, "only.Other": 1, "only.Third": 1},
		},
		{
			name: "nested import (main.proto imports dep.proto) — the union fixture",
			uri:  "testdata=main.proto",
			wantFieldNumbers: map[string]map[string]int32{
				"mainpkg.DoRequest":  {"id": 1},
				"mainpkg.DoResponse": {"ok": 1},
				"dep.PingRequest":    {"id": 1},
			},
			wantFieldCount:  map[string]int{"mainpkg.DoRequest": 1, "mainpkg.DoResponse": 1},
			wantImportFiles: []string{"dep.proto"},
			wantExtraTypes:  []string{"dep.PingRequest"},
		},
	}

	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			in, out, inFds, outFds, raw := roundTripSchema(t, tc.uri)

			// (a) The source's own declarations survive with identical content.
			compareSetsOnCommonTypes(t, in, out, "source → set → source")

			// (b) Nothing is invented: the set carries the source's own
			// declarations plus exactly the imported files' declarations.
			assertExtrasComeFromImports(t, in, out, inFds, outFds, tc.wantExtraTypes)

			// (c) The produced set is self-contained: every import it names
			// travels inside it (the property desc.CreateFileDescriptors
			// requires, asserted against the bytes rather than assumed).
			set := decodeSetProto(t, raw)
			present := map[string]bool{}
			for _, f := range set.GetFile() {
				present[f.GetName()] = true
			}
			for _, want := range tc.wantImportFiles {
				if !present[want] {
					t.Errorf("produced set does not carry its import %q (files: %v)", want, fileNames(set))
				}
			}
			for _, f := range set.GetFile() {
				for _, dep := range f.GetDependency() {
					if !present[dep] {
						t.Errorf("produced set is not self-contained: %s imports %s, absent from the set", f.GetName(), dep)
					}
				}
			}

			// (d) Field NUMBERS survive the trip — observed on the descriptor
			// graph on both ends, and pinned against the fixture's literals so
			// a producer and a consumer that agree on a wrong number still fail.
			// The "before" graph is the source's FULL linked graph (a parse
			// returns the named files; their imports are linked in but not
			// returned), which is what descriptorSetBytes serializes.
			before, after := messageFieldNumbers(linkedFds(inFds)), messageFieldNumbers(outFds)
			for msg, want := range tc.wantFieldNumbers {
				if got, ok := before[msg]; !ok {
					t.Errorf("source graph has no message %s (numbers before: %v)", msg, before)
				} else if !reflect.DeepEqual(got, want) {
					t.Errorf("source graph %s field numbers = %v, want %v (fixture literals)", msg, got, want)
				}
				if got, ok := after[msg]; !ok {
					t.Errorf("round-tripped graph has no message %s (numbers after: %v)", msg, after)
				} else if !reflect.DeepEqual(got, want) {
					t.Errorf("round-tripped graph %s field numbers = %v, want %v", msg, got, want)
				}
			}
			for msg, want := range before {
				if got, ok := after[msg]; !ok {
					t.Errorf("message %s survived the source but not the round trip", msg)
				} else if !reflect.DeepEqual(got, want) {
					t.Errorf("message %s field numbers changed across the trip: %v → %v", msg, want, got)
				}
			}

			// (e) Enum values, in declaration order, on the schema model itself.
			for fqn, want := range tc.wantEnumValues {
				got, ok := out.Type(fqn)
				if !ok {
					t.Errorf("enum %s missing from the round-tripped set %v", fqn, out.Names())
					continue
				}
				if !reflect.DeepEqual(got.EnumValues, want) {
					t.Errorf("enum %s values = %v, want %v", fqn, got.EnumValues, want)
				}
			}

			// (f) Field counts, so a trip that silently drops a field is caught
			// even where the field body happens to compare equal.
			for fqn, want := range tc.wantFieldCount {
				got, ok := out.Type(fqn)
				if !ok {
					t.Errorf("type %s missing from the round-tripped set", fqn)
					continue
				}
				if len(got.Fields) != want {
					t.Errorf("%s carries %d fields, want %d", fqn, len(got.Fields), want)
				}
			}

			// (g) Extensions survive identically (proto2 fixture: 0 elsewhere).
			if !reflect.DeepEqual(in.Extensions, out.Extensions) {
				t.Errorf("extensions changed across the trip:\nsource: %+v\nset:    %+v", in.Extensions, out.Extensions)
			}

			// (h) Type-graph closure: every reference in the set resolves in it.
			assertRefsResolve(t, out, "round-tripped set of "+tc.uri)
		})
	}
}

// TestRoundTrip_NestingSurvivesDescriptorSet pins the nesting claim on its own:
// a nested declaration is named `<Parent>.<Nested>` on both ends, the synthetic
// map-entry type protoc generates is present with its key/value fields and
// numbers, and a sibling top-level message is NOT reported as nested.
func TestRoundTrip_NestingSurvivesDescriptorSet(t *testing.T) {
	in, out, _, _, _ := roundTripSchema(t, "testdata/schema.proto")

	for _, fqn := range []string{"schema.Outer.AttrsEntry", "schema.Inner"} {
		if _, ok := in.Type(fqn); !ok {
			t.Fatalf("source set is missing %s (nesting fixture changed?)", fqn)
		}
		got, ok := out.Type(fqn)
		if !ok {
			t.Fatalf("nested type %s did not survive the round trip (types: %v)", fqn, out.Names())
		}
		want, _ := in.Type(fqn)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("nested type %s changed across the trip:\nsource: %+v\nset:    %+v", fqn, want, got)
		}
	}

	// The map-entry type is the nesting shape that exists only in the
	// descriptor graph: its FQN nests it under the message that declares the
	// map, and its two fields carry the map's key/value types.
	entry, ok := out.Type("schema.Outer.AttrsEntry")
	if !ok {
		t.Fatal("schema.Outer.AttrsEntry missing from the round-tripped set")
	}
	if !strings.HasPrefix(entry.Name, "schema.Outer.") {
		t.Errorf("map-entry type %q is not nested under schema.Outer", entry.Name)
	}
	if len(entry.Fields) != 2 || entry.Fields[0].Name != "key" || entry.Fields[1].Name != "value" {
		t.Errorf("map-entry fields = %+v, want key and value", entry.Fields)
	}
	if entry.Fields[0].Type != "string" || entry.Fields[1].Type != "string" {
		t.Errorf("map-entry field types = %q/%q, want string/string", entry.Fields[0].Type, entry.Fields[1].Type)
	}
	// schema.Inner is a sibling declaration, not a nested one: the trip must
	// not fuse it into Outer.
	if _, nested := out.Type("schema.Outer.Inner"); nested {
		t.Error("schema.Inner was re-reported as nested under schema.Outer")
	}
}

// TestRoundTrip_AdapterReadsTheProducedSet is the round trip through the SHIPPED
// entry point: the produced bytes are written to disk and ingested by
// Adapter.Schema exactly as `grpc schema <set file>` does. The adapter's view of
// the produced artifact must equal its view of the source that produced it.
func TestRoundTrip_AdapterReadsTheProducedSet(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	fromSource, err := adapter.Schema(context.Background(), "testdata/schema.proto")
	if err != nil {
		t.Fatalf("schema from source: %v", err)
	}
	inFds, err := loadProtoSources("testdata/schema.proto")
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	raw, err := descriptorSetBytes(inFds)
	if err != nil {
		t.Fatalf("produce descriptor set: %v", err)
	}
	path := writeTempSet(t, raw, "schema-roundtrip.pb")

	fromSet, err := adapter.Schema(context.Background(), path)
	if err != nil {
		t.Fatalf("schema from the produced set: %v", err)
	}
	if fromSource.Source[SourceDescriptors] != "proto" {
		t.Errorf("source stamp = %q, want proto", fromSource.Source[SourceDescriptors])
	}
	if fromSet.Source[SourceDescriptors] != "descriptorset" {
		t.Errorf("produced-set stamp = %q, want descriptorset", fromSet.Source[SourceDescriptors])
	}
	compareSetsOnCommonTypes(t, fromSource, fromSet, "adapter source → adapter set")

	// The operation surface agrees too: the same RPC is discovered from both
	// forms of the same file, with the same full method.
	fromSourceOps, err := adapter.Discover(context.Background(), "testdata/schema.proto")
	if err != nil {
		t.Fatalf("discover from source: %v", err)
	}
	fromSetOps, err := adapter.Discover(context.Background(), path)
	if err != nil {
		t.Fatalf("discover from the produced set: %v", err)
	}
	if len(fromSourceOps) != 1 || len(fromSetOps) != 1 {
		t.Fatalf("operations source=%d set=%d, want 1 and 1", len(fromSourceOps), len(fromSetOps))
	}
	if fromSourceOps[0].ID != fromSetOps[0].ID ||
		fromSourceOps[0].Source[SourceFullMethod] != fromSetOps[0].Source[SourceFullMethod] {
		t.Errorf("operation differs across the trip: %q/%q vs %q/%q",
			fromSourceOps[0].ID, fromSourceOps[0].Source[SourceFullMethod],
			fromSetOps[0].ID, fromSetOps[0].Source[SourceFullMethod])
	}
}

// TestRoundTrip_MessagesOnlyAgreesWithTheCommittedSet closes the messages-only
// path three ways: the source, the set produced from it, and the committed
// protoc-built fixture `msgs_only.pb` all describe the same FOUR types, and all
// three still declare ZERO operations (extraction stays services-only).
func TestRoundTrip_MessagesOnlyAgreesWithTheCommittedSet(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	fromSource, err := adapter.Schema(context.Background(), "testdata/msgs_only.proto")
	if err != nil {
		t.Fatalf("schema from source: %v", err)
	}
	inFds, err := loadProtoSources("testdata/msgs_only.proto")
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	raw, err := descriptorSetBytes(inFds)
	if err != nil {
		t.Fatalf("produce descriptor set: %v", err)
	}
	fromProduced, err := adapter.Schema(context.Background(), writeTempSet(t, raw, "msgs_only-roundtrip.pb"))
	if err != nil {
		t.Fatalf("schema from produced set: %v", err)
	}
	fromCommitted, err := adapter.Schema(context.Background(), fixtureMsgsOnlyPB)
	if err != nil {
		t.Fatalf("schema from committed set %s: %v", fixtureMsgsOnlyPB, err)
	}

	want := []string{"only.Flavor", "only.Other", "only.Thing", "only.Third"}
	if got := fromSource.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("source types = %v, want %v", got, want)
	}
	if got := fromProduced.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("produced-set types = %v, want %v", got, want)
	}
	if got := fromCommitted.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("committed fixture types = %v, want %v", got, want)
	}
	// Full content, not just the names: the three sources agree type by type.
	compareSetsOnCommonTypes(t, fromSource, fromProduced, "source → produced set")
	compareSetsOnCommonTypes(t, fromSource, fromCommitted, "source → committed protoc set (msgs_only.pb)")

	// A service-less file yields no operations from ANY of the three inputs:
	// the round trip must not invent RPCs for the schema surface.
	for _, uri := range []string{"testdata/msgs_only.proto", writeTempSet(t, raw, "msgs_only-roundtrip-ops.pb"), fixtureMsgsOnlyPB} {
		ops, err := adapter.Discover(context.Background(), uri)
		if err != nil {
			t.Fatalf("discover %s: %v", uri, err)
		}
		if len(ops) != 0 {
			t.Errorf("operations = %d from %s, want 0 for a service-less file", len(ops), uri)
		}
	}
}

// TestClosureGuard_GoesRedWhenImportIsRemoved proves the closure guard is ABLE
// TO FAIL, on one fixture, by mutation rather than by fixture substitution:
// `multi_all.pb` is a closed set (main.proto + dep.proto). Removing dep.proto's
// FileDescriptorProto while leaving main.proto's import of it in place is
// exactly the artifact a bare `protoc --descriptor_set_out` produces — and the
// guard must refuse it, name the missing file and the remedy, return no
// descriptors, and discover no operations. Restoring the removed file restores
// green, with the same types and operations as the untouched baseline.
func TestClosureGuard_GoesRedWhenImportIsRemoved(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	baseline := readFixture(t, fixtureMultiAll)
	baseSet := decodeSetProto(t, baseline)
	if len(baseSet.GetFile()) != 2 {
		t.Fatalf("fixture %s holds %d files, want 2 (main.proto + dep.proto)", fixtureMultiAll, len(baseSet.GetFile()))
	}

	// Baseline is green through the wired path, and its operations are known.
	baseOps, err := adapter.Discover(context.Background(), fixtureMultiAll)
	if err != nil {
		t.Fatalf("baseline discover: %v", err)
	}
	baseIDs := opIDs(baseOps)
	if len(baseIDs) != 2 {
		t.Fatalf("baseline operations = %v, want the union of both files", baseIDs)
	}

	// The mutation: drop dep.proto, keep the import that names it.
	mutated, removed := removeSetFile(t, baseline, "dep.proto")
	if removed.GetName() != "dep.proto" {
		t.Fatalf("removed %q, want dep.proto", removed.GetName())
	}
	mutSet := decodeSetProto(t, mutated)
	if len(mutSet.GetFile()) != 1 {
		t.Fatalf("mutation left %d files, want 1 (the mutation must be real)", len(mutSet.GetFile()))
	}
	if deps := mutSet.GetFile()[0].GetDependency(); len(deps) != 1 || deps[0] != "dep.proto" {
		t.Fatalf("mutated set's file imports %v, want [dep.proto] — otherwise this is not the missing-import case", deps)
	}

	// RED: the guard fails loudly, early, and names the file and the remedy.
	fds, err := decodeDescriptorSet(mutated, "<mutation: main.proto without dep.proto>")
	if err == nil {
		t.Fatal("a set that imports a file it does not contain must fail to link")
	}
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("error = %v, want AppError CodeValidation", err)
	}
	if len(fds) != 0 {
		t.Errorf("descriptors = %d, want 0 on a failed closure", len(fds))
	}
	if msg := err.Error(); !strings.Contains(msg, "dep.proto") {
		t.Errorf("error must name the missing file; got %q", msg)
	} else if !strings.Contains(msg, "--include_imports") {
		t.Errorf("error must name the remedy; got %q", msg)
	}

	// RED through the wired path too: no partial operation list next to the error.
	mutPath := writeTempSet(t, mutated, "main-without-dep.pb")
	ops, err := adapter.Discover(context.Background(), mutPath)
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("adapter error = %v, want CodeValidation", err)
	}
	if len(ops) != 0 {
		t.Errorf("operations = %d, want 0 (a failed closure discovers nothing)", len(ops))
	}
	if _, err := adapter.Schema(context.Background(), mutPath); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("schema error = %v, want CodeValidation", err)
	}

	// GREEN restored: the removed file travels inside the set again, and both
	// the bytes-level link and the wired surface agree with the baseline.
	restored := appendSetFile(t, mutated, removed)
	restoredFds, err := decodeDescriptorSet(restored, "<restored: main.proto + dep.proto>")
	if err != nil {
		t.Fatalf("restoring the import must go green: %v", err)
	}
	if len(restoredFds) != 2 {
		t.Errorf("restored link yields %d files, want 2", len(restoredFds))
	}
	restoredOps, err := adapter.Discover(context.Background(), writeTempSet(t, restored, "restored.pb"))
	if err != nil {
		t.Fatalf("restored discover: %v", err)
	}
	if got := opIDs(restoredOps); !reflect.DeepEqual(got, baseIDs) {
		t.Errorf("restored operations = %v, want the baseline %v", got, baseIDs)
	}
	// The same schema as the untouched fixture, type for type.
	compareSetsOnCommonTypes(t,
		MapFileDescriptorsToSchema(decodeSetDescriptors(t, baseline, fixtureMultiAll)),
		MapFileDescriptorsToSchema(decodeSetDescriptors(t, restored, "<restored>")),
		"baseline → restored")
}

// TestClosureGuard_NonRemovalMutationStaysGreen is the control that makes the
// red above meaningful: the SAME mutation (remove one imported file from the
// set) stays green when the missing import is a well-known type the protobuf
// runtime registry can synthesize. So the red is caused by the import being
// UNRESOLVABLE, not by mutating a set at all.
func TestClosureGuard_NonRemovalMutationStaysGreen(t *testing.T) {
	baseline := readFixture(t, fixtureSchemaAll)
	baseSet := decodeSetProto(t, baseline)
	names := fileNames(baseSet)
	if len(names) != 2 {
		t.Fatalf("fixture %s holds files %v, want schema.proto + its WKT import", fixtureSchemaAll, names)
	}
	mutated, removed := removeSetFile(t, baseline, "google/protobuf/timestamp.proto")
	if got := fileNames(decodeSetProto(t, mutated)); len(got) != 1 {
		t.Fatalf("mutation left files %v, want 1", got)
	}

	fds, err := decodeDescriptorSet(mutated, "<mutation: schema.proto without the WKT>")
	if err != nil {
		t.Fatalf("a missing well-known-type import is resolved from the protobuf runtime registry, so this must stay green: %v", err)
	}
	got := MapFileDescriptorsToSchema(fds)
	want := MapFileDescriptorsToSchema(decodeSetDescriptors(t, baseline, fixtureSchemaAll))
	compareSetsOnCommonTypes(t, want, got, "unmutated → WKT-removed")

	// The import really was the removed file, and it is really back in the
	// linked graph — otherwise "green" would prove nothing about resolution.
	if removed.GetName() != "google/protobuf/timestamp.proto" {
		t.Fatalf("removed %q, want the WKT import", removed.GetName())
	}
	found := false
	for _, fd := range fds {
		if fd.GetName() == "google/protobuf/timestamp.proto" {
			found = true
		}
	}
	if !found {
		t.Errorf("linked files %v do not contain the synthesized import", fds)
	}
}

// TestClosureGuard_SourceImportRemovalGoesRed is the same red/green proof on the
// SOURCE path: a .proto whose imported file is absent is refused (CodeValidation,
// naming the import), and adding the file back makes the identical URI parse.
//
// It also pins the resolution boundary PROTO-018 measured: with the import
// present in the requested file's OWN directory, the bare-path form still does
// not resolve it — protosource.go's Accessor resolves imports against the import
// paths declared in the URI (or the current directory), never against the
// requested file's directory, even though its doc comment says otherwise. The
// `dir=file.proto` form is the reliable one and is what goes green below.
func TestClosureGuard_SourceImportRemovalGoesRed(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	mainSrc := readFixture(t, "testdata/main.proto")
	depSrc := readFixture(t, "testdata/dep.proto")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.proto"), mainSrc, 0o644); err != nil {
		t.Fatal(err)
	}
	uri := dir + "=main.proto" // the import-path form: dep.proto must resolve inside dir

	// RED: the imported file is absent from every import path.
	for _, form := range []string{uri, filepath.Join(dir, "main.proto")} {
		_, err := adapter.Schema(context.Background(), form)
		if !apperrors.IsError(err, apperrors.CodeValidation) {
			t.Errorf("schema(%s) with a missing import = %v, want CodeValidation", form, err)
		}
		if err == nil || !strings.Contains(err.Error(), "dep.proto") {
			t.Errorf("schema(%s) must name the missing import; got %v", form, err)
		}
	}
	if _, err := adapter.Discover(context.Background(), uri); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("discover of a source missing its import = %v, want CodeValidation", err)
	}

	// GREEN: the same URI parses once the imported file exists on its import path.
	if err := os.WriteFile(filepath.Join(dir, "dep.proto"), depSrc, 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := adapter.Schema(context.Background(), uri)
	if err != nil {
		t.Fatalf("restoring the import must go green: %v", err)
	}
	for _, want := range []string{"mainpkg.DoRequest", "mainpkg.DoResponse"} {
		if _, ok := set.Type(want); !ok {
			t.Errorf("type %s missing after restoring the import (types: %v)", want, set.Names())
		}
	}
	// The named file's OWN declarations only: the imported file is linked but
	// not reported by the source path. That asymmetry is measured and pinned in
	// TestSchema_SourcePathDoesNotCarryImportedDeclarations — asserted here so
	// this test cannot be read as claiming more than the source path gives.
	if _, ok := set.Type("dep.PingRequest"); ok {
		t.Errorf("the source-path set now reports the imported declarations (%v) — see "+
			"TestSchema_SourcePathDoesNotCarryImportedDeclarations, whose tolerance cell must flip with this one", set.Names())
	}
	// Operations from a SOURCE are the named file's only: §3.3 D6 documents the
	// union-over-all-files semantics as the descriptor SET's and names the
	// `.proto` source as the way to restrict discovery to one file. The same
	// fixture as a set (multi_all.pb) yields both files' services — pinned
	// together so the two inputs' documented difference stays visible.
	ops, err := adapter.Discover(context.Background(), uri)
	if err != nil {
		t.Fatalf("discover after restoring the import: %v", err)
	}
	if got := opIDs(ops); !reflect.DeepEqual(got, []string{"mainpkg.Main-Do"}) {
		t.Errorf("operations from the source = %v, want only the named file's service", got)
	}
	unionOps, err := adapter.Discover(context.Background(), fixtureMultiAll)
	if err != nil {
		t.Fatalf("discover the union set: %v", err)
	}
	if got := opIDs(unionOps); !reflect.DeepEqual(got, []string{"dep.Extra-Ping", "mainpkg.Main-Do"}) {
		t.Errorf("operations from the union set = %v, want both files' services", got)
	}

	// THE MEASURED RESOLUTION BOUNDARY (asserted, not smoothed over): with
	// dep.proto present next to it, the bare-path form STILL fails. If this
	// cell ever goes green, the Accessor started resolving against the
	// requested file's directory — which is what protosource.go's comment
	// claims — and the comment/behaviour drift is gone.
	if _, err := adapter.Schema(context.Background(), filepath.Join(dir, "main.proto")); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("bare path with the import present next to the file = %v; "+
			"the loader now resolves from the file's dir, so protosource.go's comment is accurate — flip this cell", err)
	}
	// The committed fixtures show the same boundary: bare path red, dir= green.
	if _, err := adapter.Schema(context.Background(), "testdata/main.proto"); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Errorf("bare testdata/main.proto with an unresolved sibling import = %v, want CodeValidation", err)
	}
	resolved, err := adapter.Schema(context.Background(), "testdata=main.proto")
	if err != nil {
		t.Fatalf("`testdata=main.proto` must resolve the sibling import: %v", err)
	}
	if _, ok := resolved.Type("mainpkg.DoRequest"); !ok {
		t.Errorf("`dir=file` form lost the named file's declarations: %v", resolved.Names())
	}
	// The import's declarations do not appear in the SOURCE-path set, but the
	// descriptor set produced from the same source carries them: the round trip
	// is what makes the graph closed (see the nesting/round-trip tests above).
	importFds, err := loadProtoSources("testdata=main.proto")
	if err != nil {
		t.Fatalf("parse testdata=main.proto: %v", err)
	}
	raw, err := descriptorSetBytes(importFds)
	if err != nil {
		t.Fatalf("produce descriptor set from the import-path form: %v", err)
	}
	produced, err := adapter.Schema(context.Background(), writeTempSet(t, raw, "main-from-dir-form.pb"))
	if err != nil {
		t.Fatalf("schema from the produced set: %v", err)
	}
	if _, ok := produced.Type("dep.PingRequest"); !ok {
		t.Errorf("the produced set must carry the imported file's declarations: %v", produced.Names())
	}
}

// crossFileRefSource imports crossFileRefDep and references a type declared in
// it — the shape that exposes whether an import's declarations travel with the
// schema set the source produces.
const crossFileRefSource = `syntax = "proto3";
package xref;
import "b.proto";
message Holder { bref.Thing thing = 1; }
service Svc { rpc Do(Holder) returns (Holder); }
`

const crossFileRefDep = `syntax = "proto3";
package bref;
message Thing { string id = 1; }
`

// TestSchema_SourcePathDoesNotCarryImportedDeclarations pins the one place the
// three discovery inputs do NOT agree, and the closure consequence it has.
//
// PROTO-013's gate G-SCHEMA item 4 and specs/PROTOCOLS.md §6 row 2b both state
// that a source, its `--include_imports` descriptor set and reflection "agree".
// Measured by PROTO-018: that holds for a file with no imports (the kv.proto
// fixture the shipped equivalence test uses) and FAILS for a source that imports
// another file. A parse returns only the files it was named — their imports are
// linked in but not returned — so the schema set produced from a SOURCE omits
// the imported declarations, while the descriptor-set path (every file the set
// contains) and the reflection path (reflectSchemaFiles walks dependencies)
// include them.
//
// The consequence is a REF that does not resolve in the set it was published
// with: `bref.Thing` below. `pkg/mcp`'s `WithSchemaSet` consumer resolves
// `SchemaField.Ref` through the set (PROTO-013 §2.5 D4), so for a `.proto`
// source the referenced type is unavailable, while the SAME file discovered as a
// descriptor set resolves it. The round trip in this file is therefore the
// repair, not the defect: the produced set is closed, the source path's own set
// is open.
//
// REPORTED, not fixed (a test-only row does not change the walk). The cells pin
// the measured behaviour and flip — loudly — when the source path starts walking
// its linked closure.
func TestSchema_SourcePathDoesNotCarryImportedDeclarations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.proto"), []byte(crossFileRefSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.proto"), []byte(crossFileRefDep), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := dir + "=a.proto"

	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	fromSource, err := adapter.Schema(context.Background(), uri)
	if err != nil {
		t.Fatalf("schema from source: %v", err)
	}
	// MEASURED: the source path's set holds the named file's declarations only.
	if got := fromSource.Names(); !reflect.DeepEqual(got, []string{"xref.Holder"}) {
		t.Errorf("source-path types = %v, want only the named file's declaration", got)
	}
	holder, ok := fromSource.Type("xref.Holder")
	if !ok {
		t.Fatalf("xref.Holder missing from %v", fromSource.Names())
	}
	if len(holder.Fields) != 1 || holder.Fields[0].Ref != "bref.Thing" {
		t.Fatalf("xref.Holder fields = %+v, want one field referencing bref.Thing", holder.Fields)
	}
	ref := holder.Fields[0].Ref
	// ...and that reference is NOT resolvable in the set it was published with.
	if _, ok := fromSource.Type(ref); ok {
		t.Errorf("the source-path set resolves %s: imported declarations are now walked — remove this cell's "+
			"tolerance and the §6 row 2b equivalence claim then holds for sources with imports too", ref)
	}
	// The gap is bounded and named: this is the only unresolved reference.
	assertRefsResolveExcept(t, fromSource, "source-path set", map[string]bool{ref: true})

	// The SAME file through the produced descriptor set is closed.
	inFds, err := loadProtoSources(uri)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	raw, err := descriptorSetBytes(inFds)
	if err != nil {
		t.Fatalf("produce descriptor set: %v", err)
	}
	outFds, err := decodeDescriptorSet(raw, "<cross-file round trip>")
	if err != nil {
		t.Fatalf("decode the produced set: %v", err)
	}
	fromSet := MapFileDescriptorsToSchema(outFds)
	if _, ok := fromSet.Type(ref); !ok {
		t.Errorf("the descriptor-set path must carry the referenced import %s (types: %v)", ref, fromSet.Names())
	}
	assertRefsResolve(t, fromSet, "descriptor-set view of the same file")
	compareSetsOnCommonTypes(t, fromSource, fromSet, "source → set (cross-file reference)")

	// Both inputs discover one operation: the RPC belongs to the named file, and
	// the union semantics §3.3 D6 documents are the descriptor SET's.
	for label, input := range map[string]string{"source": uri, "descriptor set": writeTempSet(t, raw, "xref.pb")} {
		ops, err := adapter.Discover(context.Background(), input)
		if err != nil {
			t.Fatalf("discover from %s: %v", label, err)
		}
		if got := opIDs(ops); !reflect.DeepEqual(got, []string{"xref.Svc-Do"}) {
			t.Errorf("operations from %s = %v, want the named file's one service", label, got)
		}
	}
}

// TestRoundTrip_SourceCodeInfoDescriptions is the description cell, and it
// records a MEASURED fidelity gap rather than smoothing it over.
//
// PROTO-013 §2.4 (D3) has `SchemaField`/`SchemaType` carry a `Description`, and
// PROTO-011 §5a listed "comments carried through into descriptions" as shipped.
// Measured here (PROTO-018): a source with message and field comments produces
// schema types and fields whose Description is EMPTY — `protoparse.Parser` only
// attributes source info when `IncludeSourceCodeInfo` is set, and
// `loadProtoSources` does not set it, so `commentOf`/`fieldComment` have no
// SourceCodeInfo to read. `pkg/mcp`'s renderer consumes `field.Description`, so
// the gap is on the agent-facing path as well.
//
// The assertions below pin what the tree DOES, and they are written to go red
// the moment source info starts flowing: the first cell requires descriptions to
// be empty (the measured state), the second requires the round trip to drop
// nothing the parser produced. Landing the fix means flipping the first cell to
// an equality assertion in the same commit — which is the point of pinning it.
func TestRoundTrip_SourceCodeInfoDescriptions(t *testing.T) {
	const source = `syntax = "proto3";
package cmt;

// DocThing is a documented message.
message DocThing {
  // doc_id is a documented field.
  string doc_id = 1;
  int64 big = 2;
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "commented.proto")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	in, out, _, _, _ := roundTripSchema(t, path)
	typeIn, ok := in.Type("cmt.DocThing")
	if !ok {
		t.Fatalf("cmt.DocThing missing from %v", in.Names())
	}
	typeOut, ok := out.Type("cmt.DocThing")
	if !ok {
		t.Fatalf("cmt.DocThing missing from the round-tripped set %v", out.Names())
	}

	// MEASURED STATE (flip when IncludeSourceCodeInfo lands): the source's
	// comments do not reach the model.
	if typeIn.Description != "" || typeOut.Description != "" {
		t.Errorf("descriptions are now populated (type: %q / set: %q) — the source-info gap was closed: flip this "+
			"cell to assert the comment text survives", typeIn.Description, typeOut.Description)
	}
	if in.Extensions != nil && len(in.Extensions) > 0 {
		t.Logf("note: fixture has no extensions; got %+v", in.Extensions)
	}
	if len(typeIn.Fields) == 0 {
		t.Fatalf("cmt.DocThing carries no fields: %+v", typeIn)
	}
	for _, f := range typeIn.Fields {
		if f.Description != "" {
			t.Errorf("field %s description is now populated (%q) — flip this cell", f.Name, f.Description)
		}
	}

	// THE ROUND-TRIP HALF: whatever the parser produced survives the trip
	// unchanged. This one holds today and is the property the trip must keep
	// once descriptions are populated.
	compareSetsOnCommonTypes(t, in, out, "commented source → set → source")
	if !reflect.DeepEqual(in, out) {
		t.Errorf("the trip changed the schema set:\nsource: %+v\nset:    %+v", in, out)
	}
}

// TestProtoDependencyClaimIsChecked_ProtobufIsDirect re-checks the documented
// PROTO-011 baseline instead of assuming it: PROTO-011's premise ("protobuf is
// only an indirect dependency", specs/PROTOCOLS.md:12 at that HEAD) was
// corrected to DIRECT, and the design authority's executive summary now asserts
// directness with the go.mod evidence spelled out. This cell reads go.mod and
// the spec, so the claim goes red if either side drifts.
func TestProtoDependencyClaimIsChecked_ProtobufIsDirect(t *testing.T) {
	const module = "google.golang.org/protobuf"
	goMod := readFixture(t, filepath.Join("..", "..", "..", "go.mod"))

	var reqLine string
	for _, line := range strings.Split(string(goMod), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), module+" ") {
			reqLine = strings.TrimSpace(line)
		}
	}
	if reqLine == "" {
		t.Fatalf("go.mod has no requirement line for %s", module)
	}
	if strings.Contains(reqLine, "// indirect") {
		t.Errorf("go.mod marks %s indirect: %q — PROTOCOLS.md's executive summary claims it is DIRECT", module, reqLine)
	}

	spec := readFixture(t, filepath.Join("..", "..", "..", "specs", "PROTOCOLS.md"))
	body := string(spec)
	if !strings.Contains(body, module+"` is a **direct** dependency") {
		t.Error("PROTOCOLS.md no longer states that protobuf is a direct dependency (PROTO-011's corrected premise)")
	}
	if strings.Contains(body, "only an indirect dependency") {
		t.Error("PROTOCOLS.md re-states the stale 'only an indirect dependency' premise PROTO-011 corrected")
	}
}

// --- helpers -------------------------------------------------------------

// compareSetsOnCommonTypes asserts that every type name in want exists in got
// with identical content, reporting every difference rather than the first.
func compareSetsOnCommonTypes(t *testing.T, want, got *protocol.SchemaSet, label string) {
	t.Helper()
	names := want.Names()
	if len(names) == 0 {
		t.Fatalf("%s: the source schema is empty — nothing to compare", label)
	}
	for _, name := range names {
		wantType, _ := want.Type(name)
		gotType, ok := got.Type(name)
		if !ok {
			t.Errorf("%s: type %s did not survive (got %v)", label, name, got.Names())
			continue
		}
		for _, diff := range diffSchemaType(name, wantType, gotType) {
			t.Errorf("%s: %s", label, diff)
		}
	}
}

// diffSchemaType returns one line per difference between two declarations.
func diffSchemaType(name string, want, got protocol.SchemaType) []string {
	var diffs []string
	if want.Kind != got.Kind {
		diffs = append(diffs, fmt.Sprintf("%s: kind %q → %q", name, want.Kind, got.Kind))
	}
	if want.Description != got.Description {
		diffs = append(diffs, fmt.Sprintf("%s: description %q → %q", name, want.Description, got.Description))
	}
	if !reflect.DeepEqual(want.EnumValues, got.EnumValues) {
		diffs = append(diffs, fmt.Sprintf("%s: enum values %v → %v", name, want.EnumValues, got.EnumValues))
	}
	if len(want.Fields) != len(got.Fields) {
		diffs = append(diffs, fmt.Sprintf("%s: %d fields → %d fields (%v → %v)",
			name, len(want.Fields), len(got.Fields), fieldNames(want.Fields), fieldNames(got.Fields)))
		return diffs
	}
	for i := range want.Fields {
		if !reflect.DeepEqual(want.Fields[i], got.Fields[i]) {
			diffs = append(diffs, fmt.Sprintf("%s field %d (%s): %+v → %+v",
				name, i, want.Fields[i].Name, want.Fields[i], got.Fields[i]))
		}
	}
	return diffs
}

// fieldNames lists the field names of a declaration, in order.
func fieldNames(fields []protocol.SchemaField) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Name)
	}
	return out
}

// assertExtrasComeFromImports proves the trip invents nothing: the set carries
// exactly the source's own declarations plus wantExtra — and every extra is
// really declared by a file the source imported (derived from the returned
// descriptor graph, not from the expectation itself).
func assertExtrasComeFromImports(t *testing.T, in, out *protocol.SchemaSet, inFds, outFds []*desc.FileDescriptor, wantExtra []string) {
	t.Helper()
	inFiles := map[string]bool{}
	for _, fd := range inFds {
		inFiles[fd.GetName()] = true
	}
	var depFds []*desc.FileDescriptor
	for _, fd := range outFds {
		if !inFiles[fd.GetName()] {
			depFds = append(depFds, fd)
		}
	}
	allowed := map[string]bool{}
	if len(depFds) > 0 {
		for _, name := range MapFileDescriptorsToSchema(depFds).Names() {
			allowed[name] = true
		}
	}
	var extras []string
	for _, name := range out.Names() {
		if _, ok := in.Type(name); ok {
			continue
		}
		extras = append(extras, name)
		if !allowed[name] {
			t.Errorf("the round-tripped set invented type %s: it is neither declared by the source nor by an imported file (%v)",
				name, depTypes(depFds))
		}
	}
	want := append([]string(nil), wantExtra...)
	sort.Strings(want)
	if !reflect.DeepEqual(extras, want) {
		t.Errorf("the closure added types %v, want exactly %v", extras, want)
	}
}

// depTypes names the files and types the closure added, for failure messages.
func depTypes(fds []*desc.FileDescriptor) string {
	var parts []string
	for _, fd := range fds {
		parts = append(parts, fd.GetName())
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// refSites collects every FQN reference a schema set publishes, mapped to the
// sites that reference it (for failure messages).
func refSites(set *protocol.SchemaSet) map[string][]string {
	refs := map[string][]string{}
	record := func(site, ref string) {
		if ref != "" {
			refs[ref] = append(refs[ref], site)
		}
	}
	for _, name := range set.Names() {
		typ, _ := set.Type(name)
		for _, f := range typ.Fields {
			record(name+"."+f.Name, f.Ref)
			record(name+"."+f.Name+"[]", f.ItemRef)
			record(name+"."+f.Name+"{}", f.MapValueRef)
		}
	}
	for _, ext := range set.Extensions {
		record("extension "+ext.Name, ext.Ref)
	}
	return refs
}

// assertRefsResolve is the type-graph closure rule: every FQN the schema set
// references must resolve inside that same set. A non-empty Ref is always a
// named message type (well-known types carry their JSON form instead of a Ref,
// by design), so there is no exception to carve out here.
func assertRefsResolve(t *testing.T, set *protocol.SchemaSet, label string) {
	t.Helper()
	assertRefsResolveExcept(t, set, label, nil)
}

// assertRefsResolveExcept is the closure rule with explicitly tolerated
// unresolved references (used where a measured gap is pinned): every reference
// outside `tolerate` must resolve, every tolerated reference must be BOTH
// present and still unresolved — so the tolerance cannot silently grow, and the
// cell goes red the moment the gap it documents is closed.
func assertRefsResolveExcept(t *testing.T, set *protocol.SchemaSet, label string, tolerate map[string]bool) {
	t.Helper()
	for ref, sites := range refSites(set) {
		_, resolves := set.Type(ref)
		if tolerate[ref] {
			if resolves {
				t.Errorf("%s: the tolerated reference %q now resolves — the measured gap is closed: remove it from the tolerance", label, ref)
			}
			continue
		}
		if !resolves {
			sort.Strings(sites)
			t.Errorf("%s: reference %q (from %s) does not resolve in the set", label, ref, strings.Join(sites, ", "))
		}
	}
	if len(tolerate) > 0 {
		seen := 0
		for ref := range refSites(set) {
			if tolerate[ref] {
				seen++
			}
		}
		if seen != len(tolerate) {
			t.Errorf("%s: %d of the %d tolerated references are not published by this set — the tolerance is stale",
				label, len(tolerate)-seen, len(tolerate))
		}
	}
}

// linkedFds expands a descriptor list with its transitive imports (a parse
// returns the files it was asked for; their imports are linked in but not
// returned), deduplicated by file name in first-seen order.
func linkedFds(fds []*desc.FileDescriptor) []*desc.FileDescriptor {
	seen := map[string]bool{}
	var out []*desc.FileDescriptor
	var add func([]*desc.FileDescriptor)
	add = func(list []*desc.FileDescriptor) {
		for _, fd := range list {
			if fd == nil || seen[fd.GetName()] {
				continue
			}
			seen[fd.GetName()] = true
			out = append(out, fd)
			add(fd.GetDependencies())
		}
	}
	add(fds)
	return out
}

// messageFieldNumbers maps each message's fully-qualified name to its fields'
// declared numbers, keyed by field name. This is the graph-side view of the
// numbers the stage-2 SchemaField model does not carry.
func messageFieldNumbers(fds []*desc.FileDescriptor) map[string]map[string]int32 {
	out := map[string]map[string]int32{}
	var walk func(msgs []*desc.MessageDescriptor)
	walk = func(msgs []*desc.MessageDescriptor) {
		for _, m := range msgs {
			nums := map[string]int32{}
			for _, f := range m.GetFields() {
				nums[f.GetName()] = f.GetNumber()
			}
			out[m.GetFullyQualifiedName()] = nums
			walk(m.GetNestedMessageTypes())
		}
	}
	for _, fd := range fds {
		walk(fd.GetMessageTypes())
	}
	return out
}

// opIDs returns the operation IDs of a discovery result, sorted.
func opIDs(ops []*protocol.Operation) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.ID)
	}
	sort.Strings(out)
	return out
}

// decodeSetProto unmarshals descriptor-set bytes.
func decodeSetProto(t *testing.T, raw []byte) *descriptorpb.FileDescriptorSet {
	t.Helper()
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatalf("unmarshal descriptor set: %v", err)
	}
	return &set
}

// fileNames lists the file names a descriptor set holds, in order.
func fileNames(set *descriptorpb.FileDescriptorSet) []string {
	out := make([]string, 0, len(set.GetFile()))
	for _, f := range set.GetFile() {
		out = append(out, f.GetName())
	}
	return out
}

// removeSetFile returns the set bytes with the named FileDescriptorProto
// removed, plus the removed entry (so the caller can prove the mutation was the
// one it intended).
func removeSetFile(t *testing.T, raw []byte, name string) ([]byte, *descriptorpb.FileDescriptorProto) {
	t.Helper()
	set := decodeSetProto(t, raw)
	kept := make([]*descriptorpb.FileDescriptorProto, 0, len(set.GetFile()))
	var removed *descriptorpb.FileDescriptorProto
	for _, f := range set.GetFile() {
		if f.GetName() == name && removed == nil {
			removed = f
			continue
		}
		kept = append(kept, f)
	}
	if removed == nil {
		t.Fatalf("set does not contain %q (files: %v)", name, fileNames(set))
	}
	set.File = kept
	out, err := proto.Marshal(set)
	if err != nil {
		t.Fatalf("marshal mutated set: %v", err)
	}
	return out, removed
}

// appendSetFile returns the set bytes with one FileDescriptorProto appended.
func appendSetFile(t *testing.T, raw []byte, fdp *descriptorpb.FileDescriptorProto) []byte {
	t.Helper()
	set := decodeSetProto(t, raw)
	if set.File == nil {
		t.Fatal("mutated set has no files")
	}
	for _, f := range set.GetFile() {
		if f.GetName() == fdp.GetName() {
			t.Fatalf("cannot restore %q: already present", fdp.GetName())
		}
	}
	set.File = append(set.File, fdp)
	out, err := proto.Marshal(set)
	if err != nil {
		t.Fatalf("marshal restored set: %v", err)
	}
	return out
}

// decodeSetDescriptors links a committed fixture, failing the test on error.
func decodeSetDescriptors(t *testing.T, raw []byte, name string) []*desc.FileDescriptor {
	t.Helper()
	fds, err := decodeDescriptorSet(raw, name)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return fds
}
