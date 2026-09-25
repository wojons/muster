package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// IntrospectionScheme is the discovery URI scheme that tells Discover to run a
// LIVE introspection query: "graphql://host:port/path" is posted to as
// http://host:port/path. An http(s) URL, by contrast, is read as a DOCUMENT
// (SDL or a recorded introspection JSON response) — the scheme decides, so a
// caller never has to guess whether a fetch will be a GET or a GraphQL POST.
const IntrospectionScheme = "graphql://"

// IntrospectionQuery is the standard introspection query this adapter posts to
// discover a live schema. It is a constant so operators can reproduce the exact
// request by hand.
const IntrospectionQuery = `query IntrospectionQuery {
  __schema {
    queryType {
      name
    }
    mutationType {
      name
    }
    subscriptionType {
      name
    }
    types {
      kind
      name
      description
      fields {
        name
        description
        args {
          name
          description
          type {
            ...TypeRef
          }
          defaultValue
        }
        type {
          ...TypeRef
        }
      }
      inputFields {
        name
        description
        type {
          ...TypeRef
        }
        defaultValue
      }
      enumValues {
        name
        description
      }
      interfaces {
        name
      }
      possibleTypes {
        name
      }
    }
  }
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
          }
        }
      }
    }
  }
}`

// DefaultTimeout bounds an Execute call whose caller's context carries no
// deadline of its own.
const DefaultTimeout = 30 * time.Second

// rateLimiter is the minimal limiter surface this adapter composes (satisfied
// by *rate.TokenBucket per specs/PROTOCOLS.md §4.3).
type rateLimiter interface {
	Allow() bool
}

// Client is the GraphQL adapter: it discovers operations from a schema
// (introspection or SDL) and executes them as single-endpoint GraphQL POSTs. It
// implements protocol.Adapter (Protocol, Discoverer, Executor).
//
// The zero value is not usable; construct with NewClient.
type Client struct {
	endpoint   string
	httpClient *client.Client
	rate       rateLimiter
	timeout    time.Duration
	sel        SelectionOptions

	schema    *Schema
	builder   *Builder
	ops       []*protocol.Operation
	schemaURI string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies the shared HTTP client (pkg/client, with its
// tracing/retry wiring) used for discovery fetches and executions. Per
// specs/PROTOCOLS.md §3.1 adapters do not open their own HTTP clients for
// discovery; callers pass the process-wide client here.
func WithHTTPClient(c *client.Client) Option {
	return func(cl *Client) {
		if c != nil {
			cl.httpClient = c
		}
	}
}

// WithRateLimiter composes a token-bucket limiter over Execute calls
// (specs/PROTOCOLS.md §4.3: rate limit → auth → timeout → single attempt).
func WithRateLimiter(l rateLimiter) Option {
	return func(cl *Client) {
		cl.rate = l
	}
}

// WithTimeout bounds an Execute call when the caller's context carries no
// deadline. A caller's own deadline always wins, earlier or later
// (specs/PROTOCOLS.md §4.3: an adapter never overrides the caller's deadline).
func WithTimeout(d time.Duration) Option {
	return func(cl *Client) {
		cl.timeout = d
	}
}

// WithSchema makes the client self-sufficient: Execute loads the schema from
// uri on first use, so a caller that knows the operation IDs can invoke without
// calling Discover itself.
func WithSchema(uri string) Option {
	return func(cl *Client) {
		cl.schemaURI = uri
	}
}

// WithSelection sets the selection limits the request builder derives documents
// with (depth, per-level field cap, top-level field allow-list).
func WithSelection(sel SelectionOptions) Option {
	return func(cl *Client) {
		cl.sel = sel.withDefaults()
	}
}

// NewClient creates a GraphQL client bound to an endpoint. The endpoint may be
// empty when the caller only needs discovery or when the schema source itself
// names one (introspection targets do) — see SetEndpoint.
func NewClient(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint:   endpoint,
		httpClient: client.NewClient(),
		timeout:    DefaultTimeout,
		sel:        SelectionOptions{}.withDefaults(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Protocol implements protocol.Adapter.
func (c *Client) Protocol() protocol.Protocol {
	return protocol.ProtocolGraphQL
}

// Endpoint returns the endpoint Execute posts to.
func (c *Client) Endpoint() string { return c.endpoint }

// Schema returns the ingested schema, or nil before discovery.
func (c *Client) Schema() *Schema { return c.schema }

// Operations returns the operations discovered by the most recent Discover.
func (c *Client) Operations() []*protocol.Operation { return c.ops }

// Builder returns the request builder, or nil before discovery.
func (c *Client) Builder() *Builder { return c.builder }

// Discover implements protocol.Discoverer: it ingests a GraphQL schema from uri
// and returns the operations it exposes.
//
// uri forms:
//
//	graphql://host:port/path   live introspection query against that endpoint
//	http(s)://…/schema.graphql an SDL document served over HTTP
//	http(s)://…/introspection.json  a recorded introspection response
//	/path/schema.graphql or /path/introspection.json  a local document
//	file:///path/…             a local document
//
// Results are deterministic for a given document: discovery issues exactly one
// read (or one introspection query) and never writes to the target.
func (c *Client) Discover(ctx context.Context, uri string) ([]*protocol.Operation, error) {
	schema, err := c.LoadSchema(ctx, uri)
	if err != nil {
		return nil, err
	}

	ops, err := schema.Operations(c.endpoint)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		counts := schema.Counts()
		extra := ""
		if counts.Subscriptions > 0 {
			extra = fmt.Sprintf(" (%d subscription field(s) are out of scope for this adapter)", counts.Subscriptions)
		}
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: schema %s exposes no ingestable operations: %d query and %d mutation field(s)%s",
			uri, counts.Queries, counts.Mutations, extra))
	}

	builder, err := NewBuilder(schema, c.sel)
	if err != nil {
		return nil, err
	}
	c.schema, c.builder, c.ops, c.schemaURI = schema, builder, ops, uri
	return ops, nil
}

// LoadSchema ingests a schema from uri without installing it on the client
// (Discover installs it and derives the operations).
func (c *Client) LoadSchema(ctx context.Context, uri string) (*Schema, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, errors.New(errors.CodeValidation, "graphql: discovery URI is empty")
	}

	// Live introspection target.
	if endpoint, ok := strings.CutPrefix(uri, IntrospectionScheme); ok {
		if strings.TrimSpace(endpoint) == "" {
			return nil, errors.New(errors.CodeValidation, "graphql: introspection URI names no host (expected graphql://host:port/path)")
		}
		return c.introspectLive(ctx, endpoint, uri)
	}

	body, err := c.readDocument(ctx, uri)
	if err != nil {
		return nil, err
	}
	return c.ingestDocument(body, uri)
}

// introspectLive runs the introspection query against an endpoint and maps the
// response onto a Schema. The endpoint becomes the client's default endpoint
// when none was configured, so invocations against the same server need no
// repeated flag.
func (c *Client) introspectLive(ctx context.Context, hostAndPath, uri string) (*Schema, error) {
	endpoint := "http://" + strings.TrimSpace(hostAndPath)
	if c.endpoint == "" {
		c.endpoint = endpoint
	}

	req, err := NewRequest(endpoint, IntrospectionQuery, "IntrospectionQuery")
	if err != nil {
		return nil, err
	}
	raw, err := c.post(ctx, req)
	if err != nil {
		return nil, err
	}

	schema, errs, err := c.schemaFromResponse(raw)
	if err != nil {
		return nil, err
	}
	if schema == nil {
		if looksLikeIntrospectionRefusal(errs) {
			return nil, introspectionDisabledError(endpoint, errs)
		}
		if len(errs) > 0 {
			return nil, errorsAppError(errs)
		}
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: %s answered the introspection query without a __schema object; it may not be a GraphQL endpoint", endpoint))
	}

	schema.Source = uri
	schema.SourceFormat = FormatIntrospectionQuery
	return schema, nil
}

// readDocument loads a schema document from disk or over http(s). A path that
// exists on disk always wins (deterministic and testable without a server).
func (c *Client) readDocument(ctx context.Context, uri string) ([]byte, error) {
	path := strings.TrimPrefix(uri, "file://")
	if _, statErr := os.Stat(path); statErr == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeBadRequest, "graphql: read schema document")
		}
		return data, nil
	}

	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return nil, errors.New(errors.CodeNotFound, fmt.Sprintf(
			"graphql: schema source %q not found on disk and not an http(s) URL (use %s for a live introspection query)",
			uri, IntrospectionScheme))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeBadRequest, "graphql: build discovery request")
	}
	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "graphql: schema fetch timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "graphql: schema fetch failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest,
			fmt.Sprintf("graphql: schema fetch from %s was rejected (if %s is a GraphQL endpoint rather than a document URL, use the %s form to introspect it)",
				uri, uri, IntrospectionScheme))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "graphql: read schema document")
	}
	return data, nil
}

// ingestDocument maps a document body onto a Schema, choosing the ingestion
// path by content: a JSON object is an introspection response, anything else is
// SDL.
func (c *Client) ingestDocument(body []byte, uri string) (*Schema, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: schema document %s is empty", uri))
	}

	if strings.HasPrefix(trimmed, "{") {
		schema, errs, err := c.schemaFromResponse(body)
		if err != nil {
			return nil, err
		}
		if schema == nil {
			if looksLikeIntrospectionRefusal(errs) {
				return nil, introspectionDisabledError(uri, errs)
			}
			if len(errs) > 0 {
				return nil, errorsAppError(errs)
			}
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"graphql: %s is JSON but carries no __schema object and no SDL; expected a recorded introspection response or an SDL document", uri))
		}
		schema.Source = uri
		schema.SourceFormat = FormatIntrospectionJSON
		return schema, nil
	}

	schema, err := ParseSDL(string(body))
	if err != nil {
		return nil, err
	}
	schema.Source = uri
	schema.SourceFormat = FormatSDL
	return schema, nil
}

// schemaFromResponse decodes a body that may hold an introspection response and
// returns the schema (nil when the body carries none) plus any GraphQL errors
// the response reported.
func (c *Client) schemaFromResponse(body []byte) (*Schema, []GraphQLError, error) {
	is, errs, err := decodeIntrospectionSchema(body)
	if err != nil {
		return nil, errs, err
	}
	if is == nil {
		return nil, errs, nil
	}
	schema, err := schemaFromIntrospection(is)
	if err != nil {
		return nil, errs, err
	}
	return schema, errs, nil
}

// Execute implements protocol.Executor: it builds the operation's query
// document, binds the arguments as variables, sends ONE GraphQL POST to the
// endpoint, and returns the decoded data.
//
// A response carrying data AND errors (GraphQL's partial-success shape) returns
// a *PartialResult rather than hiding either half; a response carrying only
// errors maps onto AppError codes per specs/PROTOCOLS.md §4.2.
func (c *Client) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	if strings.TrimSpace(opID) == "" {
		return nil, errors.New(errors.CodeValidation, "graphql: operation ID is required")
	}
	if err := c.ensureSchema(ctx); err != nil {
		return nil, err
	}

	op := OperationByID(c.ops, opID)
	if op == nil {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: operation %q is not in the discovered schema (discovered: %s)",
			opID, strings.Join(OperationIDs(c.ops), ", ")))
	}

	req, err := c.builder.Build(op, args, c.endpoint)
	if err != nil {
		return nil, err
	}

	raw, err := c.post(ctx, req)
	if err != nil {
		return nil, err
	}
	return decodeResponse(raw)
}

// ensureSchema loads the configured schema on first use (WithSchema), so a
// caller that knows the operation IDs can Execute without calling Discover.
func (c *Client) ensureSchema(ctx context.Context) error {
	if c.builder != nil {
		return nil
	}
	if c.schemaURI == "" {
		return errors.New(errors.CodeValidation,
			"graphql: no schema is loaded; call Discover with a schema source (or configure the client with WithSchema) before Execute")
	}
	_, err := c.Discover(ctx, c.schemaURI)
	return err
}

// post sends one built request. Transport-level failures map onto AppError
// codes here, at the boundary (specs/PROTOCOLS.md §4.2), and composition order
// follows §4.3: rate limit → timeout/deadline → single attempt.
func (c *Client) post(ctx context.Context, req *Request) ([]byte, error) {
	if c.rate != nil && !c.rate.Allow() {
		return nil, errors.New(errors.CodeRateLimit, "graphql: local rate limit refused the call")
	}

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && c.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	httpReq, err := req.HTTPRequest(callCtx)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(callCtx, httpReq)
	if err != nil {
		if callCtx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "graphql: call timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "graphql: transport failure")
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusRequestTimeout:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeTimeout, "graphql: endpoint reported a timeout")
	case resp.StatusCode == http.StatusTooManyRequests:
		appErr := errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeRateLimit, "graphql: endpoint rate limited the call")
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			appErr = appErr.WithDetail("retry_after", ra)
		}
		return nil, appErr
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeUnauthorized, "graphql: endpoint refused the credentials")
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeNotFound, "graphql: endpoint not found")
	case resp.StatusCode >= http.StatusInternalServerError:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeInternal, "graphql: endpoint failed")
	case resp.StatusCode >= 400:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest, "graphql: endpoint rejected the request")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "graphql: read response body")
	}
	return raw, nil
}

// PartialResult is what Execute returns for a GraphQL response that carries both
// data and errors. GraphQL's spec allows partial success; the adapter reports
// it instead of discarding either half.
type PartialResult struct {
	// Data is the decoded data member (partial, per the errors).
	Data interface{} `json:"data"`
	// Errors are the response's errors, unmapped: a partial result is a
	// successful call with field-level failures, not a failed call.
	Errors []GraphQLError `json:"errors"`
}

// graphqlResponse is the wire shape of a GraphQL response body.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors"`
}

// decodeResponse decodes one GraphQL response body.
func decodeResponse(raw []byte) (interface{}, error) {
	var resp graphqlResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "graphql: decode response")
	}

	hasData := len(resp.Data) > 0 && strings.TrimSpace(string(resp.Data)) != "null"

	if !hasData {
		if len(resp.Errors) > 0 {
			return nil, errorsAppError(resp.Errors)
		}
		return nil, errors.New(errors.CodeInternal, "graphql: response carries neither data nor errors")
	}

	var data interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "graphql: decode data member")
	}
	if len(resp.Errors) > 0 {
		return &PartialResult{Data: data, Errors: resp.Errors}, nil
	}
	return data, nil
}

// NewRequest builds a Request directly from a query document (used for the
// introspection query, which has no ingested operation behind it).
func NewRequest(endpoint, query, operationName string) (*Request, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New(errors.CodeValidation, "graphql: query document is empty")
	}
	return &Request{
		Endpoint:      endpoint,
		Query:         query,
		OperationName: operationName,
		Variables:     map[string]interface{}{},
	}, nil
}

// compile-time pin: the client is a protocol.Adapter.
var _ protocol.Adapter = (*Client)(nil)
