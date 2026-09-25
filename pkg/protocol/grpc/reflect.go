package grpc

import (
	"context"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// DiscoverViaReflection queries a live gRPC server's reflection service and
// maps every exposed service to operations. The reflection client returns
// linked *desc.FileDescriptors, so the neutral mapping is IDENTICAL to the
// file-ingestion path (MapFileDescriptor); only the Source stamps differ.
//
// Only used against operator-specified targets (§4.6: tests run reflection
// over bufconn). opts override the default insecure credentials (tests pass
// bufconn's dialer).
func DiscoverViaReflection(ctx context.Context, target string, opts ...grpc.DialOption) ([]*protocol.Operation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dialOpts := opts
	if len(dialOpts) == 0 {
		dialOpts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	cc, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "dial reflection target "+target)
	}
	defer func() { _ = cc.Close() }()

	rc := grpcreflect.NewClientAuto(ctx, cc)
	svcNames, err := rc.ListServices()
	if err != nil {
		return nil, errors.Wrap(err, mapCode(err), "list services via reflection from "+target)
	}

	var ops []*protocol.Operation
	for _, name := range svcNames {
		// Skip the reflection service itself — it is transport plumbing,
		// not an operation surface the operator wants commands for.
		if name == "grpc.reflection.v1.ServerReflection" ||
			name == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		fd, err := rc.FileContainingSymbol(name)
		if err != nil {
			return nil, errors.Wrap(err, mapCode(err), "fetch descriptor for service "+name+" via reflection")
		}
		var service *desc.ServiceDescriptor
		for _, s := range fd.GetServices() {
			if s.GetFullyQualifiedName() == name {
				service = s
				break
			}
		}
		if service == nil {
			continue
		}
		for _, op := range MapServiceDescriptor(service) {
			op.Source[SourceDescriptors] = "reflection"
			op.Source[SourceHost] = target
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// discoverViaReflectionConn runs reflection discovery over an existing
// connection (used by Adapter.Discover when the executor already dials the
// target — one dial, both discovery and invocation).
func discoverViaReflectionConn(ctx context.Context, cc grpc.ClientConnInterface) ([]*protocol.Operation, error) {
	rc := grpcreflect.NewClientAuto(ctx, cc)
	svcNames, err := rc.ListServices()
	if err != nil {
		return nil, errors.Wrap(err, mapCode(err), "list services via reflection")
	}

	var ops []*protocol.Operation
	for _, name := range svcNames {
		if name == "grpc.reflection.v1.ServerReflection" ||
			name == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		fd, err := rc.FileContainingSymbol(name)
		if err != nil {
			return nil, errors.Wrap(err, mapCode(err), "fetch descriptor for service "+name+" via reflection")
		}
		var service *desc.ServiceDescriptor
		for _, s := range fd.GetServices() {
			if s.GetFullyQualifiedName() == name {
				service = s
				break
			}
		}
		if service == nil {
			continue
		}
		for _, op := range MapServiceDescriptor(service) {
			op.Source[SourceDescriptors] = "reflection"
			ops = append(ops, op)
		}
	}
	return ops, nil
}
