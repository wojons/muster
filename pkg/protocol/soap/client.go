package soap

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
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/request"
)

// DefaultTimeout bounds an Execute call whose caller's context carries no
// deadline of its own.
const DefaultTimeout = 30 * time.Second

// httpClient is the minimal transport surface the adapter composes
// (satisfied by *client.Client per specs/PROTOCOLS.md §3.1).
type httpClient interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// rateLimiter is the minimal limiter surface this adapter composes
// (satisfied by *rate.TokenBucket per §4.3).
type rateLimiter interface {
	Allow() bool
}

// Client is the SOAP adapter: it discovers operations from a WSDL 1.1 / 2.0
// document and executes them as version-appropriate SOAP envelopes over HTTP
// POST. It implements protocol.Adapter (Protocol, Discoverer, Executor).
//
// The zero value is not usable; construct with NewClient.
type Client struct {
	endpoint   string
	httpClient httpClient
	rate       rateLimiter
	timeout    time.Duration
	version    SOAPVersion
	addressing bool

	doc *WSDLDoc
	ops []*protocol.Operation
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies the shared HTTP client (pkg/client, with its
// tracing/retry wiring) for WSDL fetches and executions. Adapters never open
// their own HTTP clients (specs/PROTOCOLS.md §3.1).
func WithHTTPClient(c *client.Client) Option {
	return func(cl *Client) {
		if c != nil {
			cl.httpClient = c
		}
	}
}

// WithRateLimiter composes a token-bucket limiter over Execute calls (§4.3:
// rate limit → timeout/deadline → single attempt).
func WithRateLimiter(l rateLimiter) Option {
	return func(cl *Client) { cl.rate = l }
}

// WithTimeout bounds an Execute call when the caller's context carries no
// deadline. The caller's own deadline always wins (§4.3).
func WithTimeout(d time.Duration) Option {
	return func(cl *Client) { cl.timeout = d }
}

// WithVersion forces the SOAP envelope version (1.1 or 1.2) regardless of
// what the WSDL binding implies. Without it the version is taken from the
// document (1.1 for a WSDL 1.1 binding, 1.2 for WSDL 2.0).
func WithVersion(v SOAPVersion) Option {
	return func(cl *Client) {
		if v == SOAP11 || v == SOAP12 {
			cl.version = v
		}
	}
}

// WithAddressing enables the WS-Addressing 1.0 header block (wsa:Action,
// wsa:To, wsa:MessageID) on outgoing envelopes. The wsa:Action value is the
// WSDL operation's SOAPAction (or the WSDL 2.0 action). WS-Security itself
// remains OUT OF SCOPE (see the package documentation).
func WithAddressing(enable bool) Option {
	return func(cl *Client) { cl.addressing = enable }
}

// NewClient creates a SOAP client bound to an endpoint. The endpoint may be
// empty when discovery names one (the WSDL's service address) — see
// SetEndpoint.
func NewClient(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint:   endpoint,
		httpClient: client.NewClient(),
		timeout:    DefaultTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Protocol implements protocol.Adapter.
func (c *Client) Protocol() protocol.Protocol { return protocol.ProtocolSOAP }

// Endpoint returns the endpoint Execute posts to.
func (c *Client) Endpoint() string { return c.endpoint }

// Operations returns the operations discovered by the most recent Discover.
func (c *Client) Operations() []*protocol.Operation { return c.ops }

// Document returns the parsed WSDL document, or nil before discovery.
func (c *Client) Document() *WSDLDoc { return c.doc }

// envelopeVersion resolves the SOAP version for one call: an explicit
// WithVersion wins; otherwise the WSDL document's version.
func (c *Client) envelopeVersion() SOAPVersion {
	if c.version == SOAP11 || c.version == SOAP12 {
		return c.version
	}
	if c.doc != nil && c.doc.Version == SOAP12 {
		return SOAP12
	}
	return SOAP11
}

// Envelope is one built SOAP envelope: the exact bytes to post, the headers
// to send with them, and the addressing block when enabled.
type Envelope struct {
	XML         []byte
	ContentType string
	SOAPAction  string // HTTP SOAPAction header value (1.1 only)
}

// BuildEnvelope builds the operation's request envelope from argument values.
//
// Header shape per version (acceptance criterion 2):
//
//	SOAP 1.1  POST body Content-Type: text/xml; charset=utf-8
//	          HTTP header: SOAPAction: "<action>"
//	SOAP 1.2  POST body Content-Type: application/soap+xml; charset=utf-8;
//	          action="<action>" (no SOAPAction HTTP header)
//
// With WithAddressing(true) a WS-Addressing 1.0 header block is added:
// wsa:Action (the SOAPAction / WSDL 2.0 action), wsa:To (the endpoint), and
// a wsa:MessageID with a urn:uuid value derived from the current time.
//
// args keys match the operation's RequestBody property names; a Repeated
// property's value is a JSON array (one XML child element per entry), a
// nillable/optional property omitted from args is simply not emitted.
func (c *Client) BuildEnvelope(op *protocol.Operation, args map[string]interface{}) (*Envelope, error) {
	version := c.envelopeVersion()

	bodyChildren, err := c.buildBodyXML(op, args)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	if version == SOAP11 {
		b.WriteString(`<soapenv:Envelope xmlns:soapenv="` + EnvelopeNS11 + `">`)
	} else {
		b.WriteString(`<env:Envelope xmlns:env="` + EnvelopeNS12 + `">`)
	}

	action := op.Source[SourceSOAPAction]
	if c.addressing {
		b.WriteString(`<wsa:Header xmlns:wsa="` + AddressingNS + `">`)
		b.WriteString(`<wsa:Action>` + xmlEscape(action) + `</wsa:Action>`)
		b.WriteString(`<wsa:To>` + xmlEscape(c.endpoint) + `</wsa:To>`)
		b.WriteString(`<wsa:MessageID>urn:uuid:` + messageID() + `</wsa:MessageID>`)
		b.WriteString(`</wsa:Header>`)
	}

	envPrefix := "soapenv"
	if version == SOAP12 {
		envPrefix = "env"
	}
	b.WriteString(`<` + envPrefix + `:Body>`)
	b.WriteString(bodyChildren)
	b.WriteString(`</` + envPrefix + `:Body></` + envPrefix + `:Envelope>`)

	env := &Envelope{XML: []byte(b.String())}
	switch version {
	case SOAP11:
		env.ContentType = "text/xml; charset=utf-8"
		env.SOAPAction = action
	case SOAP12:
		env.ContentType = `application/soap+xml; charset=utf-8; action="` + action + `"`
	}
	return env, nil
}

// buildBodyXML renders the operation's payload element: document style wraps
// the input element QName's local part; the element's depth-1 fields become
// child elements in declaration order. rpc style wraps the operation name
// with its parts as children.
func (c *Client) buildBodyXML(op *protocol.Operation, args map[string]interface{}) (string, error) {
	style := op.Source[SourceStyle]
	inputElem := op.Source[SourceInputElem]

	var children strings.Builder
	for _, prop := range op.RequestBody {
		value, present := args[prop.Name]
		if !present || value == nil {
			continue // optional / nillable element omitted
		}
		fragment, err := valueToXML(prop, value)
		if err != nil {
			return "", errors.Wrap(err, errors.CodeValidation,
				fmt.Sprintf("soap: encode request field %q", prop.Name))
		}
		children.WriteString(fragment)
	}

	if style == "rpc" || inputElem == "" {
		opName := op.Source[SourceOperation]
		return "<" + xmlEscape(opName) + ">" + children.String() + "</" + xmlEscape(opName) + ">", nil
	}
	local := inputElem
	if i := strings.LastIndex(local, "}"); i >= 0 {
		local = local[i+1:]
	}
	return "<" + xmlEscape(local) + ">" + children.String() + "</" + xmlEscape(local) + ">", nil
}

// valueToXML renders one request-body property as an XML element fragment.
// A repeated property's array value emits one element per entry; scalars are
// XML-escaped text; objects render each key as a child element (depth-2
// simple structure — nested objects beyond that render their JSON form,
// which keeps the builder honest rather than silently lossy).
func valueToXML(prop protocol.RequestBodyProperty, value interface{}) (string, error) {
	var renderOne func(name string, v interface{}) (string, error)
	renderOne = func(name string, v interface{}) (string, error) {
		switch val := v.(type) {
		case []interface{}:
			var b strings.Builder
			for _, item := range val {
				frag, err := renderOne(name, item)
				if err != nil {
					return "", err
				}
				b.WriteString(frag)
			}
			return b.String(), nil
		case map[string]interface{}:
			var b strings.Builder
			b.WriteString("<" + xmlEscape(name) + ">")
			keys := make([]string, 0, len(val))
			for k := range val {
				keys = append(keys, k)
			}
			sortStrings(keys)
			for _, k := range keys {
				frag, err := renderOne(k, val[k])
				if err != nil {
					return "", err
				}
				b.WriteString(frag)
			}
			b.WriteString("</" + xmlEscape(name) + ">")
			return b.String(), nil
		default:
			return "<" + xmlEscape(name) + ">" + xmlEscape(scalarString(v)) + "</" + xmlEscape(name) + ">", nil
		}
	}
	return renderOne(prop.Name, value)
}

// scalarString renders a scalar argument value as its XML text form.
func scalarString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		return trimFloat(val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprint(v)
	}
}

// trimFloat renders a JSON-decoded number without a trailing ".0" artifact.
func trimFloat(f float64) string {
	s := fmt.Sprintf("%g", f)
	return s
}

// messageID derives a WS-Addressing MessageID suffix. Deterministic per call
// instant; uniqueness across a second boundary is the caller's concern (the
// header is diagnostic for this client-side adapter).
func messageID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// xmlEscape escapes text/attribute content for embedding in the envelope.
func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sortStrings sorts in place (stdlib sort via a tiny indirection to keep the
// import set flat in this file).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Execute implements protocol.Executor: it resolves the named operation,
// builds the version-appropriate envelope, posts it once over pkg/client +
// pkg/request.Builder, and decodes the response body as JSON by default or
// raw XML when the args carry {"_output": "xml"}.
func (c *Client) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	if strings.TrimSpace(opID) == "" {
		return nil, errors.New(errors.CodeValidation, "soap: operation ID is required")
	}
	if c.doc == nil {
		return nil, errors.New(errors.CodeValidation,
			"soap: no WSDL is loaded; call Discover with a WSDL document before Execute")
	}
	var op *protocol.Operation
	for _, o := range c.ops {
		if o.ID == opID {
			op = o
			break
		}
	}
	if op == nil {
		ids := make([]string, 0, len(c.ops))
		for _, o := range c.ops {
			ids = append(ids, o.ID)
		}
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"soap: operation %q is not in the discovered WSDL (discovered: %s)", opID, strings.Join(ids, ", ")))
	}

	endpoint := c.endpoint
	if endpoint == "" {
		endpoint = op.Source[SourceEndpoint]
	}
	if endpoint == "" {
		return nil, errors.New(errors.CodeValidation,
			"soap: no endpoint: pass one to NewClient or supply a wsdl:service address in the document")
	}

	// Response rendering selection: JSON default, raw XML on demand.
	output := "json"
	if args != nil {
		if raw, ok := args["_output"]; ok {
			if s, isStr := raw.(string); isStr && (s == "xml" || s == "json") {
				output = s
			} else {
				return nil, errors.New(errors.CodeValidation,
					`soap: "_output" must be "json" or "xml"`)
			}
		}
	}

	env, err := c.BuildEnvelope(op, args)
	if err != nil {
		return nil, err
	}

	raw, err := c.post(ctx, endpoint, env)
	if err != nil {
		return nil, err
	}
	return decodeResponse(raw, output)
}

// post sends one envelope over pkg/request.Builder + pkg/client. Transport
// failures map onto AppError codes at this boundary (§4.2); composition order
// follows §4.3 (rate limit → deadline → single attempt).
func (c *Client) post(ctx context.Context, endpoint string, env *Envelope) ([]byte, error) {
	if c.rate != nil && !c.rate.Allow() {
		return nil, errors.New(errors.CodeRateLimit, "soap: local rate limit refused the call")
	}

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && c.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	builder := request.NewBuilder(endpoint, "", http.MethodPost)
	builder.WithBody(strings.NewReader(string(env.XML)))
	builder.WithContentType(env.ContentType)
	builder.SetHeader("Content-Type", env.ContentType)
	if env.SOAPAction != "" {
		builder.SetHeader("SOAPAction", env.SOAPAction)
	}
	req, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "soap: build request")
	}
	req = req.WithContext(callCtx)

	resp, err := c.httpClient.Do(callCtx, req)
	if err != nil {
		if callCtx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "soap: call timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "soap: transport failure")
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusRequestTimeout:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeTimeout, "soap: endpoint reported a timeout")
	case resp.StatusCode == http.StatusTooManyRequests:
		appErr := errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeRateLimit, "soap: endpoint rate limited the call")
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			appErr = appErr.WithDetail("retry_after", ra)
		}
		return nil, appErr
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeUnauthorized, "soap: endpoint refused the credentials")
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeNotFound, "soap: endpoint not found")
	case resp.StatusCode == http.StatusNotImplemented:
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest, "soap: endpoint does not implement the operation (HTTP 501)")
	case resp.StatusCode >= http.StatusInternalServerError:
		// A 5xx body may still carry a SOAP Fault: read it and prefer the
		// fault mapping over the bare status mapping.
		body, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			if fault, faultErr := parseFault(body); faultErr == nil && fault != nil {
				return nil, fault.toAppError()
			}
		}
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeInternal, "soap: endpoint failed")
	case resp.StatusCode >= 400:
		// SOAP 1.2 faults conventionally arrive with HTTP 4xx/5xx; map the
		// fault when one is present.
		body, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			if fault, faultErr := parseFault(body); faultErr == nil && fault != nil {
				return nil, fault.toAppError()
			}
		}
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest, "soap: endpoint rejected the call")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "soap: read response body")
	}
	return raw, nil
}

// decodeResponse renders a SOAP response envelope: JSON (default) or raw XML.
// A response carrying a SOAP Fault (either version's shape, including 200 OK
// responses — faults do not require a non-2xx status) maps onto AppError
// codes with the fault code and detail preserved.
func decodeResponse(raw []byte, output string) (interface{}, error) {
	if output == "xml" {
		return string(raw), nil
	}

	// Fault first: a Fault anywhere in the Body is a failed call even on a
	// 200 status.
	if fault, err := parseFault(raw); err == nil && fault != nil {
		return nil, fault.toAppError()
	}

	root, err := parseXMLTree(raw)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "soap: decode response envelope")
	}
	body := findBody(root)
	if body == nil {
		return nil, errors.New(errors.CodeInternal, "soap: response carries no SOAP Body")
	}
	// The first payload element inside the Body (skipping any non-fault
	// infrastructure elements) becomes the JSON result.
	var payload *xmlNode
	for _, child := range body.Children {
		if child.Name == "Fault" {
			continue
		}
		payload = child
		break
	}
	if payload == nil {
		return map[string]interface{}{}, nil // empty body: an accepted one-way call
	}
	return nodeToJSON(payload), nil
}

// findBody locates the SOAP Body element (either version's namespace) at any
// depth below the root — real services nest it directly under the Envelope.
func findBody(n *xmlNode) *xmlNode {
	if n == nil {
		return nil
	}
	if n.Name == "Body" {
		return n
	}
	for _, c := range n.Children {
		if b := findBody(c); b != nil {
			return b
		}
	}
	return nil
}

// nodeToJSON converts a decoded XML node into JSON-serializable data:
// attributes appear under "@name", children under their element names
// (repeated children become arrays), text-only elements become strings.
func nodeToJSON(n *xmlNode) interface{} {
	obj := map[string]interface{}{}
	for k, v := range n.Attrs {
		if strings.Contains(k, ":") {
			continue // namespaced attribute keys are noise for the JSON view
		}
		obj["@"+k] = v
	}
	text := strings.TrimSpace(n.Text)
	for _, c := range n.Children {
		val := nodeToJSON(c)
		existing, ok := obj[c.Name]
		if !ok {
			obj[c.Name] = val
			continue
		}
		list, isList := existing.([]interface{})
		if !isList {
			list = []interface{}{existing}
		}
		obj[c.Name] = append(list, val)
	}
	if len(obj) == 0 {
		return text
	}
	if text != "" && len(n.Children) == 0 && len(obj) == 0 {
		return text
	}
	if _, hasAttrs := obj[""]; hasAttrs {
		delete(obj, "")
	}
	return obj
}

// compile-time pin: the client is a protocol.Adapter.
var _ protocol.Adapter = (*Client)(nil)
