package grpc

import (
	"context"
	"encoding/json"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/rate"
)

// rateTokenBucket aliases the shared limiter so the adapter's cross-cutting
// composition (§4.3) uses THE pkg/rate limiter rather than a private one.
type rateTokenBucket = rate.TokenBucket

// MetadataCarrier maps pkg/auth credentials into gRPC's native metadata
// channel (§4.1). The adapter NEVER invents a token store — callers hand it
// metadata built from pkg/auth.Token values (TokenType "Bearer" becomes
// "authorization: Bearer <token>").
type MetadataCarrier struct {
	md metadata.MD
}

// NewMetadataCarrier builds a carrier from key/value pairs.
func NewMetadataCarrier(pairs ...string) *MetadataCarrier {
	return &MetadataCarrier{md: metadata.Pairs(pairs...)}
}

// Bearer returns a carrier with the standard OAuth2 authorization header for
// the given token (pkg/auth.Token.AccessToken; TokenType "Bearer" per §4.1).
func Bearer(token string) *MetadataCarrier {
	return NewMetadataCarrier("authorization", "Bearer "+token)
}

// OutgoingContext attaches the carrier's metadata to ctx. Empty carriers
// return ctx unchanged.
func (c *MetadataCarrier) OutgoingContext(ctx context.Context) context.Context {
	if c == nil || len(c.md) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, c.md)
}

// reflectLookupMethod resolves a method descriptor via one server-reflection
// round-trip against the bound connection.
func reflectLookupMethod(ctx context.Context, cc grpc.ClientConnInterface, serviceName, methodName string) *desc.MethodDescriptor {
	if ctx == nil {
		ctx = context.Background()
	}
	rc := grpcreflect.NewClientAuto(ctx, cc)
	fd, err := rc.FileContainingSymbol(serviceName)
	if err != nil {
		return nil
	}
	for _, svc := range fd.GetServices() {
		if svc.GetFullyQualifiedName() != serviceName {
			continue
		}
		return svc.FindMethodByName(methodName)
	}
	return nil
}

// buildRequest converts JSON-style args into a dynamic protobuf message for
// the method's input type (dynamicproto: no generated stubs, no protoc).
// Client-side argument mismatch is CodeValidation BEFORE any bytes hit the
// wire (§4.2).
func buildRequest(input *desc.MessageDescriptor, opID string, args map[string]interface{}) (*dynamic.Message, error) {
	msg := dynamic.NewMessage(input)
	if len(args) > 0 {
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeValidation, "serialize arguments for "+opID)
		}
		if err := msg.UnmarshalJSON(raw); err != nil {
			return nil, errors.Wrap(err, errors.CodeValidation,
				"arguments do not match message "+input.GetFullyQualifiedName()+" for "+opID)
		}
	}
	return msg, nil
}

// decodeResult converts a response message into JSON-serializable data
// (§4.4: the same shape the HTTP path feeds to output rendering).
//
// *dynamic.Message (the grpcdynamic default) renders via its own canonical
// proto3 JSON mapping — protojson CANNOT be used on these: they carry no
// registered v2 type, so the protoadapt wrapper marshals to an empty object.
// Generated messages fall through to protojson.
func decodeResult(msg interface{}) (interface{}, error) {
	if msg == nil {
		return nil, nil
	}
	switch m := msg.(type) {
	case *dynamic.Message:
		raw, err := m.MarshalJSON()
		if err != nil {
			return nil, err
		}
		return jsonBytesToInterface(raw)
	case proto.Message:
		raw, err := protojson.Marshal(m)
		if err != nil {
			return nil, err
		}
		return jsonBytesToInterface(raw)
	default:
		return nil, errors.Errorf(errors.CodeInternal, "unsupported response message type %T", msg)
	}
}

// jsonBytesToInterface decodes JSON bytes into generic JSON-serializable data.
func jsonBytesToInterface(raw []byte) (interface{}, error) {
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
