package protocol

import (
	"context"
	"testing"

	"github.com/wojons/muster/pkg/errors"
)

// fakeAdapter is a minimal Adapter for registry tests.
type fakeAdapter struct {
	proto Protocol
}

func (f *fakeAdapter) Protocol() Protocol { return f.proto }
func (f *fakeAdapter) Discover(_ context.Context, _ string) ([]*Operation, error) {
	return nil, nil
}
func (f *fakeAdapter) Execute(_ context.Context, _ string, _ map[string]interface{}) (interface{}, error) {
	return nil, nil
}

// TestRegistry_Register_Lookup proves the §1.3 registry contract.
func TestRegistry_Register_Lookup(t *testing.T) {
	r := NewRegistry()
	a := &fakeAdapter{proto: ProtocolGRPC}
	if err := r.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := r.Adapter(ProtocolGRPC)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Protocol() != ProtocolGRPC {
		t.Fatal("lookup returned the wrong adapter")
	}
}

// TestRegistry_DuplicateProtocol: duplicate Protocol → error (§1.3).
func TestRegistry_DuplicateProtocol(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeAdapter{proto: ProtocolGRPC}); err != nil {
		t.Fatal(err)
	}
	err := r.Register(&fakeAdapter{proto: ProtocolGRPC})
	if err == nil {
		t.Fatal("duplicate registration must error")
	}
	if !errors.IsError(err, errors.CodeValidation) {
		t.Fatalf("duplicate error = %v, want CodeValidation", err)
	}
}

// TestRegistry_UnknownProtocol: unknown → AppError CodeNotFound (§1.3).
func TestRegistry_UnknownProtocol(t *testing.T) {
	r := NewRegistry()
	_, err := r.Adapter(ProtocolSOAP)
	if !errors.IsError(err, errors.CodeNotFound) {
		t.Fatalf("unknown protocol error = %v, want CodeNotFound", err)
	}
}

// TestRegistry_NilAdapter: nil registration is rejected.
func TestRegistry_NilAdapter(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatal("nil adapter must not register")
	}
}

// TestOperationKinds_Exhaustive pins the iota order the safety rules key on.
func TestOperationKinds_Exhaustive(t *testing.T) {
	if KindUnary != 0 || KindServerStream != 1 || KindClientStream != 2 || KindBidiStream != 3 || KindEventSource != 4 {
		t.Fatal("OperationKind iota order drifted from specs/PROTOCOLS.md §1.3")
	}
	if ProtocolGRPC != "grpc" || ProtocolOpenAPI != "openapi" {
		t.Fatal("Protocol constants drifted from specs/PROTOCOLS.md §1.3")
	}
}
