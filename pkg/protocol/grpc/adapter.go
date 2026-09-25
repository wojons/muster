package grpc

import (
	"context"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// Adapter implements protocol.Adapter for gRPC: file/descriptor-set/reflection
// discovery plus unary execution (PROTO-002 scope).
type Adapter struct {
	// conn dials the reflection/invocation target.
	conn *grpc.ClientConn

	// files caches descriptors from the most recent file-based discovery so
	// Execute resolves input message types without a second parse.
	files []*desc.FileDescriptor

	// limiter applies the pkg/rate token bucket before dialing (§4.3).
	limiter *rateTokenBucket

	// carrier maps pkg/auth credentials into gRPC metadata (§4.1).
	carrier *MetadataCarrier
}

// NewAdapter creates a gRPC adapter.
func NewAdapter() *Adapter { return &Adapter{} }

// Protocol reports the protocol family (protocol.Adapter).
func (a *Adapter) Protocol() protocol.Protocol { return protocol.ProtocolGRPC }

// WithConn binds an existing connection (bufconn in tests, an
// operator-provided dial in the CLI layer). Reflection discovery reuses it.
func (a *Adapter) WithConn(cc *grpc.ClientConn) *Adapter {
	a.conn = cc
	return a
}

// WithRateLimiter attaches the shared token bucket (§4.3: non-HTTP adapters
// wrap Execute with TokenBucket.Allow before dial).
func (a *Adapter) WithRateLimiter(tb *rateTokenBucket) *Adapter {
	a.limiter = tb
	return a
}

// WithMetadata attaches a metadata carrier built from pkg/auth token values
// (Bearer(token) / NewMetadataCarrier(...)). Auth mapping is static per §4.1.
func (a *Adapter) WithMetadata(c *MetadataCarrier) *Adapter {
	a.carrier = c
	return a
}

// Close releases the adapter's connection, if it owns one.
func (a *Adapter) Close() error {
	if a.conn != nil {
		err := a.conn.Close()
		a.conn = nil
		return err
	}
	return nil
}

// Configure dials the invocation target when no connection is bound. Used by
// the CLI/MCP wiring after discovery names a host.
func (a *Adapter) Configure(ctx context.Context, target string, opts ...grpc.DialOption) error {
	if a.conn != nil {
		return nil
	}
	if target == "" {
		return errors.New(errors.CodeValidation, "gRPC target required: pass grpc://host:port or bind a connection")
	}
	dialOpts := opts
	if len(dialOpts) == 0 {
		dialOpts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	cc, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "dial gRPC target "+target)
	}
	a.conn = cc
	return nil
}

// Discover ingests the discovery source (protocol.Discoverer).
//
// URI forms:
//   - "grpc://host:port" / "reflection://host:port" — server reflection
//     against a live server (reuses the bound connection when present).
//   - "*.proto" — .proto source files (optionally "dir1:dir2=file.proto").
//   - anything else — compiled FileDescriptorSet file.
//
// Streaming methods are mapped (discovery lists them) but Execute rejects
// them: PROTO-003 boundary.
func (a *Adapter) Discover(ctx context.Context, uri string) ([]*protocol.Operation, error) {
	switch {
	case strings.HasPrefix(uri, "grpc://"), strings.HasPrefix(uri, "reflection://"):
		target := strings.TrimPrefix(strings.TrimPrefix(uri, "grpc://"), "reflection://")
		if err := a.Configure(ctx, target); err != nil {
			return nil, err
		}
		ops, err := discoverViaReflectionConn(ctx, a.conn)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			op.Source[SourceDescriptors] = "reflection"
			op.Source[SourceHost] = target
		}
		a.files = nil
		return ops, nil
	default:
		fds, source, err := loadDescriptors(uri)
		if err != nil {
			return nil, err
		}
		a.files = fds
		ops := MapFileDescriptors(fds)
		for _, op := range ops {
			op.Source[SourceDescriptors] = source
		}
		return ops, nil
	}
}

// Execute runs one operation (protocol.Executor). Per §4.3 the ctx deadline
// IS the deadline (gRPC applies it natively); the adapter never sets its own
// default timeout. Composition order: rate limit → auth metadata →
// deadline → single attempt.
func (a *Adapter) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if a.conn == nil {
		return nil, errors.New(errors.CodeValidation, "gRPC adapter has no connection: Discover against grpc://host or call Configure first")
	}
	if opID == "" {
		return nil, errors.New(errors.CodeValidation, "empty operation ID")
	}

	// §4.3 composition order, step 1: rate limit (client-side, local).
	if a.limiter != nil && !a.limiter.Allow() {
		return nil, errors.New(errors.CodeRateLimit, "gRPC call rate-limited by local token bucket")
	}

	// Resolve the method descriptor (cache → reflection).
	serviceName, methodName, ok := splitOpID(opID)
	if !ok {
		return nil, errors.Errorf(errors.CodeValidation,
			"operation ID %q is not in service-method form (e.g. kv.KeyValue-Get)", opID)
	}
	method, err := a.resolveMethod(ctx, serviceName, methodName)
	if err != nil {
		return nil, err
	}

	// PROTO-003 boundary: reject streaming at the transport boundary.
	if method.IsServerStreaming() || method.IsClientStreaming() {
		return nil, streamingRejection(kindOfMethod(method))
	}

	reqMsg, err := buildRequest(method.GetInputType(), opID, args)
	if err != nil {
		return nil, err
	}

	// §4.3 step 2+3: auth metadata under the caller's deadline. gRPC applies
	// the ctx deadline natively; no adapter-default timeout exists here.
	callCtx := a.carrier.OutgoingContext(ctx)

	fullMethod := "/" + serviceName + "/" + methodName
	stub := grpcdynamic.NewStub(a.conn)
	resp, rpcErr := stub.InvokeRpc(callCtx, method, reqMsg)
	if rpcErr != nil {
		return nil, mapTransportError(rpcErr, fullMethod)
	}
	out, decErr := decodeResult(resp)
	if decErr != nil {
		return nil, mapDecodeError(decErr, fullMethod)
	}
	return out, nil
}

// resolveMethod finds a method descriptor: first in the cached file
// descriptors (file-based discovery), then via one reflection round-trip
// against the bound connection.
func (a *Adapter) resolveMethod(ctx context.Context, serviceName, methodName string) (*desc.MethodDescriptor, error) {
	if m := lookupMethod(a.files, serviceName, methodName); m != nil {
		return m, nil
	}
	if m := reflectLookupMethod(ctx, a.conn, serviceName, methodName); m != nil {
		return m, nil
	}
	return nil, errors.Errorf(errors.CodeNotFound,
		"gRPC method %q not found on service %q: discover first (file, descriptor set, or reflection)", methodName, serviceName)
}

// kindOfMethod classifies a method descriptor into the neutral OperationKind.
func kindOfMethod(m *desc.MethodDescriptor) protocol.OperationKind {
	switch {
	case m.IsClientStreaming() && m.IsServerStreaming():
		return protocol.KindBidiStream
	case m.IsClientStreaming():
		return protocol.KindClientStream
	case m.IsServerStreaming():
		return protocol.KindServerStream
	default:
		return protocol.KindUnary
	}
}

// splitOpID splits "kv.KeyValue-Get" into ("kv.KeyValue", "Get"). Service
// names contain dots but no dashes; method names are proto identifiers (no
// dashes), so the LAST dash is the boundary.
func splitOpID(opID string) (service, method string, ok bool) {
	idx := strings.LastIndex(opID, "-")
	if idx <= 0 || idx == len(opID)-1 {
		return "", "", false
	}
	return opID[:idx], opID[idx+1:], true
}
