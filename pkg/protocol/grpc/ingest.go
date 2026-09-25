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
	"strings"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/proto"
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

// mapMessageFields flattens a protobuf message type into request-body
// properties. Scalar/enum fields map by name; nested messages are surfaced
// as "object" (agents pass JSON objects; dynamicproto converts them).
func mapMessageFields(msg *desc.MessageDescriptor) []protocol.RequestBodyProperty {
	var props []protocol.RequestBodyProperty
	if msg == nil {
		return props
	}
	for _, f := range msg.GetFields() {
		props = append(props, protocol.RequestBodyProperty{
			Name:        f.GetName(),
			Type:        protoFieldJSONType(f),
			Description: strings.TrimSpace(fieldComment(f)),
			Required:    f.HasPresence() && !f.IsRepeated() && f.GetOneOf() == nil && isRequiredByRule(f),
		})
	}
	return props
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
func decodeDescriptorSet(data []byte, name string) ([]*desc.FileDescriptor, error) {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, errors.Wrap(err, errors.CodeValidation,
			"decode FileDescriptorSet from "+name+" (not a compiled descriptor set?)")
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
