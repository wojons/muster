// Package jsonrpc implements the JSON-RPC 2.0 / OpenRPC protocol adapter
// for Muster's protocol adapter seam (specs/PROTOCOLS.md §1, board row
// PROTO-004).
//
// Discovery (stage 1) parses an OpenRPC 1.x discovery document from a local
// file or an http(s):// URL and maps each OpenRPC method onto the
// protocol-neutral Operation model. Execution (stage 4) sends JSON-RPC 2.0
// requests over HTTP POST, reusing the existing codec shapes from pkg/mcp
// (JSONRPCRequest / JSONRPCResponse / JSONRPCError) — this package does not
// fork a second codec. Transport assembly goes through pkg/request.Builder
// and pkg/client so tracing/retry/rate-limit wiring matches the HTTP path.
//
// Batch semantics: JSON-RPC 2.0 batches are OPTIONAL, and this adapter does
// not implement them. Every call sends exactly one request object and
// expects exactly one response object; a batch (array) response is rejected
// with a BatchNotImplementedError. The choice is deliberate: the adapter's
// operations are unary command-shaped calls, and the transport composition
// (auth, retry, rate limit) is defined per call, not per batch.
//
// Error mapping follows the spec's table (specs/PROTOCOLS.md §4.2):
// JSON-RPC error objects map to pkg/errors.AppError codes by their error
// code, transport timeouts map to CodeTimeout, and client-side validation
// failures map to CodeValidation.
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/request"
)

// rpcSpecVersion is the value required in every JSON-RPC 2.0 request's
// "jsonrpc" member (reuses pkg/mcp's codec; kept as a package constant so
// the executor does not hard-code magic strings).
const rpcSpecVersion = "2.0"

// BatchNotImplementedError is returned when a JSON-RPC batch (array-shaped
// request or response) is encountered. Batch support is OPTIONAL in JSON-RPC
// 2.0 and deliberately not implemented by this adapter (see the package
// documentation). Callers can distinguish the documented not-implemented
// case from transport failures with errors.As.
type BatchNotImplementedError struct {
	Direction string // "request" or "response"
}

// Error implements the error interface.
func (e *BatchNotImplementedError) Error() string {
	return fmt.Sprintf("jsonrpc: batch %ss are not implemented (single-call only; JSON-RPC 2.0 batches are optional)", e.Direction)
}

// rateLimiter is the minimal limiter surface this adapter composes
// (satisfied by *rate.TokenBucket per specs/PROTOCOLS.md §4.3).
type rateLimiter interface {
	Allow() bool
}

// nextIDFunc produces unique request IDs.
type nextIDFunc func() interface{}

// Client executes JSON-RPC 2.0 methods over HTTP POST against a single
// endpoint. It implements protocol.Adapter (Protocol, Discoverer, Executor).
//
// The zero value is not usable; construct with NewClient.
type Client struct {
	endpoint   string
	httpClient *client.Client
	rate       rateLimiter
	timeout    time.Duration
	nextID     nextIDFunc
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies the shared HTTP client (pkg/client with its
// tracing/retry wiring) used for both discovery fetches and executions.
// Per specs/PROTOCOLS.md §3.1 adapters must not open their own HTTP clients
// for discovery; callers should pass the process-wide client here.
func WithHTTPClient(c *client.Client) Option {
	return func(cl *Client) {
		if c != nil {
			cl.httpClient = c
		}
	}
}

// WithRateLimiter composes a token-bucket limiter over Execute calls
// (specs/PROTOCOLS.md §4.3: rate limit → auth → timeout → single attempt).
// A nil limiter disables rate limiting.
func WithRateLimiter(l rateLimiter) Option {
	return func(cl *Client) {
		cl.rate = l
	}
}

// WithTimeout sets a per-call wall-clock bound applied when the caller's
// context carries no deadline. The caller's own deadline always wins
// (specs/PROTOCOLS.md §4.3: an adapter never overrides the caller's
// deadline). Zero or negative disables the bound.
func WithTimeout(d time.Duration) Option {
	return func(cl *Client) {
		cl.timeout = d
	}
}

// WithIDSource overrides the JSON-RPC request-ID source. JSON-RPC 2.0 IDs
// may be strings, numbers, or null; the default is a per-Client numeric
// counter. Supplying a deterministic source is useful for tests, replay
// tooling, and callers that must correlate IDs with upstream systems.
func WithIDSource(fn func() interface{}) Option {
	return func(cl *Client) {
		if fn != nil {
			cl.nextID = fn
		}
	}
}

// NewClient creates a JSON-RPC 2.0 client bound to the given HTTP endpoint
// (the URL an OpenRPC document's "servers" entry would name).
func NewClient(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint:   endpoint,
		httpClient: client.NewClient(),
		timeout:    30 * time.Second,
	}
	c.nextID = defaultIDSource()
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Protocol implements protocol.Adapter.
func (c *Client) Protocol() protocol.Protocol {
	return protocol.ProtocolJSONRPC
}

// Discover implements protocol.Discoverer: it ingests an OpenRPC 1.x
// discovery document from uri (a filesystem path or an http(s) URL —
// file first) and maps its methods onto protocol-neutral operations.
func (c *Client) Discover(ctx context.Context, uri string) ([]*protocol.Operation, error) {
	docBytes, err := loadDocument(ctx, c.httpClient, uri)
	if err != nil {
		return nil, err
	}

	var doc openrpcDocument
	if err := json.Unmarshal(docBytes, &doc); err != nil {
		return nil, errors.Wrap(err, errors.CodeBadRequest, "jsonrpc: decode OpenRPC document")
	}
	if doc.OpenRPC == "" {
		return nil, errors.New(errors.CodeValidation, `jsonrpc: document is missing the "openrpc" field; not an OpenRPC 1.x document`)
	}

	ops := doc.operations()
	if err := uniqueIDs(ops); err != nil {
		return nil, err
	}
	return ops, nil
}

// Execute implements protocol.Executor: it runs the named operation as a
// single JSON-RPC 2.0 request over HTTP POST and returns the decoded result
// value. args keys match the operation's Parameter names and are sent as the
// request's "params" object (empty args are sent as {}).
func (c *Client) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	if strings.TrimSpace(opID) == "" {
		return nil, errors.New(errors.CodeValidation, "jsonrpc: operation ID is required")
	}
	if args == nil {
		args = map[string]interface{}{}
	}

	params, err := json.Marshal(args)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeValidation, "jsonrpc: encode params")
	}

	req := mcp.JSONRPCRequest{
		JSONRPC: rpcSpecVersion,
		ID:      c.nextID(),
		Method:  opID,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "jsonrpc: encode request")
	}

	callCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	rawResp, err := c.post(callCtx, body)
	if err != nil {
		return nil, err
	}
	return decodeResponse(rawResp, req.ID)
}

// post sends the request body to the endpoint and returns the raw response
// bytes. Transport-level failures map onto AppError codes here; the JSON-RPC
// response layer is decoded by decodeResponse. Composition order follows
// specs/PROTOCOLS.md §4.3: rate limit → timeout/deadline → single attempt
// (retries, when configured, loop the whole order via pkg/client).
func (c *Client) post(ctx context.Context, body []byte) ([]byte, error) {
	if c.rate != nil && !c.rate.Allow() {
		return nil, errors.New(errors.CodeRateLimit, "jsonrpc: local rate limit refused the call")
	}

	builder := request.NewBuilder(c.endpoint, "", http.MethodPost)
	builder.JSONBody(string(body))
	builder.SetHeader("Content-Type", "application/json")

	req, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "jsonrpc: build request")
	}
	req = req.WithContext(ctx)

	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "jsonrpc: call timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "jsonrpc: transport failure")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusRequestTimeout {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeTimeout, "jsonrpc: endpoint reported timeout")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		appErr := errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeRateLimit, "jsonrpc: endpoint rate limited the call")
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			appErr = appErr.WithDetail("retry_after", ra)
		}
		return nil, appErr
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeUnauthorized, "jsonrpc: endpoint refused the credentials")
	}
	if resp.StatusCode >= 400 {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest, "jsonrpc: endpoint rejected the call")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "jsonrpc: read response body")
	}
	return raw, nil
}

// decodeResponse parses a single (non-batch) JSON-RPC 2.0 response object
// and maps a JSON-RPC error object onto AppError codes per the spec table.
func decodeResponse(raw []byte, sentID interface{}) (interface{}, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		return nil, &BatchNotImplementedError{Direction: "response"}
	}

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "jsonrpc: decode response")
	}
	if resp.JSONRPC != "" && resp.JSONRPC != rpcSpecVersion {
		return nil, errors.New(errors.CodeInternal, fmt.Sprintf("jsonrpc: response carries unsupported jsonrpc version %q", resp.JSONRPC))
	}
	if !idsMatch(resp.ID, sentID) {
		return nil, errors.New(errors.CodeInternal, "jsonrpc: response ID does not match the request ID")
	}
	if resp.Error != nil {
		return nil, mapRPCError(resp.Error)
	}
	if len(resp.Result) == 0 {
		return nil, errors.New(errors.CodeInternal, "jsonrpc: response carries neither result nor error")
	}

	var result interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "jsonrpc: decode result")
	}
	return result, nil
}
