// Package grpc implements the gRPC protocol adapter (specs/PROTOCOLS.md,
// matrix row 2 / gate order 1): discovery from .proto sources, compiled
// FileDescriptorSets, or gRPC server reflection; and unary invocation
// (KindUnary) through the protocol-neutral operation model.
//
// Streaming methods (server/client/bidi) are PROTO-003 scope: they are
// discovered and mapped but Execute rejects them with a mapped AppError so
// the unary gate boundary stays honest (§6 order 2 gates on this unary gate).
//
// All parsing is pure Go via jhump/protoreflect (+ bufbuild/protocompile);
// protoc is never invoked (acceptance criterion).
package grpc

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// DescriptorSourceTag values stamped into Operation.Source by every
// discovery path, so Executor (and only Executor — stage-3 consumers never
// read Source per §3.2) can recover transport ground truth.
const (
	SourceFullMethod  = "full_method" // "/pkg.Service/Method" — the canonical gRPC route
	SourceService     = "service"     // fully-qualified service name
	SourceMethod      = "method"      // method name as declared in the service
	SourceInputType   = "input_type"  // fully-qualified input message name
	SourceOutputType  = "output_type" // fully-qualified output message name
	SourceDescriptors = "descriptors" // discovery source: "proto" | "descriptorset" | "reflection"
	SourceHost        = "host"        // reflection target address, reflection path only
)

// MapMethodDescriptor maps one RPC method to a protocol-neutral Operation.
// The operation ID is "service-method" (dot-free so pkg/generator's
// normalizeOperationID kebab-casing produces clean command names).
// Message fields become RequestBodyProperty entries (In="message-field"
// parameters are NOT emitted — fields live on the request message body).
func MapMethodDescriptor(serviceName string, method *desc.MethodDescriptor) *protocol.Operation {
	opID := fmt.Sprintf("%s-%s", serviceName, method.GetName())
	kind := protocol.KindUnary
	if method.IsServerStreaming() || method.IsClientStreaming() {
		// PROTO-003 boundary: mapped (discovered, listed) but never
		// executed this row — Execute rejects with streamingRejection.
		kind = protocol.KindBidiStream
		if method.IsServerStreaming() && !method.IsClientStreaming() {
			kind = protocol.KindServerStream
		} else if !method.IsServerStreaming() && method.IsClientStreaming() {
			kind = protocol.KindClientStream
		}
	}

	op := &protocol.Operation{
		ID:          opID,
		Protocol:    protocol.ProtocolGRPC,
		Kind:        kind,
		Summary:     fmt.Sprintf("Invoke %s", opID),
		RequestBody: mapMessageFields(method.GetInputType()),
		Source: map[string]string{
			SourceFullMethod:  fmt.Sprintf("/%s/%s", serviceName, method.GetName()),
			SourceService:     serviceName,
			SourceMethod:      method.GetName(),
			SourceInputType:   method.GetInputType().GetFullyQualifiedName(),
			SourceOutputType:  method.GetOutputType().GetFullyQualifiedName(),
			SourceDescriptors: "proto",
		},
	}
	return op
}

// MapServiceDescriptor maps every method of one service to Operations.
func MapServiceDescriptor(service *desc.ServiceDescriptor) []*protocol.Operation {
	ops := make([]*protocol.Operation, 0, len(service.GetMethods()))
	for _, m := range service.GetMethods() {
		ops = append(ops, MapMethodDescriptor(service.GetFullyQualifiedName(), m))
	}
	return ops
}

// MapFileDescriptor maps all services of a file descriptor to Operations.
// Services are visited in declaration order per file and files in the order
// given, so ID sets are deterministic.
func MapFileDescriptor(fd *desc.FileDescriptor) []*protocol.Operation {
	var ops []*protocol.Operation
	for _, svc := range fd.GetServices() {
		ops = append(ops, MapServiceDescriptor(svc)...)
	}
	return ops
}

// MapFileDescriptors maps linked file descriptors to operations.
func MapFileDescriptors(fds []*desc.FileDescriptor) []*protocol.Operation {
	var ops []*protocol.Operation
	for _, fd := range fds {
		ops = append(ops, MapFileDescriptor(fd)...)
	}
	return ops
}

// mapMessageFields maps a protobuf message type into request-body properties.
// Each property carries the depth-1 fidelity facts the descriptor graph knows
// (PROTO-013 §2.5, decision D4): a nested message is referenced by FQN, a
// repeated field carries its element type, a map carries key and value types, an
// enum carries its value names, a oneof member carries its group name, and a
// well-known type carries the JSON form the wire actually uses.
func mapMessageFields(msg *desc.MessageDescriptor) []protocol.RequestBodyProperty {
	if msg == nil {
		return nil
	}
	fields := msg.GetFields()
	props := make([]protocol.RequestBodyProperty, 0, len(fields))
	for _, f := range fields {
		props = append(props, protoFieldProperty(f))
	}
	return props
}

// protoFieldProperty maps one field to a neutral request-body property.
func protoFieldProperty(f *desc.FieldDescriptor) protocol.RequestBodyProperty {
	prop := protocol.RequestBodyProperty{
		Name:        f.GetName(),
		Type:        protoFieldJSONType(f),
		Description: strings.TrimSpace(fieldComment(f)),
		Required:    f.HasPresence() && !f.IsRepeated() && f.GetOneOf() == nil && isRequiredByRule(f),
		Repeated:    f.IsRepeated(),
		Map:         f.IsMap(),
		Enum:        enumValueNames(f.GetEnumType()),
	}
	switch {
	case f.IsMap():
		// A map field is a repeated synthetic entry message on the wire and a
		// JSON object in the proto3 JSON mapping: the neutral model carries
		// the KEY and VALUE types rather than the entry type.
		key := classifyProtoFieldType(f.GetMapKeyType())
		val := classifyProtoFieldType(f.GetMapValueType())
		prop.MapKeyType = key.JSONType
		prop.MapValueType = val.JSONType
		prop.MapValueRef = val.Ref
	case f.IsRepeated():
		item := classifyProtoFieldType(f)
		prop.ItemType = item.JSONType
		prop.ItemRef = item.Ref
	default:
		declared := classifyProtoFieldType(f)
		prop.Type = declared.JSONType
		prop.Format = declared.Format
		prop.Ref = declared.Ref
	}
	// A proto3 `optional` field is carried in a SYNTHETIC oneof; that is a
	// presence mechanism, not a user-declared group, so it is not reported.
	if oo := f.GetOneOf(); oo != nil && !oo.IsSynthetic() {
		prop.Oneof = oo.GetName()
	}
	return prop
}

// protoFieldType is the JSON shape of a proto field's declared type: the type
// string a consumer renders, the JSON-schema-style format when the wire form
// has one, and the FQN reference when the JSON is an object of a named type.
type protoFieldType struct {
	JSONType string
	Format   string
	Ref      string
}

// classifyProtoFieldType classifies a field's DECLARED type (ignoring repeated
// cardinality: for a repeated field it describes the element).
func classifyProtoFieldType(f *desc.FieldDescriptor) protoFieldType {
	if f == nil {
		return protoFieldType{JSONType: "string"}
	}
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		if mt := f.GetMessageType(); mt != nil {
			if wkt, ok := wellKnownJSONForm(mt.GetFullyQualifiedName()); ok {
				return wkt
			}
			return protoFieldType{JSONType: "object", Ref: mt.GetFullyQualifiedName()}
		}
		return protoFieldType{JSONType: "object"}
	default:
		return protoFieldType{JSONType: protoFieldJSONType(f)}
	}
}

// wellKnownJSONForm maps a google.protobuf well-known type to the JSON shape its
// wire JSON takes: a Timestamp is an RFC 3339 string, a Duration a string, a
// FieldMask a string — not an object of the descriptor's own type. A well-known
// type therefore carries NO Ref: there is no object to dereference.
func wellKnownJSONForm(fqn string) (protoFieldType, bool) {
	switch fqn {
	case "google.protobuf.Timestamp":
		return protoFieldType{JSONType: "string", Format: "date-time"}, true
	case "google.protobuf.Duration":
		return protoFieldType{JSONType: "string", Format: "duration"}, true
	case "google.protobuf.FieldMask":
		return protoFieldType{JSONType: "string", Format: "field-mask"}, true
	case "google.protobuf.ListValue":
		return protoFieldType{JSONType: "array"}, true
	case "google.protobuf.Any", "google.protobuf.Struct", "google.protobuf.Value",
		"google.protobuf.Empty":
		return protoFieldType{JSONType: "object"}, true
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue":
		return protoFieldType{JSONType: "number"}, true
	case "google.protobuf.Int64Value", "google.protobuf.UInt64Value":
		return protoFieldType{JSONType: "string"}, true
	case "google.protobuf.Int32Value", "google.protobuf.UInt32Value":
		return protoFieldType{JSONType: "integer"}, true
	case "google.protobuf.BoolValue":
		return protoFieldType{JSONType: "boolean"}, true
	case "google.protobuf.StringValue", "google.protobuf.BytesValue":
		return protoFieldType{JSONType: "string"}, true
	default:
		return protoFieldType{}, false
	}
}

// enumValueNames returns an enum type's value names in declaration order — the
// values RequestBodyProperty.Enum carries (it existed but was never populated
// before PROTO-016, so an agent saw "string" with no enum array).
func enumValueNames(ed *desc.EnumDescriptor) []interface{} {
	if ed == nil {
		return nil
	}
	values := ed.GetValues()
	if len(values) == 0 {
		return nil
	}
	names := make([]interface{}, 0, len(values))
	for _, v := range values {
		names = append(names, v.GetName())
	}
	return names
}

// isRequiredByRule reports whether a field should be surfaced as required in
// the neutral model. Only proto2's explicit `required` label maps to
// required=true; proto3 has no required keyword on the wire, and proto3
// `optional` (explicit presence, not part of a oneof) stays optional so
// agents are never blocked on input the server treats as optional.
func isRequiredByRule(f *desc.FieldDescriptor) bool {
	return !f.GetFile().IsProto3() && f.IsRequired()
}

// protoFieldJSONType maps a proto field to the JSON-schema-style type
// vocabulary pkg/mcp and pkg/generator already use
// ("string","number","integer","boolean","array","object").
func protoFieldJSONType(f *desc.FieldDescriptor) string {
	if f.IsMap() {
		return "object"
	}
	if f.IsRepeated() {
		return "array"
	}
	t := f.GetType()
	switch t {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "boolean"
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64, descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		// 64-bit ints travel as JSON strings per the proto3 JSON mapping;
		// surfacing "string" keeps CLI/MCP input valid for both directions.
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32, descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return "integer"
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return "number"
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return "object"
	default:
		return "string"
	}
}

// fieldComment returns the leading comment for a field when source info is
// present (proto sources parsed with comments); best-effort, empty otherwise.
func fieldComment(f *desc.FieldDescriptor) string {
	loc := f.GetSourceInfo()
	if loc == nil {
		return ""
	}
	return loc.GetLeadingComments()
}

// loadDescriptors resolves a file-based URI into linked file descriptors
// (reflection URIs are handled by Adapter.Discover before this is called).
func loadDescriptors(uri string) ([]*desc.FileDescriptor, string, error) {
	if uri == "" {
		return nil, "", errors.New(errors.CodeValidation, "empty discovery URI")
	}
	if strings.HasSuffix(strings.ToLower(uri), ".proto") {
		fds, err := loadProtoSources(uri)
		if err != nil {
			return nil, "", err
		}
		return fds, "proto", nil
	}
	fds, err := loadDescriptorSet(uri)
	if err != nil {
		return nil, "", err
	}
	return fds, "descriptorset", nil
}

// loadDescriptorSet reads a compiled FileDescriptorSet from disk.
func loadDescriptorSet(path string) ([]*desc.FileDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeNotFound, "read descriptor set "+path)
	}
	return decodeDescriptorSet(data, path)
}

// decodeDescriptorSet decodes raw FileDescriptorSet bytes into linked
// descriptors (protoreflect handles both raw sets and ones built by protoc).
//
// A descriptor set is a DISCOVERY SOURCE (specs/PROTOCOLS.md §3.1): discovery
// returns the union of the operations declared by every file the set contains,
// so to restrict discovery to one file's services, pass that file's .proto
// source instead of a set that also carries its dependencies.
//
// A set built without `protoc --include_imports` references its imports but
// does not contain them. desc.CreateFileDescriptors resolves imports ONLY from
// the supplied set, so such a set is closed here first: dependencies the
// protobuf runtime registry knows (the well-known types the binary links in)
// are synthesized into the set, which makes the descriptor-set path behave like
// the .proto-source path for the same file. Anything still missing fails early,
// before any operation is produced, naming the file and the remedy.
func decodeDescriptorSet(data []byte, name string) ([]*desc.FileDescriptor, error) {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, errors.Wrap(err, errors.CodeValidation,
			"decode FileDescriptorSet from "+name+" (not a compiled descriptor set?)")
	}
	unresolved := closeDescriptorSetDependencies(&set)
	if len(unresolved) > 0 {
		return nil, errors.New(errors.CodeValidation, nonSelfContainedMessage(name, unresolved))
	}
	fds, err := desc.CreateFileDescriptors(set.GetFile())
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeValidation,
			"link descriptors from "+name)
	}
	out := make([]*desc.FileDescriptor, 0, len(fds))
	for _, fd := range fds {
		out = append(out, fd)
	}
	// Deterministic order for downstream ID stability.
	sort.Slice(out, func(i, j int) bool { return out[i].GetName() < out[j].GetName() })
	return out, nil
}

// closeDescriptorSetDependencies makes a descriptor set self-contained by
// appending the FileDescriptorProto of every transitively imported file the set
// does not contain but the Go protobuf runtime registry knows (the generated
// well-known-type packages register their files at init). It returns the names
// it could not resolve, in first-seen order; nil means the set is now closed.
//
// Not one new dependency: google.golang.org/protobuf is already direct
// (go.mod), and the registry lookup is the same runtime the adapter uses to
// build request messages.
func closeDescriptorSetDependencies(set *descriptorpb.FileDescriptorSet) []string {
	present := make(map[string]bool, len(set.File))
	for _, f := range set.File {
		present[f.GetName()] = true
	}
	for {
		missing := missingDependencyNames(set, present)
		if len(missing) == 0 {
			return nil
		}
		var unresolved []string
		progress := false
		for _, dep := range missing {
			fd, err := protoregistry.GlobalFiles.FindFileByPath(dep)
			if err != nil {
				unresolved = append(unresolved, dep)
				continue
			}
			fdp := protodesc.ToFileDescriptorProto(fd)
			if present[fdp.GetName()] {
				continue
			}
			set.File = append(set.File, fdp)
			present[fdp.GetName()] = true
			progress = true
		}
		if !progress {
			return unresolved
		}
	}
}

// missingDependencyNames lists the imports referenced by the set's files that
// the set itself does not contain: deduplicated, first-seen order.
func missingDependencyNames(set *descriptorpb.FileDescriptorSet, present map[string]bool) []string {
	var missing []string
	seen := make(map[string]bool)
	for _, f := range set.File {
		for _, dep := range f.GetDependency() {
			if present[dep] || seen[dep] {
				continue
			}
			seen[dep] = true
			missing = append(missing, dep)
		}
	}
	return missing
}

// nonSelfContainedMessage is the operator-facing remedy for a descriptor set
// whose imports cannot be resolved, naming every missing file and both fixes
// (PROTO-013 §3.2, branch 2).
func nonSelfContainedMessage(name string, unresolved []string) string {
	quoted := make([]string, 0, len(unresolved))
	for _, dep := range unresolved {
		quoted = append(quoted, strconv.Quote(dep))
	}
	return "descriptor set " + name + " is not self-contained: it imports " +
		strings.Join(quoted, ", ") + ", which it does not contain and which are not " +
		"well-known types the protobuf runtime resolves. " +
		"Fix: rebuild the set with `protoc --include_imports` (or the equivalent of the tool " +
		"that produced it) so the imported files travel inside the set, or supply the missing " +
		"file(s) as .proto sources / as an import-path URI of the form \"dir1:dir2=a.proto,b.proto\"."
}

// descriptorSetBytes compiles linked file descriptors into FileDescriptorSet
// bytes (used by the grpcurl conformance path and available to callers that
// need a standard artifact from a discovery run).
func descriptorSetBytes(fds []*desc.FileDescriptor) ([]byte, error) {
	set := &descriptorpb.FileDescriptorSet{}
	seen := make(map[string]bool)
	var add func(*desc.FileDescriptor)
	add = func(fd *desc.FileDescriptor) {
		name := fd.GetName()
		if seen[name] {
			return
		}
		seen[name] = true
		for _, dep := range fd.GetDependencies() {
			add(dep)
		}
		set.File = append(set.File, fd.AsFileDescriptorProto())
	}
	for _, fd := range fds {
		add(fd)
	}
	return proto.Marshal(set)
}
