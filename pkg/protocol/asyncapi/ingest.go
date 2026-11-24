package asyncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// Source keys stamped on every operation (PROTO-007 §2.2). Stage-3
// consumers MUST NOT interpret them; the executor and the adapter's own
// CLI wiring do.
const (
	SourceDocumentURI    = "document_uri"
	SourceDocumentTitle  = "document_title"
	SourceDocumentVer    = "document_version"
	SourceAsyncAPIVer    = "asyncapi_version"
	SourceChannelID      = "channel_id"
	SourceChannelAddress = "channel_address"
	SourceOperationID    = "operation_id"
	SourceMessageID      = "message_id"
	SourceServerName     = "server_name"
	SourceServerURL      = "server_url"
	SourceServerProtocol = "server_protocol"
	SourceServerBinding  = "binding.server"
	SourceChannelBinding = "binding.channel"
	SourceOpBinding      = "binding.operation"
	SourceMsgBinding     = "binding.message"
	SourceSecurity       = "security"
	SourceReply          = "reply"
	SourcePayloadRef     = "payload_ref"
)

// Parameter locations used by the adapter (PROTO-007 §2.2).
const (
	InChannel       = "channel"        // channel address parameter
	InMessageHeader = "message-header" // depth-1 message header property
)

// actionSend / actionReceive are the AsyncAPI 3.x operation actions this
// adapter maps. Any other action fails discovery with CodeValidation.
const (
	actionSend    = "send"
	actionReceive = "receive"
)

// reservedArgKey is the invocation argument that carries the Kafka message
// key of a publish (the document's kafka message-binding key schema is
// preserved as metadata, not enforced; the key travels as an explicit
// invocation input, mirroring the SOAP adapter's reserved "_output" arg).
const reservedArgKey = "_key"

// reservedArgPayload names the whole-message argument of an operation whose
// payload schema carries no depth-1 properties (an opaque payload is sent
// as-is rather than silently flattened).
const reservedArgPayload = "payload"

// Reserved bounded-collect knobs of a receive operation (MCP schema
// vocabulary). They may LOWER the configured bounds; the executor's own
// limits (the MCP hard caps on the MCP path, the CLI flags on the CLI
// path) are never raised by them.
const (
	reservedArgMaxMessages    = "max_messages"
	reservedArgTimeoutSeconds = "timeout_seconds"
)

// identity stamp keys attached to resolved document objects so the map key
// an object was declared under survives $ref resolution (never collide with
// real AsyncAPI fields).
const (
	stampChannelKey = "___channel_key___"
	stampMessageKey = "___message_key___"
)

// Document is a parsed AsyncAPI 3.x document: the neutral facts the
// discoverer needs, with references already resolved.
type Document struct {
	URI      string
	Title    string
	Version  string // info.version
	AsyncAPI string // the document's asyncapi version string ("3.0.0")
	Servers  map[string]serverInfo
	Ops      []*protocol.Operation
}

// serverInfo is one servers[] entry.
type serverInfo struct {
	Name     string
	URL      string // host + pathname
	Protocol string
	Bindings string // canonical JSON, "" when absent
}

// operationDecl is one document operation after reference resolution.
type operationDecl struct {
	key      string // map key under "operations"
	id       string // operationId or "" (synthesized later)
	action   string
	summary  string
	desc     string
	tags     []string
	security string // canonical JSON, "" when absent
	reply    string // canonical JSON, "" when absent
	binding  string // canonical JSON, "" when absent
	channel  channelDecl
	messages []messageDecl
}

// channelDecl is one document channel after reference resolution.
type channelDecl struct {
	key     string
	address string
	params  map[string]interface{}
	binding string // canonical JSON, "" when absent
}

// messageDecl is one message after reference resolution.
type messageDecl struct {
	id      string
	headers map[string]interface{} // headers schema object, nil when absent
	payload map[string]interface{} // payload schema object, nil when absent
	ref     string                 // payload $ref when the payload is a bare reference
	binding string                 // canonical JSON, "" when absent
}

// idAssignment is the two-phase operation-ID assignment: explicit
// operationIds claim their names first (colliding explicit IDs are
// rejected), then synthesized IDs fill in with deterministic -2, -3
// suffixes on collision (PROTO-007 §2.2 hard rule: deterministic synthesis
// when absent, collisions rejected when they cannot resolve).
type idAssignment struct {
	key      string // operation map key
	msgIndex int    // index within the operation's expanded messages
	explicit bool
	base     string // the desired id before collision resolution
}

// ParseDocument validates and parses an AsyncAPI 3.x document body. It is
// the pure half of Discover (split out for deterministic unit tests):
// parsing never performs network activity.
func ParseDocument(uri string, data []byte) (*Document, error) {
	root, err := decodeDocument(data)
	if err != nil {
		return nil, err
	}

	version, _ := root["asyncapi"].(string)
	if !strings.HasPrefix(version, "3.") {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: unsupported document version %q: this adapter ingests AsyncAPI 3.x documents only (PROTO-007 §2.1)", version))
	}

	doc := &Document{URI: uri, AsyncAPI: version, Servers: map[string]serverInfo{}}

	info, _ := root["info"].(map[string]interface{})
	if info != nil {
		doc.Title, _ = info["title"].(string)
		doc.Version, _ = info["version"].(string)
	}

	if err := parseServers(doc, root); err != nil {
		return nil, err
	}

	channels, err := resolveChannels(root)
	if err != nil {
		return nil, err
	}

	opsRaw, ok := root["operations"].(map[string]interface{})
	if !ok || len(opsRaw) == 0 {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: document %s declares no operations; nothing to ingest (PROTO-007 §2.1: channels without operations are not synthesized)", uri))
	}

	// Phase 1 — resolve every operation (sorted keys), expanding one decl
	// per (operation, message) pair.
	type expanded struct {
		decl *operationDecl
		msg  messageDecl
	}
	var expansions []expanded
	for _, key := range sortedKeys(opsRaw) {
		raw, ok := opsRaw[key].(map[string]interface{})
		if !ok {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: operation %q is not an object", key))
		}
		decl, err := resolveOperation(root, key, raw, channels)
		if err != nil {
			return nil, err
		}
		for _, msg := range decl.messages {
			expansions = append(expansions, expanded{decl: decl, msg: msg})
		}
	}

	// Phase 2 — assign operation IDs (PROTO-007 §2.2).
	var assignments []idAssignment
	for _, e := range expansions {
		assignments = append(assignments, idAssignment{
			key:      e.decl.key,
			explicit: e.decl.id != "",
			base:     firstNonEmpty(e.decl.id, fmt.Sprintf("%s-%s-%s", e.decl.action, e.decl.channel.key, e.msg.id)),
		})
	}
	ids, err := assignOperationIDs(assignments)
	if err != nil {
		return nil, err
	}

	// Phase 3 — build the neutral operations in document order.
	ops := make([]*protocol.Operation, 0, len(expansions))
	for i, e := range expansions {
		ops = append(ops, buildOperation(doc, e.decl, e.msg, ids[i]))
	}
	doc.Ops = ops
	return doc, nil
}

// assignOperationIDs assigns every expansion a unique ID. Explicit IDs are
// claimed first (sorted); two explicit IDs colliding are a document defect
// and fail. Synthesized IDs claim next; a synthesized ID colliding with any
// claimed ID is re-suffixed -2, -3, ... until free (deterministic).
func assignOperationIDs(assignments []idAssignment) ([]string, error) {
	ids := make([]string, len(assignments))
	claimed := map[string]string{} // id → owning operation key
	claim := func(i int, id string) bool {
		if _, taken := claimed[id]; taken {
			return false
		}
		claimed[id] = assignments[i].key
		ids[i] = id
		return true
	}

	// Explicit first: a collision between two explicit IDs is unresolvable.
	for i, a := range assignments {
		if !a.explicit {
			continue
		}
		if prev, dup := claimed[a.base]; dup && prev != a.key {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: duplicate operationId %q (declared by operations %q and %q); resolve the document before discovery",
				a.base, prev, a.key))
		}
		claim(i, a.base)
	}
	// Synthesized next, suffixing deterministically on any collision.
	for i, a := range assignments {
		if a.explicit {
			continue
		}
		if claim(i, a.base) {
			continue
		}
		for n := 2; ; n++ {
			if claim(i, fmt.Sprintf("%s-%d", a.base, n)) {
				break
			}
		}
	}
	return ids, nil
}

// buildOperation maps one (operation, message) pair onto the neutral model.
func buildOperation(doc *Document, decl *operationDecl, msg messageDecl, id string) *protocol.Operation {
	op := &protocol.Operation{
		ID:          id,
		Protocol:    protocol.ProtocolAsyncAPI,
		Kind:        kindFor(decl.action),
		Tags:        decl.tags,
		Summary:     decl.summary,
		Description: decl.desc,
		Source: map[string]string{
			SourceDocumentURI:    doc.URI,
			SourceDocumentTitle:  doc.Title,
			SourceDocumentVer:    doc.Version,
			SourceAsyncAPIVer:    doc.AsyncAPI,
			SourceOperationID:    firstNonEmpty(decl.id, decl.key),
			SourceChannelID:      decl.channel.key,
			SourceChannelAddress: decl.channel.address,
			SourceMessageID:      msg.id,
		},
	}
	// Document-wide values are context, not unique operation IDs (§2.2):
	// they fill Summary/Description only when the per-operation value is
	// absent.
	if op.Summary == "" {
		op.Summary = doc.Title
	}
	if op.Description == "" && doc.Title != "" {
		op.Description = fmt.Sprintf("AsyncAPI %s operation %s (document %s v%s)",
			decl.action, decl.key, doc.Title, doc.Version)
	}

	if decl.channel.binding != "" {
		op.Source[SourceChannelBinding] = decl.channel.binding
	}
	if decl.binding != "" {
		op.Source[SourceOpBinding] = decl.binding
	}
	if msg.binding != "" {
		op.Source[SourceMsgBinding] = msg.binding
	}
	if decl.security != "" {
		op.Source[SourceSecurity] = decl.security
	}
	if decl.reply != "" {
		op.Source[SourceReply] = decl.reply
	}
	if msg.ref != "" {
		op.Source[SourcePayloadRef] = msg.ref
	}

	// Channel address parameters (§2.2): the document's required flag is
	// the truth; the executor additionally enforces every parameter the
	// address template actually needs.
	for _, name := range sortedKeys(decl.channel.params) {
		p, _ := decl.channel.params[name].(map[string]interface{})
		required, _ := p["required"].(bool)
		op.Parameters = append(op.Parameters, protocol.Parameter{
			Name:        name,
			In:          InChannel,
			Type:        paramSchemaType(p),
			Required:    required,
			Description: paramDescription(p),
		})
	}

	// Depth-1 message header properties (§2.2).
	if msg.headers != nil {
		props, _ := msg.headers["properties"].(map[string]interface{})
		required := requiredSet(msg.headers)
		for _, name := range sortedKeys(props) {
			p, _ := props[name].(map[string]interface{})
			op.Parameters = append(op.Parameters, protocol.Parameter{
				Name:        name,
				In:          InMessageHeader,
				Type:        schemaType(p),
				Required:    required[name],
				Enum:        enumValues(p),
				Default:     p["default"],
				Example:     p["example"],
				Description: schemaDescription(p),
			})
		}
	}

	// Payload: depth-1 properties onto RequestBody; an opaque payload (no
	// depth-1 properties) is carried as a single whole-object argument so
	// the operation ingests with an honest shape (§2.3).
	fields, hasFields := payloadProperties(msg.payload)
	if !hasFields {
		op.RequestBody = append(op.RequestBody, protocol.RequestBodyProperty{
			Name:        reservedArgPayload,
			Type:        "object",
			Description: opaquePayloadDescription(msg),
		})
	} else {
		required := requiredSet(msg.payload)
		for _, name := range sortedKeys(fields) {
			p, _ := fields[name].(map[string]interface{})
			op.RequestBody = append(op.RequestBody, protocol.RequestBodyProperty{
				Name:        name,
				Type:        schemaType(p),
				Description: schemaDescription(p),
				Required:    required[name],
				Enum:        enumValues(p),
				Default:     p["default"],
			})
		}
	}
	return op
}

// opaquePayloadDescription names what the whole-object argument carries,
// including a schema reference when the payload was one.
func opaquePayloadDescription(msg messageDecl) string {
	if msg.ref != "" {
		return fmt.Sprintf("whole message payload (schema %s, sent as-is)", msg.ref)
	}
	return "whole message payload (no depth-1 schema properties; sent as-is)"
}

// payloadProperties returns the payload's depth-1 properties; ok is false
// when the payload has none (opaque).
func payloadProperties(payload map[string]interface{}) (props map[string]interface{}, ok bool) {
	if payload == nil {
		return nil, false
	}
	props, _ = payload["properties"].(map[string]interface{})
	return props, len(props) > 0
}

// kindFor maps the AsyncAPI action onto the neutral kind (PROTO-007 §2.2,
// decision D2).
func kindFor(action string) protocol.OperationKind {
	if action == actionReceive {
		return protocol.KindEventSource
	}
	return protocol.KindUnary
}

// parseServers validates the servers map into doc.Servers.
func parseServers(doc *Document, root map[string]interface{}) error {
	servers, _ := root["servers"].(map[string]interface{})
	for _, name := range sortedKeys(servers) {
		raw, ok := servers[name].(map[string]interface{})
		if !ok {
			return errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: server %q is not an object", name))
		}
		host, _ := raw["host"].(string)
		pathname, _ := raw["pathname"].(string)
		proto, _ := raw["protocol"].(string)
		url := host + pathname
		if url == "" {
			return errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: server %q declares no host; it cannot be selected for execution", name))
		}
		si := serverInfo{Name: name, URL: url, Protocol: proto}
		if b, present := raw["bindings"].(map[string]interface{}); present {
			si.Bindings = canonicalJSON(b)
		}
		doc.Servers[name] = si
	}
	return nil
}

// resolveChannels resolves the channels map (references and all), stamping
// channel and message identities so operation references can find them.
func resolveChannels(root map[string]interface{}) (map[string]channelDecl, error) {
	channelsRaw, _ := root["channels"].(map[string]interface{})
	out := make(map[string]channelDecl, len(channelsRaw))
	for _, key := range sortedKeys(channelsRaw) {
		raw, err := deref(root, channelsRaw[key])
		if err != nil {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: channel %q: %v", key, err))
		}
		ch, ok := raw.(map[string]interface{})
		if !ok {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: channel %q is not an object", key))
		}
		ch[stampChannelKey] = key
		// Stamp message identities declared under the channel.
		if msgs, ok := ch["messages"].(map[string]interface{}); ok {
			for _, mkey := range sortedKeys(msgs) {
				if mObj, err := deref(root, msgs[mkey]); err == nil {
					if m, ok := mObj.(map[string]interface{}); ok {
						m[stampMessageKey] = mkey
					}
				}
			}
		}
		// Stamp component message identities too.
		if components, ok := root["components"].(map[string]interface{}); ok {
			if msgs, ok := components["messages"].(map[string]interface{}); ok {
				for _, mkey := range sortedKeys(msgs) {
					if mObj, err := deref(root, msgs[mkey]); err == nil {
						if m, ok := mObj.(map[string]interface{}); ok {
							m[stampMessageKey] = mkey
						}
					}
				}
			}
		}
		decl := channelDecl{key: key}
		decl.address, _ = ch["address"].(string)
		if decl.address == "" {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: channel %q declares no address; the address is the Kafka topic ground truth (PROTO-007 §2.2)", key))
		}
		if params, ok := ch["parameters"].(map[string]interface{}); ok {
			decl.params = params
		}
		if b, present := ch["bindings"].(map[string]interface{}); present {
			decl.binding = canonicalJSON(b)
		}
		out[key] = decl
	}
	return out, nil
}

// resolveOperation resolves one operation: action validation, channel
// reference, and message expansion (one decl entry per message, §2.1).
func resolveOperation(root map[string]interface{}, key string, raw map[string]interface{}, channels map[string]channelDecl) (*operationDecl, error) {
	decl := &operationDecl{key: key}
	decl.id, _ = raw["operationId"].(string)
	decl.summary, _ = raw["summary"].(string)
	decl.desc, _ = raw["description"].(string)

	decl.action, _ = raw["action"].(string)
	if decl.action != actionSend && decl.action != actionReceive {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q has invalid action %q (expected send or receive; AsyncAPI 2.x publish/subscribe actions are not 3.x)", key, decl.action))
	}

	// Channel reference: required, resolvable, present in channels.
	chRef := raw["channel"]
	if chRef == nil {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q references no channel (PROTO-007 §9.2: operations without channels fail discovery)", key))
	}
	chObj, err := deref(root, chRef)
	if err != nil {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q channel: %v", key, err))
	}
	chMap, ok := chObj.(map[string]interface{})
	if !ok {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q channel reference does not resolve to a declared channel", key))
	}
	chName, _ := chMap[stampChannelKey].(string)
	if chName == "" {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q references channel %q which is not declared", key, sourceRef(chRef)))
	}
	ch, ok := channels[chName]
	if !ok {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q references channel %q which is not declared", key, chName))
	}
	decl.channel = ch

	// Messages: required, resolvable; one decl per message (§2.1/§9.2).
	msgsRaw, ok := raw["messages"].([]interface{})
	if !ok || len(msgsRaw) == 0 {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q references no messages (PROTO-007 §9.2)", key))
	}
	for i, m := range msgsRaw {
		mObj, err := deref(root, m)
		if err != nil {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: operation %q message %d: %v", key, i, err))
		}
		msgMap, ok := mObj.(map[string]interface{})
		if !ok {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: operation %q message %d is not an object", key, i))
		}
		msg, err := resolveMessage(root, msgMap)
		if err != nil {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: operation %q message %d: %v", key, i, err))
		}
		decl.messages = append(decl.messages, msg)
	}

	// Tags, security, reply, bindings: preserved facts.
	decl.tags = tagNames(raw["tags"])
	if sec, present := raw["security"].([]interface{}); present {
		decl.security = canonicalJSON(sec)
	}
	if reply, present := raw["reply"]; present && reply != nil {
		decl.reply = canonicalJSON(reply)
	}
	if b, present := raw["bindings"].(map[string]interface{}); present {
		decl.binding = canonicalJSON(b)
	}
	return decl, nil
}

// sourceRef extracts the raw $ref of a reference object for error messages.
func sourceRef(v interface{}) string {
	if m, ok := v.(map[string]interface{}); ok {
		if r, ok := m["$ref"].(string); ok {
			return r
		}
	}
	return "<inline>"
}

// resolveMessage extracts the message facts the adapter maps (headers
// schema, payload schema, kafka binding).
func resolveMessage(root map[string]interface{}, msg map[string]interface{}) (messageDecl, error) {
	var out messageDecl
	if id, ok := msg[stampMessageKey].(string); ok && id != "" {
		out.id = id
	} else {
		// Inline message objects (no declared identity): a stable derived
		// identity. AsyncAPI messages reached via channel/components refs
		// always carry their key from the resolveChannels stamping.
		out.id = "inline-message"
	}
	if h := msg["headers"]; h != nil {
		resolved, err := deref(root, h)
		if err != nil {
			return out, fmt.Errorf("headers: %w", err)
		}
		if m, ok := resolved.(map[string]interface{}); ok {
			out.headers = m
		}
	}
	if p := msg["payload"]; p != nil {
		resolved, err := deref(root, p)
		if err != nil {
			return out, fmt.Errorf("payload: %w", err)
		}
		if m, ok := resolved.(map[string]interface{}); ok {
			out.payload = m
		}
		if ref, isRef := p.(map[string]interface{})["$ref"].(string); isRef {
			// A bare payload reference: preserved as a source fact (§2.2),
			// the referenced schema still resolves for depth-1 mapping.
			out.ref = ref
		}
	}
	if b, present := msg["bindings"].(map[string]interface{}); present {
		out.binding = canonicalJSON(b)
	}
	return out, nil
}

// deref resolves in-document JSON references ($ref "#/..."). An object
// carrying a $ref is replaced by its target. Unresolved and external
// references fail (PROTO-007 §9.2: no partial sets). root may be nil: the
// walk never needs it because targets are looked up from the reference
// path itself against the STAMPED tree — pass the real root for pointer
// walking.
func deref(root map[string]interface{}, value interface{}) (interface{}, error) {
	return derefDepth(root, value, 0)
}

func derefDepth(root map[string]interface{}, value interface{}, depth int) (interface{}, error) {
	if depth > 16 {
		return nil, fmt.Errorf("$ref chain exceeds 16 hops (reference cycle)")
	}
	obj, ok := value.(map[string]interface{})
	if !ok {
		return value, nil
	}
	ref, isRef := obj["$ref"].(string)
	if !isRef {
		return obj, nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("references %q: only in-document #/ references are supported", ref)
	}
	target, err := jsonPointer(root, ref)
	if err != nil {
		return nil, fmt.Errorf("unresolved $ref %q", ref)
	}
	return derefDepth(root, target, depth+1)
}

// jsonPointer walks a JSON pointer ("#/channels/userSignedup") through the
// decoded document.
func jsonPointer(root map[string]interface{}, ptr string) (interface{}, error) {
	if root == nil {
		return nil, fmt.Errorf("no document root to walk %q", ptr)
	}
	var cur interface{} = root
	for _, seg := range strings.Split(strings.TrimPrefix(ptr, "#/"), "/") {
		seg = strings.ReplaceAll(seg, "~1", "/")
		seg = strings.ReplaceAll(seg, "~0", "~")
		switch node := cur.(type) {
		case map[string]interface{}:
			next, ok := node[seg]
			if !ok {
				return nil, fmt.Errorf("missing key %q in %q", seg, ptr)
			}
			cur = next
		case []interface{}:
			idx := -1
			if _, err := fmt.Sscanf(seg, "%d", &idx); err != nil || idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf("invalid array index %q in %q", seg, ptr)
			}
			cur = node[idx]
		default:
			return nil, fmt.Errorf("cannot descend into %q", ptr)
		}
	}
	return cur, nil
}

// ─── Discoverer ───────────────────────────────────────────────────────────

// Discover implements protocol.Discoverer (PROTO-007 §2.1): it ingests an
// AsyncAPI 3.x document from uri (a filesystem path or an http(s) URL —
// file first) and returns the operations it exposes. Discovery never
// contacts a broker.
func (c *Client) Discover(ctx context.Context, uri string) ([]*protocol.Operation, error) {
	data, err := loadDocument(ctx, c.httpClient, uri)
	if err != nil {
		return nil, err
	}
	doc, err := ParseDocument(uri, data)
	if err != nil {
		return nil, err
	}

	// Server selection (§2.2): explicit option wins; a single server is
	// deterministic; two or more require an explicit choice — never a
	// guess, never a dial.
	sel := c.server
	if sel == "" && len(doc.Servers) == 1 {
		for name := range doc.Servers {
			sel = name
		}
	}
	if sel != "" {
		si, ok := doc.Servers[sel]
		if !ok {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: selected server %q is not declared by the document (declared: %s)",
				sel, strings.Join(sortedKeys(doc.Servers), ", ")))
		}
		doc = stampServer(doc, si)
	} else if len(doc.Servers) > 1 {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: document declares %d servers (%s); select one explicitly instead of guessing (PROTO-007 §2.2)",
			len(doc.Servers), strings.Join(sortedKeys(doc.Servers), ", ")))
	}

	c.docMu.Lock()
	c.doc = doc
	c.ops = doc.Ops
	c.byID = make(map[string]*protocol.Operation, len(doc.Ops))
	for _, op := range doc.Ops {
		c.byID[op.ID] = op
	}
	c.docMu.Unlock()
	return doc.Ops, nil
}

// stampServer copies the selected server's identity onto every operation
// (PROTO-007 §2.2: server identity is runtime ground truth).
func stampServer(doc *Document, si serverInfo) *Document {
	for _, op := range doc.Ops {
		op.Source[SourceServerName] = si.Name
		op.Source[SourceServerURL] = si.URL
		op.Source[SourceServerProtocol] = si.Protocol
		if si.Bindings != "" {
			op.Source[SourceServerBinding] = si.Bindings
		}
	}
	return doc
}

// loadDocument reads a discovery document from disk or over http(s); a path
// that exists on disk always wins (deterministic, testable without a
// server). JSON and YAML bodies are accepted; YAML is bridged to JSON
// through a generic map decode (the same pattern pkg/openapi uses for
// Swagger 2.0 documents).
func loadDocument(ctx context.Context, hc httpClient, uri string) ([]byte, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, errors.New(errors.CodeValidation, "asyncapi: discovery URI is empty")
	}
	path := strings.TrimPrefix(uri, "file://")
	if _, statErr := os.Stat(path); statErr == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeBadRequest, "asyncapi: read AsyncAPI document")
		}
		return data, nil
	}
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return nil, errors.New(errors.CodeNotFound, fmt.Sprintf(
			"asyncapi: document source %q not found on disk and not an http(s) URL", uri))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeBadRequest, "asyncapi: build discovery request")
	}
	resp, err := hc.Do(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "asyncapi: document fetch timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "asyncapi: document fetch failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest,
			"asyncapi: document fetch was rejected")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "asyncapi: read document body")
	}
	return data, nil
}

// decodeDocument decodes a JSON or YAML document body into a generic map.
func decodeDocument(data []byte) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasPrefix(trimmed, "{") {
		if m, err := yamlToMap(data); err == nil {
			return m, nil
		}
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: document is neither valid JSON nor YAML: %v", err))
	}
	return root, nil
}

// ─── small helpers ────────────────────────────────────────────────────────

// canonicalJSON marshals a value deterministically (encoding/json sorts map
// keys), so Source values are stable across discoveries.
func canonicalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// schemaType maps a JSON-schema type onto the neutral type vocabulary;
// absent/unknown types fall back to "string" (the safe default).
func schemaType(schema map[string]interface{}) string {
	if schema == nil {
		return "string"
	}
	if t, ok := schema["type"].(string); ok && t != "" {
		return t
	}
	if _, isRef := schema["$ref"]; isRef {
		return "object" // referenced schemas are objects from the neutral view
	}
	return "string"
}

func schemaDescription(schema map[string]interface{}) string {
	if schema == nil {
		return ""
	}
	d, _ := schema["description"].(string)
	return d
}

// paramSchemaType maps a channel-address parameter onto the neutral type.
// AsyncAPI 3.x parameters carry their schema nested under "schema".
func paramSchemaType(p map[string]interface{}) string {
	if p == nil {
		return "string"
	}
	if s, ok := p["schema"].(map[string]interface{}); ok {
		return schemaType(s)
	}
	return schemaType(p)
}

func paramDescription(p map[string]interface{}) string {
	if p == nil {
		return ""
	}
	return schemaDescription(p)
}

// requiredSet reads a schema's required[] into a name set.
func requiredSet(schema map[string]interface{}) map[string]bool {
	out := map[string]bool{}
	if schema == nil {
		return out
	}
	req, _ := schema["required"].([]interface{})
	for _, r := range req {
		if name, ok := r.(string); ok {
			out[name] = true
		}
	}
	return out
}

// enumValues reads a schema's enum[] into neutral values.
func enumValues(schema map[string]interface{}) []interface{} {
	if schema == nil {
		return nil
	}
	raw, _ := schema["enum"].([]interface{})
	return raw
}

// tagNames reads AsyncAPI tag objects' names in order.
func tagNames(raw interface{}) []string {
	list, _ := raw.([]interface{})
	out := make([]string, 0, len(list))
	for _, t := range list {
		if m, ok := t.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// sortedKeys returns a generic map's keys in sorted order.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
