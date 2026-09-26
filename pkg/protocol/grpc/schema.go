package grpc

import (
	"context"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// This file is PROTO-013 increment S2 (decision D1/D3): the SCHEMA surface.
//
// Operation extraction is services-only by design — operations are RPCs, and a
// .proto with no `service` has none (MapFileDescriptor's walk is unchanged). A
// schema surface answers a different question ("what types does this source
// declare?") and therefore gets its own walk over the same loaders. The two
// surfaces are deliberately distinguishable, which is the point: before this
// work `grpc discover msgs_only.proto` printed "No operations found." for both
// "nothing parsed" and "no RPCs in it", and nothing could tell them apart.
//
// There is no Executor for a SchemaSet: a schema graph is rendered, validated or
// emitted, never called.

// Schema ingests the SAME discovery sources Discover accepts (.proto source,
// compiled FileDescriptorSet, server reflection) and returns the protobuf type
// graph they declare: every message type (including nested declarations and the
// synthetic map-entry messages) and every enum type, per file in the set, plus
// the files' extension fields.
//
// The Source stamps reuse the adapter's existing vocabulary, so a SchemaSet
// says where it came from exactly as operations do.
func (a *Adapter) Schema(ctx context.Context, uri string) (*protocol.SchemaSet, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch {
	case strings.HasPrefix(uri, "grpc://"), strings.HasPrefix(uri, "reflection://"):
		target := strings.TrimPrefix(strings.TrimPrefix(uri, "grpc://"), "reflection://")
		if err := a.Configure(ctx, target); err != nil {
			return nil, err
		}
		fds, err := reflectSchemaFiles(ctx, a.conn)
		if err != nil {
			return nil, err
		}
		set := MapFileDescriptorsToSchema(fds)
		set.Source = map[string]string{SourceDescriptors: "reflection", SourceHost: target}
		return set, nil
	default:
		fds, source, err := loadDescriptors(uri)
		if err != nil {
			return nil, err
		}
		set := MapFileDescriptorsToSchema(fds)
		set.Source = map[string]string{SourceDescriptors: source}
		return set, nil
	}
}

// MapFileDescriptorsToSchema extracts the type graph declared by the given file
// descriptors. Files are visited in the order given, so a set assembled from a
// descriptor set keeps that set's (sorted) file order and the result is
// deterministic.
func MapFileDescriptorsToSchema(fds []*desc.FileDescriptor) *protocol.SchemaSet {
	set := &protocol.SchemaSet{Types: map[string]protocol.SchemaType{}}
	for _, fd := range fds {
		addFileToSchema(fd, set)
	}
	return set
}

// addFileToSchema adds one file's declarations to the set.
func addFileToSchema(fd *desc.FileDescriptor, set *protocol.SchemaSet) {
	if fd == nil {
		return
	}
	for _, msg := range fd.GetMessageTypes() {
		addMessageToSchema(msg, set)
	}
	for _, enum := range fd.GetEnumTypes() {
		addEnumToSchema(enum, set)
	}
	for _, ext := range fd.GetExtensions() {
		set.Extensions = append(set.Extensions, schemaFieldFromProperty(protoFieldProperty(ext)))
	}
}

// addMessageToSchema adds a message type and every nested declaration it
// contains. Nested messages include the synthetic `<Message>.<Field>Entry`
// types protoc generates for map fields, which is why the schema set of a
// message carrying a map has one more type than its source has `message`
// keywords.
func addMessageToSchema(msg *desc.MessageDescriptor, set *protocol.SchemaSet) {
	if msg == nil {
		return
	}
	fields := msg.GetFields()
	typ := protocol.SchemaType{
		Name:        msg.GetFullyQualifiedName(),
		Kind:        protocol.SchemaTypeMessage,
		Description: commentOf(msg.GetSourceInfo()),
	}
	typ.Fields = make([]protocol.SchemaField, 0, len(fields))
	for _, f := range fields {
		typ.Fields = append(typ.Fields, schemaFieldFromProperty(protoFieldProperty(f)))
	}
	set.Types[typ.Name] = typ

	for _, nested := range msg.GetNestedMessageTypes() {
		addMessageToSchema(nested, set)
	}
	for _, nested := range msg.GetNestedEnumTypes() {
		addEnumToSchema(nested, set)
	}
}

// addEnumToSchema adds an enum type with its value names in declaration order.
func addEnumToSchema(enum *desc.EnumDescriptor, set *protocol.SchemaSet) {
	if enum == nil {
		return
	}
	typ := protocol.SchemaType{
		Name:        enum.GetFullyQualifiedName(),
		Kind:        protocol.SchemaTypeEnum,
		Description: commentOf(enum.GetSourceInfo()),
	}
	for _, v := range enum.GetValues() {
		typ.EnumValues = append(typ.EnumValues, v.GetName())
	}
	set.Types[typ.Name] = typ
}

// schemaFieldFromProperty is the single mapping authority for both surfaces: a
// SchemaField is the RequestBodyProperty facts in the graph's own terms, so the
// operation path and the schema path can never drift.
func schemaFieldFromProperty(p protocol.RequestBodyProperty) protocol.SchemaField {
	field := protocol.SchemaField{
		Name:         p.Name,
		Type:         p.Type,
		Format:       p.Format,
		Description:  p.Description,
		Required:     p.Required,
		Ref:          p.Ref,
		Repeated:     p.Repeated,
		ItemType:     p.ItemType,
		ItemRef:      p.ItemRef,
		Map:          p.Map,
		MapKeyType:   p.MapKeyType,
		MapValueType: p.MapValueType,
		MapValueRef:  p.MapValueRef,
		Oneof:        p.Oneof,
	}
	for _, v := range p.Enum {
		if name, ok := v.(string); ok {
			field.Enum = append(field.Enum, name)
		}
	}
	return field
}

// commentOf returns a descriptor's leading comment, or "".
func commentOf(info *descriptorpb.SourceCodeInfo_Location) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.GetLeadingComments())
}

// reflectSchemaFiles fetches the file descriptors a live server exposes through
// reflection: the file containing each service, walking its dependencies so the
// type graph is complete (a service file's messages usually reference types
// declared in imported files).
func reflectSchemaFiles(ctx context.Context, cc grpc.ClientConnInterface) ([]*desc.FileDescriptor, error) {
	if cc == nil {
		return nil, errors.New(errors.CodeValidation, "gRPC adapter has no connection: use grpc://host:port or bind one")
	}
	rc := grpcreflect.NewClientAuto(ctx, cc)
	svcNames, err := rc.ListServices()
	if err != nil {
		return nil, errors.Wrap(err, mapCode(err), "list services via reflection")
	}
	seen := map[string]bool{}
	var fds []*desc.FileDescriptor
	var add func(*desc.FileDescriptor)
	add = func(fd *desc.FileDescriptor) {
		if fd == nil || seen[fd.GetName()] {
			return
		}
		seen[fd.GetName()] = true
		for _, dep := range fd.GetDependencies() {
			add(dep)
		}
		fds = append(fds, fd)
	}
	for _, name := range svcNames {
		// The reflection service itself is transport plumbing, not a schema
		// surface the operator asked about (same rule as operation discovery).
		if name == "grpc.reflection.v1.ServerReflection" ||
			name == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		fd, err := rc.FileContainingSymbol(name)
		if err != nil {
			return nil, errors.Wrap(err, mapCode(err), "fetch descriptor for service "+name+" via reflection")
		}
		add(fd)
	}
	return fds, nil
}
