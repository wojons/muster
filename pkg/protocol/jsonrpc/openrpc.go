package jsonrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// loadDocument fetches an OpenRPC discovery document. File paths win over
// HTTP: a URI that exists on disk is read directly (deterministic and
// testable without a server); anything else is fetched over http(s) through
// the shared pkg/client (specs/PROTOCOLS.md §3.1 — adapters do not open
// their own HTTP clients for discovery).
func loadDocument(ctx context.Context, c *client.Client, uri string) ([]byte, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, errors.New(errors.CodeValidation, "jsonrpc: discovery URI is empty")
	}

	if _, statErr := os.Stat(uri); statErr == nil {
		data, err := os.ReadFile(uri)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeBadRequest, "jsonrpc: read OpenRPC document")
		}
		return data, nil
	}

	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return nil, errors.New(errors.CodeNotFound, fmt.Sprintf("jsonrpc: discovery source %q not found on disk and not an http(s) URL", uri))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeBadRequest, "jsonrpc: build discovery request")
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "jsonrpc: discovery fetch timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "jsonrpc: discovery fetch failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest, "jsonrpc: discovery fetch rejected")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "jsonrpc: read discovery document")
	}
	return data, nil
}

// openrpcDocument is the subset of the OpenRPC 1.x schema this adapter
// consumes. Content descriptors carry the parameter type/required/
// description vocabulary mapped onto the neutral model.
type openrpcDocument struct {
	OpenRPC string          `json:"openrpc"`
	Info    openrpcInfo     `json:"info"`
	Servers []openrpcServer `json:"servers"`
	Methods []openrpcMethod `json:"methods"`
}

type openrpcInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type openrpcServer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type openrpcMethod struct {
	Name        string            `json:"name"`
	Tags        []openrpcTag      `json:"tags"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Params      []openrpcParam    `json:"params"`
	Result      *openrpcContent   `json:"result"`
	Errors      []openrpcRPCError `json:"errors"`
}

type openrpcTag struct {
	Name string `json:"name"`
}

// openrpcParam is an OpenRPC content descriptor for a method parameter.
type openrpcParam struct {
	openrpcContent
}

// openrpcContent is an OpenRPC content descriptor: a name plus a JSON
// schema. Required is signaled by the descriptor's own "required" field.
type openrpcContent struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Required    bool                   `json:"required"`
	Schema      map[string]interface{} `json:"schema"`
}

type openrpcRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// schemaType extracts the JSON-schema type string from a content
// descriptor's schema, defaulting to "string" like pkg/mcp's converters do.
func (c openrpcContent) schemaType() string {
	if c.Schema == nil {
		return "string"
	}
	if t, ok := c.Schema["type"].(string); ok && t != "" {
		return t
	}
	return "string"
}

// operations maps the document's OpenRPC methods onto protocol-neutral
// operations (stage 2 mapping, specs/PROTOCOLS.md §3.2). OpenRPC params are
// positional-or-named; this adapter models each param as a named Parameter
// (In: "payload") because the CLI/MCP surface is flag/JSON-argument shaped.
// Kind is always KindUnary: JSON-RPC 2.0 is request/response only, and the
// empty result (`{"result": null}` for notifications) is still a single
// response.
func (d *openrpcDocument) operations() []*protocol.Operation {
	serverURL := ""
	if len(d.Servers) > 0 && d.Servers[0].URL != "" {
		serverURL = d.Servers[0].URL
	}

	ops := make([]*protocol.Operation, 0, len(d.Methods))
	for _, m := range d.Methods {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			continue
		}

		params := make([]protocol.Parameter, 0, len(m.Params))
		for _, p := range m.Params {
			params = append(params, protocol.Parameter{
				Name:        p.Name,
				In:          "payload",
				Type:        p.schemaType(),
				Required:    p.Required,
				Description: p.Description,
			})
		}

		description := m.Description
		if description == "" && m.Result != nil && m.Result.Description != "" {
			description = fmt.Sprintf("Returns: %s", m.Result.Description)
		}

		op := &protocol.Operation{
			ID:          normalizeID(name),
			Protocol:    protocol.ProtocolJSONRPC,
			Kind:        protocol.KindUnary,
			Tags:        tagsToStrings(m.Tags),
			Summary:     m.Summary,
			Description: description,
			Parameters:  params,
			Source: map[string]string{
				"full_method": name,
			},
		}
		if serverURL != "" {
			op.Source[sourceKeyServer] = serverURL
		}
		ops = append(ops, op)
	}
	return ops
}

// tagsToStrings flattens OpenRPC tag objects to their names.
func tagsToStrings(tags []openrpcTag) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t.Name != "" {
			out = append(out, t.Name)
		}
	}
	return out
}

// defaultIDSource returns the package's default JSON-RPC request-ID source:
// monotonically increasing numeric IDs (JSON-RPC 2.0 allows string, number,
// or null; numbers are simplest to correlate and per-Client unique).
func defaultIDSource() func() interface{} {
	var counter uint64
	return func() interface{} {
		return atomic.AddUint64(&counter, 1)
	}
}

// idsMatch reports whether a response's ID matches the request's ID
// (JSON-RPC 2.0: the server MUST reply with the same id). Go decodes JSON
// numbers as float64 while the client sends uint64, so numeric comparison
// is normalized. A null response ID cannot be correlated; it is treated as
// matching only when nothing was sent (notifications), otherwise it is a
// mismatch.
func idsMatch(respID, sentID interface{}) bool {
	if respID == nil || sentID == nil {
		return respID == nil && sentID == nil
	}
	switch s := sentID.(type) {
	case uint64:
		if r, ok := respID.(float64); ok {
			return r == float64(s)
		}
	case string:
		r, ok := respID.(string)
		return ok && r == s
	}
	return fmt.Sprint(sentID) == fmt.Sprint(respID)
}

// stringsReplaceAll is a tiny local alias to keep the file's imports flat.
func stringsReplaceAll(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}
