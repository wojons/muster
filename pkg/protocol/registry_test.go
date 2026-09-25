package protocol

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// stubAdapter is a minimal Adapter for registry tests.
type stubAdapter struct {
	proto Protocol
}

func (a *stubAdapter) Protocol() Protocol { return a.proto }

func (a *stubAdapter) Discover(ctx context.Context, uri string) ([]*Operation, error) {
	return nil, nil
}

func (a *stubAdapter) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	return nil, nil
}

// compile-time pin: stubAdapter satisfies the seam.
var _ Adapter = (*stubAdapter)(nil)

func TestRegistry_Register_And_Adapter(t *testing.T) {
	r := NewRegistry()
	a := &stubAdapter{proto: ProtocolJSONRPC}

	require.NoError(t, r.Register(a))

	got, err := r.Adapter(ProtocolJSONRPC)
	require.NoError(t, err)
	assert.Same(t, a, got)
}

func TestRegistry_Register_DuplicateProtocol_Fails(t *testing.T) {
	r := NewRegistry()
	first := &stubAdapter{proto: ProtocolGRPC}
	second := &stubAdapter{proto: ProtocolGRPC}

	require.NoError(t, r.Register(first))
	err := r.Register(second)
	require.Error(t, err)
	assert.True(t, apperrors.IsError(err, apperrors.CodeValidation), "duplicate registration must be a validation error")
}

func TestRegistry_Adapter_UnknownProtocol_ReturnsNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Adapter(ProtocolSOAP)
	require.Error(t, err)
	assert.True(t, apperrors.IsError(err, apperrors.CodeNotFound), "unknown protocol must surface CodeNotFound per the seam contract")
}

func TestRegistry_Register_NilAdapter_Fails(t *testing.T) {
	r := NewRegistry()

	err := r.Register(nil)
	require.Error(t, err)
}

// seamSignatureTest pins the frozen interface signatures from
// specs/PROTOCOLS.md §1.3 at compile time: if this file's method shapes stop
// satisfying Adapter, the seam has drifted from the spec.
func TestSeam_Signatures_Frozen(t *testing.T) {
	var _ func(*stubAdapter, context.Context, string) ([]*Operation, error) = (*stubAdapter).Discover
	var _ func(*stubAdapter, context.Context, string, map[string]interface{}) (interface{}, error) = (*stubAdapter).Execute
	var _ func(*stubAdapter) Protocol = (*stubAdapter).Protocol
	var _ Discoverer = (*stubAdapter)(nil)
	var _ Executor = (*stubAdapter)(nil)
	var _ Adapter = (*stubAdapter)(nil)
	var _ OperationKind = KindUnary
	var _ OperationKind = KindServerStream
	var _ OperationKind = KindClientStream
	var _ OperationKind = KindBidiStream
	var _ OperationKind = KindEventSource
	var _ Protocol = ProtocolJSONRPC
	var _ Protocol = ProtocolGRPC
}
