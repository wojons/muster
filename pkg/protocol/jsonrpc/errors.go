package jsonrpc

import (
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/protocol"
)

// JSON-RPC 2.0 reserved error codes.
const (
	parseErrorCode     = -32700
	invalidRequestCode = -32600
	methodNotFoundCode = -32601
	invalidParamsCode  = -32602
	internalErrorCode  = -32603
	// Server-range errors are -32000 to -32099 (implementation-defined).
	serverErrorCode = -32000
)

// mapRPCError maps a JSON-RPC 2.0 error object onto pkg/errors.AppError
// codes per specs/PROTOCOLS.md §4.2 (one table-driven mapping function at
// the transport boundary, wrapping the original error so errors.IsError
// works and the cause chain is preserved for --verbose output).
//
//	-32601 method not found        → CodeNotFound
//	-32602 invalid params          → CodeValidation
//	-32600 invalid request,
//	-32700 parse error             → CodeBadRequest
//	-32603 internal error          → CodeInternal
//	≤ -32000 (server range)        → CodeGeneric
func mapRPCError(rpcErr *mcp.JSONRPCError) error {
	if rpcErr == nil {
		return nil
	}
	cause := fmt.Errorf("jsonrpc error %d: %s", rpcErr.Code, rpcErr.Message)

	var code errors.Code
	switch rpcErr.Code {
	case methodNotFoundCode:
		code = errors.CodeNotFound
	case invalidParamsCode:
		code = errors.CodeValidation
	case invalidRequestCode, parseErrorCode:
		code = errors.CodeBadRequest
	case internalErrorCode:
		code = errors.CodeInternal
	default:
		// Server-range (-32000..-32099) and any implementation-defined code.
		code = errors.CodeGeneric
	}

	appErr := errors.Wrap(cause, code, fmt.Sprintf("jsonrpc: remote call failed (%s)", rpcErr.Message))
	return appErr.WithDetail("rpc_code", rpcErr.Code).WithDetail("protocol", string(protocol.ProtocolJSONRPC))
}

// sourceKeyServer is the Operation.Source key carrying the endpoint URL an
// operation was discovered against (adapters stamp Source; stage-3
// consumers never read it — the Executor does, per specs/PROTOCOLS.md §3.2).
const sourceKeyServer = "server"

// endpointFromSource extracts the endpoint URL stamped on an operation, if any.
func endpointFromSource(op *protocol.Operation) (string, bool) {
	if op == nil || op.Source == nil {
		return "", false
	}
	ep, ok := op.Source[sourceKeyServer]
	return ep, ok && ep != ""
}

// normalizeID normalizes an OpenRPC method name into a stable operation ID
// that survives pkg/generator's command-name normalization (specs/PROTOCOLS.md
// §3.1): trimmed, lower-cased, dots become underscores.
func normalizeID(name string) string {
	return stringsReplaceAll(strings.ToLower(strings.TrimSpace(name)), ".", "_")
}

// uniqueIDs verifies operation IDs are unique within a discovery document
// (specs/PROTOCOLS.md §3.1: IDs MUST be unique per adapter).
func uniqueIDs(ops []*protocol.Operation) error {
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		if seen[op.ID] {
			return errors.New(errors.CodeValidation, fmt.Sprintf("jsonrpc: duplicate operation ID %q in discovery document", op.ID))
		}
		seen[op.ID] = true
	}
	return nil
}
