package soap

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// operationID joins the service (or interface) and operation name with a
// dash — the same service-method shape the gRPC adapter uses, so command
// name normalization behaves identically.
func operationID(service, op string) string {
	if service == "" {
		return op
	}
	return service + "-" + op
}

// Operations maps every parsed WSDL operation onto the protocol-neutral
// model (stage 2). Each input part / input element becomes typed
// RequestBodyProperty entries at depth 1; required = minOccurs != 0,
// repeated = maxOccurs "unbounded" (or a numeric value > 1), nillable stays
// on the property's description contract via Type semantics (nullable XSD
// types are emitted as their JSON type with the nillable fact preserved in
// the source stamp's element record).
func (d *WSDLDoc) Operations() ([]*protocol.Operation, error) {
	ops := make([]*protocol.Operation, 0, len(d.Ops))
	seen := map[string]bool{}
	for _, wop := range d.Ops {
		id := operationID(d.ServiceName, wop.Name)
		if seen[id] {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"soap: WSDL declares duplicate operation identity %q; resolve the document before discovery", id))
		}
		seen[id] = true

		op := &protocol.Operation{
			ID:       id,
			Protocol: protocol.ProtocolSOAP,
			Kind:     protocol.KindUnary,
			Summary:  fmt.Sprintf("Invoke SOAP operation %s", wop.Name),
			Source: map[string]string{
				SourceService:     d.ServiceName,
				SourceOperation:   wop.Name,
				SourceSOAPAction:  wop.SOAPAction,
				SourceSOAPVersion: string(d.Version),
				SourceStyle:       wop.Style,
				SourceEndpoint:    d.Endpoint,
				SourceWSDL:        d.SourceURI,
			},
		}
		for _, part := range wop.InputParts {
			elemQName := part.Element
			if elemQName == "" && part.Type != "" {
				// rpc-style / type-reference part: the part itself is the
				// payload holder and its children (when the type is a known
				// complexType) are the message fields.
				if fields, ok := d.types[part.Type]; ok {
					op.RequestBody = append(op.RequestBody, d.fieldsToProperties(fields, part.Name)...)
				}
				continue
			}
			decl, ok := d.elements[elemQName]
			if !ok {
				// The element is not declared in the document's schemas:
				// carry the part as an opaque object so the operation still
				// ingests with an honest shape (unknown, not silently empty).
				op.RequestBody = append(op.RequestBody, protocol.RequestBodyProperty{
					Name:        partName(part),
					Type:        "object",
					Description: fmt.Sprintf("element %s (not declared in the WSDL types; sent as-is)", elemQName),
				})
				continue
			}
			if len(decl.Fields) > 0 {
				op.RequestBody = append(op.RequestBody, d.fieldsToProperties(decl.Fields, "")...)
			} else if decl.Type == "" {
				op.RequestBody = append(op.RequestBody, protocol.RequestBodyProperty{
					Name: partName(part), Type: "object",
					Description: fmt.Sprintf("element %s (no type information)", decl.Name),
				})
			} else {
				op.RequestBody = append(op.RequestBody, protocol.RequestBodyProperty{
					Name: partName(part), Type: xsdJSONType(xsdLocalType(decl.Type)),
				})
			}
			if op.Source[SourceInputElem] == "" {
				op.Source[SourceInputElem] = elemQName
			}
		}
		if out := wop.OutputParts; len(out) > 0 && out[0].Element != "" {
			op.Source[SourceOutputElem] = out[0].Element
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// SourceURI is set by the discoverer on the parsed document.
// (Declared here as a field through the struct literal in wsdl.go.)

// partName names a part-derived property: the part's own name, or a
// deterministic fallback for WSDL 2.0 element references that have none.
func partName(p Part) string {
	if p.Name != "" {
		return p.Name
	}
	local := p.Element
	if i := strings.LastIndex(local, "}"); i >= 0 {
		local = local[i+1:]
	}
	return local
}

// fieldsToProperties maps XSD depth-1 fields onto neutral request-body
// properties, translating the XSD cardinality/nillability facts:
//
//	xsd required  → Required=true   (minOccurs absent or ≥1)
//	xsd repeated  → Repeated=true, Type="array", ItemType=<item JSON type>
//	                (maxOccurs "unbounded" or a numeric value > 1)
//	xsd nillable  → the JSON type is unchanged (JSON null is always a legal
//	                value); the fact is preserved for the executor.
func (d *WSDLDoc) fieldsToProperties(fields []XSDField, prefix string) []protocol.RequestBodyProperty {
	props := make([]protocol.RequestBodyProperty, 0, len(fields))
	for _, f := range fields {
		jsonType := xsdJSONType(f.Type)
		prop := protocol.RequestBodyProperty{
			Name:     f.Name,
			Type:     jsonType,
			Required: f.MinOccurs == "" || f.MinOccurs == "1",
		}
		if prefix != "" {
			prop.Name = prefix + "." + f.Name
		}
		if repeated(f.MaxOccurs) {
			prop.Repeated = true
			prop.Type = "array"
			prop.ItemType = jsonType
		}
		if f.Nillable {
			prop.Description = "nillable: the element may carry xsi:nil"
		}
		props = append(props, prop)
	}
	return props
}

// repeated reports whether a maxOccurs value means "more than one".
func repeated(maxOccurs string) bool {
	switch maxOccurs {
	case "", "0", "1":
		return false
	case "unbounded":
		return true
	default:
		// A numeric bound > 1.
		return maxOccurs != "1"
	}
}

// xsdJSONType maps an XSD built-in type local name onto the JSON-schema-style
// type vocabulary pkg/mcp and pkg/generator already use. Unknown or
// schema-namespace-less types map to "string" (the safe default for CLI text
// input); derived types (limits, tokens) are strings on the wire too.
func xsdJSONType(xsdType string) string {
	switch xsdType {
	case "int", "integer", "long", "short", "byte", "nonNegativeInteger",
		"positiveInteger", "nonPositiveInteger", "negativeInteger",
		"unsignedLong", "unsignedInt", "unsignedShort", "unsignedByte":
		return "integer"
	case "float", "double", "decimal":
		return "number"
	case "boolean":
		return "boolean"
	case "dateTime", "date", "time", "duration", "gYear", "gYearMonth",
		"gMonth", "gMonthDay", "gDay":
		return "string" // lexical XML forms; no implicit JSON date rewrite
	default:
		// string, normalizedString, token, base64Binary, hexBinary, anyURI,
		// QName, NOTATION, ID/IDREF families, and unknown/complex types.
		return "string"
	}
}

// Discover implements protocol.Discoverer: it ingests a WSDL 1.1 or 2.0
// document from uri (a filesystem path or an http(s) URL — file first) and
// returns the operations it exposes. Discovery never contacts the endpoint.
func (c *Client) Discover(ctx context.Context, uri string) ([]*protocol.Operation, error) {
	data, err := loadDocument(ctx, c.httpClient, uri)
	if err != nil {
		return nil, err
	}
	doc, err := parseWSDL(data)
	if err != nil {
		return nil, err
	}
	doc.SourceURI = uri
	ops, err := doc.Operations()
	if err != nil {
		return nil, err
	}
	c.doc = doc
	c.ops = ops
	return ops, nil
}

// loadDocument reads a discovery document from disk or over http(s); a path
// that exists on disk always wins (deterministic, testable without a server).
func loadDocument(ctx context.Context, hc httpClient, uri string) ([]byte, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, errors.New(errors.CodeValidation, "soap: discovery URI is empty")
	}
	path := strings.TrimPrefix(uri, "file://")
	if _, statErr := os.Stat(path); statErr == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeBadRequest, "soap: read WSDL document")
		}
		return data, nil
	}
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return nil, errors.New(errors.CodeNotFound, fmt.Sprintf(
			"soap: WSDL source %q not found on disk and not an http(s) URL", uri))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeBadRequest, "soap: build discovery request")
	}
	resp, err := hc.Do(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Wrap(err, errors.CodeTimeout, "soap: WSDL fetch timed out")
		}
		return nil, errors.Wrap(err, errors.CodeGeneric, "soap: WSDL fetch failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeBadRequest,
			"soap: WSDL fetch was rejected")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "soap: read WSDL body")
	}
	return data, nil
}
