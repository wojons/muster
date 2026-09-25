package grpc

import (
	"context"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// statusToCode maps a gRPC status code to the AppError code table in
// specs/PROTOCOLS.md §4.2. Table-driven; one mapping function per adapter,
// applied at the transport boundary only.
func statusToCode(c codes.Code) errors.Code {
	switch c {
	case codes.DeadlineExceeded, codes.Canceled:
		// Canceled covers the caller aborting the deadline (ctx cancel);
		// §4.3 folds both into the caller-controlled deadline story.
		return errors.CodeTimeout
	case codes.ResourceExhausted:
		return errors.CodeRateLimit
	case codes.Unauthenticated:
		return errors.CodeUnauthorized
	case codes.PermissionDenied:
		// §4.2: distinct from unauthenticated; same code, different detail.
		return errors.CodeUnauthorized
	case codes.InvalidArgument, codes.OutOfRange, codes.FailedPrecondition:
		return errors.CodeBadRequest
	case codes.NotFound:
		return errors.CodeNotFound
	case codes.Unimplemented:
		return errors.CodeNotFound
	default:
		// Internal, DataLoss, Unknown, Unavailable, Aborted, Canceled, OK,
		// and anything unlisted fall to the generic default per §4.2.
		return errors.CodeGeneric
	}
}

// mapCode maps any error to its AppError code. Non-status errors (dial
// failures, decode errors) are CodeInternal per §4.2's protocol-internal row;
// raw context deadline/cancellation maps to CodeTimeout.
func mapCode(err error) errors.Code {
	if err == nil {
		return errors.CodeGeneric
	}
	if _, ok := status.FromError(err); ok {
		return statusToCode(status.Code(err))
	}
	if err == context.DeadlineExceeded || err == context.Canceled {
		return errors.CodeTimeout
	}
	return errors.CodeInternal
}

// mapTransportError wraps a transport error as an AppError with the §4.2
// code table applied, preserving the cause chain for --verbose output and
// errors.IsError checks. Server retry hints (ResourceExhausted) are kept in
// the detail map.
func mapTransportError(err error, fullMethod string) *errors.AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(*errors.AppError); ok {
		// Already mapped deeper in our own stack: pass through unchanged.
		return appErr
	}
	code := mapCode(err)
	wrapped := errors.Wrap(err, code, "gRPC call "+fullMethod+" failed")
	if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
		wrapped.WithDetail("grpc_code", st.Code().String())
		switch st.Code() {
		case codes.PermissionDenied:
			wrapped.WithDetail("reason", "permission denied (distinct from unauthenticated)")
		case codes.ResourceExhausted:
			wrapped.WithDetail("server_message", st.Message())
		}
	}
	return wrapped
}

// mapLocalError wraps client-side (pre-wire) failures: validation before any
// bytes hit the wire is CodeValidation per §4.2.
func mapLocalError(err error, what string) *errors.AppError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeValidation, what)
}

// mapDecodeError wraps response decoding failures as CodeInternal (§4.2:
// "Protocol-internal decode/serialization failures").
func mapDecodeError(err error, fullMethod string) *errors.AppError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeInternal, "decode response from "+fullMethod)
}

// streamingRejection builds the PROTO-003 boundary error for a streaming op.
func streamingRejection(kind protocol.OperationKind) *errors.AppError {
	return errors.Errorf(errors.CodeValidation,
		"streaming methods are out of scope for the gRPC unary gate (PROTO-003): kind %d operations are not executable yet", int(kind))
}

// lookupMethod finds a method descriptor across linked files.
func lookupMethod(files []*desc.FileDescriptor, serviceName, methodName string) *desc.MethodDescriptor {
	for _, fd := range files {
		for _, svc := range fd.GetServices() {
			if svc.GetFullyQualifiedName() != serviceName {
				continue
			}
			if m := svc.FindMethodByName(methodName); m != nil {
				return m
			}
		}
	}
	return nil
}
