// Package jsonrpc_test provides the PROTO-004 acceptance battery:
// discovery from OpenRPC documents (file + HTTP), unary execution over an
// in-process httptest JSON-RPC server, JSON-RPC error → AppError mapping,
// the timeout path, batch rejection, and wire-byte conformance
// (specs/PROTOCOLS.md §4.6). All tests are network-free (loopback only).
package jsonrpc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/jsonrpc"
)

// ── in-process JSON-RPC 2.0 test server ─────────────────────────────────

// recordingServer is an httptest JSON-RPC 2.0 server: an "echo" method that
// returns its params, a "fail" method returning a JSON-RPC error object, and
// a "slow" method used for the timeout path. It records the raw request
// bodies so the wire conformance test can golden-compare them.
type recordingServer struct {
	*httptest.Server

	mu        sync.Mutex
	lastBody  []byte
	lastHdr   http.Header
	failCode  int
	failMsg   string
	sendArray bool // when true, responds with a batch (array) envelope
}

func newRecordingServer(t *testing.T, mutate func(*recordingServer)) *recordingServer {
	t.Helper()
	s := &recordingServer{failCode: -32603, failMsg: "boom"}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		s.mu.Lock()
		s.lastBody = body
		s.lastHdr = r.Header.Clone()
		sendArray := s.sendArray
		failCode, failMsg := s.failCode, s.failMsg
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		var req mcp.JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}`))
			return
		}

		// Batch request (array): not implemented server-side either; answer
		// with an invalid-request error object for any batch.
		if strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batch requests not supported by test server"}}`))
			return
		}

		resp := mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "echo":
			resp.Result = mustMarshalJSON(req.Params)
		case "fail":
			resp.Error = &mcp.JSONRPCError{Code: failCode, Message: failMsg}
		case "slow":
			time.Sleep(300 * time.Millisecond)
			resp.Result = mustMarshalJSON(map[string]string{"woke": "up"})
		case "string_id":
			resp.Result = mustMarshalJSON(map[string]string{"ok": "yes"})
		default:
			resp.Error = &mcp.JSONRPCError{Code: -32601, Message: "Method not found"}
		}

		out, _ := json.Marshal(resp)
		if sendArray {
			out = []byte("[" + string(out) + "]")
		}
		_, _ = w.Write(out)
	}))
	t.Cleanup(s.Close)
	if mutate != nil {
		mutate(s)
	}
	return s
}

// lastRequest returns the most recent request body and headers.
func (s *recordingServer) lastRequest() ([]byte, http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastBody, s.lastHdr
}

func mustMarshalJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
}

// sampleOpenRPCDoc is a minimal OpenRPC 1.x document with two methods.
const sampleOpenRPCDoc = `{
  "openrpc": "1.2.6",
  "info": {"title": "Calculator", "version": "1.0.0"},
  "servers": [{"name": "local", "url": "http://127.0.0.1:1/rpc"}],
  "methods": [
    {
      "name": "add",
      "summary": "Add two numbers",
      "description": "Adds two integers and returns the sum.",
      "params": [
        {"name": "a", "required": true, "schema": {"type": "integer"}, "description": "first addend"},
        {"name": "b", "required": true, "schema": {"type": "integer"}}
      ],
      "result": {"name": "sum", "schema": {"type": "integer"}, "description": "the sum"}
    },
    {
      "name": "ping",
      "summary": "Ping the node",
      "tags": [{"name": "system"}]
    }
  ]
}`

// writeTempDoc writes an OpenRPC document to a temp file and returns its path.
func writeTempDoc(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openrpc.json")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))
	return path
}

// newClientFor builds a jsonrpc.Client bound to the endpoint with a fixed
// request ID (deterministic wire bytes) and a small fixed timeout.
func newClientFor(endpoint string) *jsonrpc.Client {
	return jsonrpc.NewClient(endpoint,
		jsonrpc.WithIDSource(func() interface{} { return uint64(42) }),
	)
}

// ── protocol.Adapter conformance ─────────────────────────────────────────

func TestClient_ImplementsProtocolAdapter(t *testing.T) {
	var _ protocol.Adapter = jsonrpc.NewClient("http://127.0.0.1:1")
	var _ protocol.Discoverer = jsonrpc.NewClient("http://127.0.0.1:1")
	var _ protocol.Executor = jsonrpc.NewClient("http://127.0.0.1:1")
}

func TestClient_ProtocolName(t *testing.T) {
	c := jsonrpc.NewClient("http://127.0.0.1:1")
	assert.Equal(t, protocol.ProtocolJSONRPC, c.Protocol())
}

// ── discovery: file + HTTP ────────────────────────────────────────────────

func TestDiscover_FromFile_MapsOperations(t *testing.T) {
	c := newClientFor("http://127.0.0.1:1")

	ops, err := c.Discover(context.Background(), writeTempDoc(t, sampleOpenRPCDoc))
	require.NoError(t, err)
	require.Len(t, ops, 2)

	byID := map[string]*protocol.Operation{}
	for _, op := range ops {
		byID[op.ID] = op
	}

	add := byID["add"]
	require.NotNil(t, add, "method add must map to operation add")
	assert.Equal(t, protocol.ProtocolJSONRPC, add.Protocol)
	assert.Equal(t, protocol.KindUnary, add.Kind)
	assert.Equal(t, "Add two numbers", add.Summary)
	assert.Equal(t, "Adds two integers and returns the sum.", add.Description)
	require.Len(t, add.Parameters, 2)
	assert.Equal(t, "a", add.Parameters[0].Name)
	assert.Equal(t, "integer", add.Parameters[0].Type)
	assert.True(t, add.Parameters[0].Required)
	assert.Equal(t, "payload", add.Parameters[0].In)
	assert.Equal(t, "first addend", add.Parameters[0].Description)
	assert.Equal(t, []string{"full_method", "server"}, sortedSourceKeys(add.Source))
	assert.Equal(t, "http://127.0.0.1:1/rpc", add.Source["server"], "servers[0].url must be stamped into Source for the executor")
	assert.Equal(t, "add", add.Source["full_method"])

	ping := byID["ping"]
	require.NotNil(t, ping)
	assert.Equal(t, []string{"system"}, ping.Tags)
	assert.Equal(t, "Ping the node", ping.Summary)
	assert.Empty(t, ping.Parameters)
}

// sortedSourceKeys returns the Source map keys, sorted.
func sortedSourceKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func TestDiscover_FromHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleOpenRPCDoc))
	}))
	t.Cleanup(srv.Close)

	c := newClientFor("http://127.0.0.1:1")
	ops, err := c.Discover(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, "add", ops[0].ID)
}

func TestDiscover_RejectsNonOpenRPCDocument(t *testing.T) {
	c := newClientFor("http://127.0.0.1:1")

	_, err := c.Discover(context.Background(), writeTempDoc(t, `{"swagger": "2.0"}`))
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
	assert.Contains(t, err.Error(), "openrpc")
}

func TestDiscover_RejectsDuplicateMethodNames(t *testing.T) {
	dupDoc := `{
	  "openrpc": "1.2.6",
	  "methods": [
	    {"name": "add"},
	    {"name": "ADD"}
	  ]
	}`
	c := newClientFor("http://127.0.0.1:1")

	_, err := c.Discover(context.Background(), writeTempDoc(t, dupDoc))
	require.Error(t, err, "add/ADD normalize to the same ID and must be rejected (unique IDs per seam contract)")
}

func TestDiscover_MissingSource_ReturnsNotFound(t *testing.T) {
	c := newClientFor("http://127.0.0.1:1")

	_, err := c.Discover(context.Background(), "/nonexistent/path/openrpc.json")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeNotFound))
}

func TestDiscover_EmptyURI_Validation(t *testing.T) {
	c := newClientFor("http://127.0.0.1:1")

	_, err := c.Discover(context.Background(), "  ")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
}

// ── execution: unary round-trip over httptest ─────────────────────────────

func TestExecute_Echo_RoundTrip(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := newClientFor(srv.URL)

	result, err := c.Execute(context.Background(), "echo", map[string]interface{}{"msg": "hello", "n": float64(7)})
	require.NoError(t, err)

	m, ok := result.(map[string]interface{})
	require.True(t, ok, "result must decode as a JSON object")
	assert.Equal(t, "hello", m["msg"])
	assert.Equal(t, float64(7), m["n"])

	// Wire conformance: the request the server saw must be a spec-shaped
	// JSON-RPC 2.0 request object.
	body, hdr := srv.lastRequest()
	assert.Equal(t, "application/json", hdr.Get("Content-Type"))
	var wire mcp.JSONRPCRequest
	require.NoError(t, json.Unmarshal(body, &wire))
	assert.Equal(t, "2.0", wire.JSONRPC)
	assert.Equal(t, "echo", wire.Method)
	assert.Equal(t, float64(42), wire.ID, "WithIDSource-fixed ID must ride the wire")
	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(wire.Params, &sent))
	assert.Equal(t, "hello", sent["msg"])
}

func TestExecute_MethodNotFound_MapsToAppError(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "nope", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeNotFound), "-32601 must map to CodeNotFound")

	var appErr *errors.AppError
	require.True(t, stdErrorsAs(err, &appErr), "error must be an *errors.AppError")
	assert.Equal(t, errors.CodeNotFound, appErr.Code())
	details := appErr.Details()
	assert.Equal(t, -32601, details["rpc_code"], "rpc error code preserved in details")
	assert.Equal(t, "jsonrpc", details["protocol"])
}

func TestExecute_ServerError_MapsToCodeGeneric(t *testing.T) {
	srv := newRecordingServer(t, func(s *recordingServer) {
		s.failCode = -32000
		s.failMsg = "node is syncing"
	})
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "fail", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeGeneric), "server-range errors map to CodeGeneric")
}

func TestExecute_InvalidParams_MapsToValidation(t *testing.T) {
	srv := newRecordingServer(t, func(s *recordingServer) {
		s.failCode = -32602
		s.failMsg = "missing a"
	})
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "fail", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "-32602 must map to CodeValidation")
}

func TestExecute_ParseError_MapsToBadRequest(t *testing.T) {
	srv := newRecordingServer(t, func(s *recordingServer) {
		s.failCode = -32700
		s.failMsg = "parse error"
	})
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "fail", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeBadRequest), "-32700 must map to CodeBadRequest")
}

func TestExecute_InvalidRequest_MapsToBadRequest(t *testing.T) {
	srv := newRecordingServer(t, func(s *recordingServer) {
		s.failCode = -32600
		s.failMsg = "invalid request"
	})
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "fail", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeBadRequest), "-32600 must map to CodeBadRequest")
}

func TestExecute_InternalError_MapsToCodeInternal(t *testing.T) {
	srv := newRecordingServer(t, func(s *recordingServer) {
		s.failCode = -32603
		s.failMsg = "boom"
	})
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "fail", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeInternal), "-32603 must map to CodeInternal")
}

// ── ID handling ───────────────────────────────────────────────────────────

func TestExecute_StringIDRoundTrip(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := jsonrpc.NewClient(srv.URL, jsonrpc.WithIDSource(func() interface{} { return "req-1" }))

	result, err := c.Execute(context.Background(), "string_id", nil)
	require.NoError(t, err)
	m := result.(map[string]interface{})
	assert.Equal(t, "yes", m["ok"])
}

func TestExecute_IDMismatchRejected(t *testing.T) {
	// A server that echoes a different ID must be rejected (correlation).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req mcp.JSONRPCRequest
		_ = json.Unmarshal(body, &req)
		resp := mcp.JSONRPCResponse{JSONRPC: "2.0", ID: "different-id", Result: json.RawMessage(`{}`)}
		out, _ := json.Marshal(resp)
		_, _ = w.Write(out)
	}))
	t.Cleanup(srv.Close)

	c := jsonrpc.NewClient(srv.URL, jsonrpc.WithIDSource(func() interface{} { return uint64(42) }))
	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ID")
}

// ── timeout path ──────────────────────────────────────────────────────────

func TestExecute_ContextDeadline_MapsToTimeout(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := newClientFor(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Execute(ctx, "slow", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeTimeout), "context deadline must surface as CodeTimeout per §4.2")
}

func TestExecute_AdapterTimeout_MapsToTimeout(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := jsonrpc.NewClient(srv.URL, jsonrpc.WithTimeout(50*time.Millisecond))

	_, err := c.Execute(context.Background(), "slow", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeTimeout), "adapter timeout must surface as CodeTimeout")
}

func TestExecute_ZeroAdapterTimeout_DisablesBound(t *testing.T) {
	// WithTimeout(0) disables the adapter bound entirely: the slow method
	// (~300ms) completes instead of being cut off by any default.
	srv := newRecordingServer(t, nil)
	c := jsonrpc.NewClient(srv.URL, jsonrpc.WithTimeout(0))

	result, err := c.Execute(context.Background(), "slow", nil)
	require.NoError(t, err, "a zero adapter timeout must not bound the call")
	m, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "up", m["woke"])
}

// ── transport status mapping ──────────────────────────────────────────────

func TestExecute_HTTP429_MapsToRateLimitWithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := newClientFor(srv.URL)
	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeRateLimit))

	var appErr *errors.AppError
	require.True(t, stdErrorsAs(err, &appErr))
	assert.Equal(t, "7", appErr.Details()["retry_after"], "Retry-After must be preserved per §4.2")
}

func TestExecute_HTTP401_MapsToUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := newClientFor(srv.URL)
	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeUnauthorized))
}

func TestExecute_HTTP403_MapsToUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	c := newClientFor(srv.URL)
	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeUnauthorized), "403 permission failure maps to CodeUnauthorized per §4.2")
}

func TestExecute_HTTP500_MapsToBadRequestBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := newClientFor(srv.URL)
	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeBadRequest))
}

func TestExecute_EmptyOpID_Validation(t *testing.T) {
	c := newClientFor("http://127.0.0.1:1")
	_, err := c.Execute(context.Background(), "  ", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
}

// ── batch semantics (optional — rejected with a documented error) ─────────

func TestExecute_BatchResponseRejected(t *testing.T) {
	srv := newRecordingServer(t, func(s *recordingServer) { s.sendArray = true })
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)

	var batchErr *jsonrpc.BatchNotImplementedError
	require.True(t, stdErrorsAs(err, &batchErr), "batch response must surface BatchNotImplementedError, got: %v", err)
	assert.Equal(t, "response", batchErr.Direction)
}

// ── rate limiting composition (§4.3) ──────────────────────────────────────

type failingLimiter struct{}

func (failingLimiter) Allow() bool { return false }

func TestExecute_RateLimitRefusal_Local(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := jsonrpc.NewClient(srv.URL, jsonrpc.WithRateLimiter(failingLimiter{}))

	_, err := c.Execute(context.Background(), "echo", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeRateLimit))

	// The server must never have seen a request.
	body, _ := srv.lastRequest()
	assert.Nil(t, body, "rate-limited call must not hit the wire")
}

// ── wire-bytes conformance (golden, §4.6) ─────────────────────────────────

func TestConformance_WireBytes_Golden(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := newClientFor(srv.URL)

	_, err := c.Execute(context.Background(), "echo", map[string]interface{}{"a": float64(1)})
	require.NoError(t, err)

	body, _ := srv.lastRequest()
	golden := `{"jsonrpc":"2.0","id":42,"method":"echo","params":{"a":1}}`
	assert.JSONEq(t, golden, string(body), "wire bytes must match the golden JSON-RPC 2.0 request")
}

// ── source-stamp executor endpoint (executor ground truth) ───────────────

func TestDiscoverThenExecute_EndToEnd(t *testing.T) {
	srv := newRecordingServer(t, nil)

	// Discovery document pointing at the JSON-RPC server via servers[0].
	e2eDoc := fmt.Sprintf(`{
	  "openrpc": "1.2.6",
	  "servers": [{"name": "test", "url": %q}],
	  "methods": [
	    {"name": "echo", "params": [{"name": "msg", "schema": {"type": "string"}}]}
	  ]
	}`, srv.URL)
	docSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(e2eDoc))
	}))
	t.Cleanup(docSrv.Close)

	// Stage 1: discover (the discoverer's own endpoint is irrelevant; the
	// document names the RPC endpoint).
	discoverer := newClientFor("http://127.0.0.1:1")
	ops, err := discoverer.Discover(context.Background(), docSrv.URL)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "echo", ops[0].ID)
	assert.Equal(t, srv.URL, ops[0].Source["server"], "discovery must stamp the document's server URL")

	// Stage 4: build the executor from the operation's Source stamp — the
	// documented pattern (adapters stamp Source; only the Executor reads it).
	endpoint, ok := ops[0].Source["server"]
	require.True(t, ok)
	executor := newClientFor(endpoint)

	result, err := executor.Execute(context.Background(), ops[0].ID, map[string]interface{}{"msg": "hello"})
	require.NoError(t, err)
	m, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "hello", m["msg"])
}

// ── std errors.As helper (avoids importing std errors over pkg/errors) ────

func stdErrorsAs(err error, target interface{}) bool {
	for err != nil {
		if e, ok := err.(interface{ As(interface{}) bool }); ok {
			_ = e
		}
		if asErr, ok := err.(*jsonrpc.BatchNotImplementedError); ok {
			if be, ok2 := target.(**jsonrpc.BatchNotImplementedError); ok2 {
				*be = asErr
				return true
			}
		}
		if ae, ok := err.(*errors.AppError); ok {
			if ap, ok2 := target.(**errors.AppError); ok2 {
				*ap = ae
				return true
			}
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// silence unused-import guards for fmt in constrained build configs
var _ = fmt.Sprintf
