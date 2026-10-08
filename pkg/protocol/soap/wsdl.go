// Package soap implements the SOAP 1.1 / 1.2 (via WSDL 1.1 / 2.0) protocol
// adapter for Muster's protocol adapter seam (specs/PROTOCOLS.md, matrix
// row 8, board row PROTO-008).
//
// Discovery (stage 1) parses a WSDL 1.1 <definitions> or WSDL 2.0
// <description> document with a light encoding/xml parser (no WSDL codegen
// dependency) and maps each portType/interface operation onto the
// protocol-neutral Operation model, carrying typed message parts, optional
// (minOccurs=0 / nillable) and repeated (maxOccurs="unbounded") element facts.
//
// Execution (stage 4) builds a version-appropriate SOAP envelope
// (text/xml + SOAPAction header for 1.1; application/soap+xml + action
// content-type parameter for 1.2; optional WS-Addressing 1.0 header block),
// posts it over pkg/client + pkg/request.Builder, and decodes the response
// body as JSON by default or raw XML on demand. A SOAP Fault (1.1 or 1.2
// shape) maps onto pkg/errors AppError codes with the fault code and detail
// preserved.
//
// OUT OF SCOPE (documented, not silently ignored — see the implementation
// spec §6): WS-Security (WS-SecurityPolicy, UsernameToken, X.509 signing)
// and MTOM/XOP binary attachments. The adapter rejects neither by surprise:
// a WSDL whose binding declares policies it cannot honor still ingests, and
// the docs state the limitation.
package soap

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
)

// SOAPVersion names the SOAP envelope version the adapter speaks.
type SOAPVersion string

const (
	// SOAP11 is the SOAP 1.1 envelope (POST body text/xml, SOAPAction header).
	SOAP11 SOAPVersion = "1.1"
	// SOAP12 is the SOAP 1.2 envelope (POST body application/soap+xml with an
	// action parameter).
	SOAP12 SOAPVersion = "1.2"
)

// Envelope namespace URIs.
const (
	EnvelopeNS11 = "http://schemas.xmlsoap.org/soap/envelope/"
	EnvelopeNS12 = "http://www.w3.org/2003/05/soap-envelope"
	// AddressingNS is the WS-Addressing 1.0 (2005/08) namespace.
	AddressingNS = "http://www.w3.org/2005/08/addressing"
)

// Source-stamp keys stamped into Operation.Source (specs/PROTOCOLS.md §3.2 —
// stage-3 consumers never interpret these; the adapter's own executor does).
const (
	SourceService     = "service"       // service/portType/interface name
	SourceOperation   = "operation"     // operation name as declared
	SourceSOAPAction  = "soap_action"   // the binding's SOAPAction / soapAction
	SourceSOAPVersion = "soap_version"  // "1.1" or "1.2"
	SourceStyle       = "style"         // "document" (default) or "rpc"
	SourceEndpoint    = "endpoint"      // the wsdl:service address
	SourceWSDL        = "wsdl_uri"      // the discovery URI
	SourceInputElem   = "input_element" // resolved input element QName
	SourceOutputElem  = "output_element"
)

// ElementDecl is one global XSD element declaration found in the WSDL's
// types section.
type ElementDecl struct {
	Name      string
	Type      string // QName of the declared type ("" when inline complexType)
	Nillable  bool
	MinOccurs string
	MaxOccurs string
	Fields    []XSDField // depth-1 children when the type is inline complexType
}

// XSDField is one depth-1 element of a complexType — the typed message part
// the neutral model maps onto RequestBodyProperty.
type XSDField struct {
	Name      string
	Type      string // XSD type local name ("string", "dateTime", ...)
	Nillable  bool
	MinOccurs string
	MaxOccurs string
}

// Part is one wsdl:message part (1.1) or interface operation input/output
// element reference (2.0).
type Part struct {
	Name    string // part name (1.1); "" for a 2.0 element reference
	Element string // resolved QName "{ns}Local" of the part element
	Type    string // resolved type QName (rpc style / simple part)
}

// WSDLOperation is one operation assembled from the document.
type WSDLOperation struct {
	Name        string
	SOAPAction  string
	Style       string // "document" or "rpc"
	InputParts  []Part
	OutputParts []Part
}

// WSDLDoc is the parsed discovery document, version-agnostic.
type WSDLDoc struct {
	Version         SOAPVersion
	TargetNamespace string
	ServiceName     string
	Endpoint        string
	Style           string // document-wide default style
	Ops             []*WSDLOperation

	// SourceURI is the discovery URI the document was loaded from.
	SourceURI string

	elements map[string]*ElementDecl // resolved QName → global element decl
	types    map[string][]XSDField   // resolved QName → complexType children
}

// xmlNode is a minimal decoded XML node: local name, attribute map
// (local-name keyed), child nodes and character data. Namespaces are tracked
// during parsing so QName attributes can be resolved to "{uri}Local" form.
type xmlNode struct {
	Name     string // local name
	NS       string // resolved namespace URI ("" when none)
	Attrs    map[string]string
	Text     string
	Children []*xmlNode
}

// child returns the first child with the given local name (any namespace
// unless ns is non-empty — WSDL parsers conventionally match by local name
// across the wsdl/soap/xsd namespaces whose URIs vary by version).
func (n *xmlNode) child(name string) *xmlNode {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// children returns every child with the given local name.
func (n *xmlNode) children(name string) []*xmlNode {
	var out []*xmlNode
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// attr returns the attribute with the given local name.
func (n *xmlNode) attr(name string) string { return n.Attrs[name] }

// parseXMLTree decodes a whole XML document into a node tree, resolving
// namespace prefixes of element names and QName-valued attributes against
// in-scope xmlns declarations.
func parseXMLTree(data []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false // tolerate the loose XML real WSDLs ship

	// scope is the in-scope prefix → URI map (one entry per nesting level).
	type scopeT map[string]string
	scopes := []scopeT{{}}
	current := func(prefix string) string {
		for i := len(scopes) - 1; i >= 0; i-- {
			if uri, ok := scopes[i][prefix]; ok {
				return uri
			}
		}
		return ""
	}

	var root *xmlNode
	var stack []*xmlNode

	for {
		tok, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
				break
			}
			return nil, errors.Wrap(err, errors.CodeValidation, "soap: parse XML document")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			ns := map[string]string{}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" {
					ns[a.Name.Local] = a.Value
				} else if a.Name.Space == "" && a.Name.Local == "xmlns" {
					ns[""] = a.Value
				}
			}
			scopes = append(scopes, ns)

			attrs := map[string]string{}
			uri := current(t.Name.Space)
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				key := a.Name.Local
				if a.Name.Space != "" && a.Name.Space != "xmlns" {
					// Keep namespaced attributes under "uri:local" only when
					// meaningful (xml:lang etc.); plain local wins for WSDL.
					if res := current(a.Name.Space); res != "" {
						key = res + ":" + a.Name.Local
					}
				}
				attrs[key] = a.Value
			}

			node := &xmlNode{Name: t.Name.Local, NS: uri, Attrs: attrs}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			} else if root == nil {
				root = node
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				scopes = scopes[:len(scopes)-1]
			}
		}
	}
	if root == nil {
		return nil, errors.New(errors.CodeValidation, "soap: document contains no XML root element")
	}
	return root, nil
}

// parseWSDL parses a WSDL 1.1 or 2.0 document into a WSDLDoc.
func parseWSDL(data []byte) (*WSDLDoc, error) {
	root, err := parseXMLTree(data)
	if err != nil {
		return nil, err
	}
	switch root.Name {
	case "definitions":
		return parseWSDL11(root)
	case "description":
		return parseWSDL20(root)
	default:
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"soap: document root %q is neither a WSDL 1.1 <definitions> nor a WSDL 2.0 <description>", root.Name))
	}
}

// parseWSDL11 interprets a WSDL 1.1 <definitions> document.
func parseWSDL11(root *xmlNode) (*WSDLDoc, error) {
	doc := &WSDLDoc{
		Version:         SOAP11,
		TargetNamespace: root.attr("targetNamespace"),
		Style:           "document",
		elements:        map[string]*ElementDecl{},
		types:           map[string][]XSDField{},
	}
	if doc.TargetNamespace == "" {
		return nil, errors.New(errors.CodeValidation, "soap: WSDL 1.1 document has no targetNamespace")
	}

	// Types: collect global element declarations and complex types from every
	// inline/embedded schema.
	for _, types := range root.children("types") {
		for _, schema := range types.children("schema") {
			collectSchema(schema, doc)
		}
	}

	// Messages: name → parts.
	messages := map[string][]Part{} // resolved QName of the message → parts
	for _, msg := range root.children("message") {
		name := msg.attr("name")
		if name == "" {
			continue
		}
		var parts []Part
		for _, p := range msg.children("part") {
			part := Part{Name: p.attr("name")}
			if e := p.attr("element"); e != "" {
				part.Element = resolveQNameIn(p, e, doc.TargetNamespace)
			}
			if tp := p.attr("type"); tp != "" {
				part.Type = resolveQNameIn(p, tp, doc.TargetNamespace)
			}
			parts = append(parts, part)
		}
		messages[resolveQNameIn(msg, name, doc.TargetNamespace)] = parts
	}

	// Bindings: operation name → SOAPAction + style (binding-level default,
	// overridden per operation).
	type binding struct {
		soapAction string
		style      string
	}
	bindings := map[string]binding{}
	bindingStyle := "document"
	for _, b := range root.children("binding") {
		if soapB := b.child("binding"); soapB != nil {
			if st := soapB.attr("style"); st != "" {
				bindingStyle = st
			}
		}
		for _, op := range b.children("operation") {
			bop := binding{style: bindingStyle}
			if soapOp := op.child("operation"); soapOp != nil {
				bop.soapAction = soapOp.attr("soapAction")
				if st := soapOp.attr("style"); st != "" {
					bop.style = st
				}
			}
			bindings[op.attr("name")] = bop
		}
	}
	doc.Style = bindingStyle

	// Service: the first port's address is the endpoint ground truth.
	if svc := root.child("service"); svc != nil {
		doc.ServiceName = svc.attr("name")
		if port := svc.child("port"); port != nil {
			if addr := port.child("address"); addr != nil {
				doc.Endpoint = addr.attr("location")
			}
		}
	}

	// Port type operations.
	portType := root.child("portType")
	if portType == nil {
		return nil, errors.New(errors.CodeValidation, "soap: WSDL 1.1 document declares no portType")
	}
	for _, op := range portType.children("operation") {
		name := op.attr("name")
		if name == "" {
			continue
		}
		wop := &WSDLOperation{Name: name}
		if b, ok := bindings[name]; ok {
			wop.SOAPAction = b.soapAction
			wop.Style = b.style
		} else {
			wop.Style = doc.Style
		}
		if in := op.child("input"); in != nil {
			wop.InputParts = messages[resolveQNameIn(in, in.attr("message"), doc.TargetNamespace)]
		}
		if out := op.child("output"); out != nil {
			wop.OutputParts = messages[resolveQNameIn(out, out.attr("message"), doc.TargetNamespace)]
		}
		doc.Ops = append(doc.Ops, wop)
	}
	if len(doc.Ops) == 0 {
		return nil, errors.New(errors.CodeValidation,
			"soap: WSDL 1.1 document exposes no operations (portType declares none)")
	}
	return doc, nil
}

// parseWSDL20 interprets a WSDL 2.0 <description> document (SOAP 1.2 binding).
func parseWSDL20(root *xmlNode) (*WSDLDoc, error) {
	doc := &WSDLDoc{
		Version:         SOAP12,
		TargetNamespace: root.attr("targetNamespace"),
		Style:           "document",
		elements:        map[string]*ElementDecl{},
		types:           map[string][]XSDField{},
	}
	if doc.TargetNamespace == "" {
		return nil, errors.New(errors.CodeValidation, "soap: WSDL 2.0 document has no targetNamespace")
	}

	for _, types := range root.children("types") {
		for _, schema := range types.children("schema") {
			collectSchema(schema, doc)
		}
	}

	type ifaceOp struct {
		input  string // element QName
		output string
	}
	ifaceOps := map[string]ifaceOp{}
	for _, iface := range root.children("interface") {
		for _, op := range iface.children("operation") {
			var io ifaceOp
			if in := op.child("input"); in != nil {
				io.input = resolveQNameIn(in, in.attr("element"), doc.TargetNamespace)
			}
			if out := op.child("output"); out != nil {
				io.output = resolveQNameIn(out, out.attr("element"), doc.TargetNamespace)
			}
			ifaceOps[op.attr("name")] = io
		}
	}

	// SOAP binding: operation ref → soapAction (wsoap:soapAction attribute,
	// matched by local name).
	bindings := map[string]string{}
	bindingIface := ""
	for _, b := range root.children("binding") {
		bindingIface = resolveQNameIn(b, b.attr("interface"), doc.TargetNamespace)
		for _, op := range b.children("operation") {
			ref := resolveQNameIn(op, op.attr("ref"), doc.TargetNamespace)
			// The WSDL 2.0 SOAP binding names the action "wsoap:action"
			// (namespaced attribute, stored as "uri:local" by the parser)
			// but plain soapAction appears in the wild; accept both.
			action := op.attr("soapAction")
			if action == "" {
				for k, v := range op.Attrs {
					if k == "action" || strings.HasSuffix(k, ":action") {
						action = v
						break
					}
				}
			}
			if action != "" {
				local := ref
				if i := strings.LastIndex(ref, "}"); i >= 0 {
					local = ref[i+1:]
				}
				bindings[local] = action
			}
		}
	}

	// Service endpoints.
	if svc := root.child("service"); svc != nil {
		doc.ServiceName = svc.attr("name")
		if ep := svc.child("endpoint"); ep != nil {
			doc.Endpoint = ep.attr("address")
		}
	}

	// Operations come from the bound interface (its name resolves to the
	// interface's QName; match by local name).
	ifaceLocal := bindingIface
	if i := strings.LastIndex(ifaceLocal, "}"); i >= 0 {
		ifaceLocal = ifaceLocal[i+1:]
	}
	for _, iface := range root.children("interface") {
		if iface.attr("name") != ifaceLocal && bindingIface != "" {
			continue
		}
		for _, op := range iface.children("operation") {
			name := op.attr("name")
			if name == "" {
				continue
			}
			wop := &WSDLOperation{Name: name, Style: doc.Style, SOAPAction: bindings[name]}
			if io, ok := ifaceOps[name]; ok {
				if io.input != "" {
					wop.InputParts = []Part{{Element: io.input}}
				}
				if io.output != "" {
					wop.OutputParts = []Part{{Element: io.output}}
				}
			}
			doc.Ops = append(doc.Ops, wop)
		}
	}
	if len(doc.Ops) == 0 {
		return nil, errors.New(errors.CodeValidation,
			"soap: WSDL 2.0 document exposes no operations (interface declares none)")
	}
	return doc, nil
}

// collectSchema gathers global element declarations and named complexTypes
// from one xsd:schema node. Both are keyed by resolved QName
// ({targetNamespace}Local), so cross-schema references resolve.
func collectSchema(schema *xmlNode, doc *WSDLDoc) {
	tns := doc.TargetNamespace
	if t := schema.attr("targetNamespace"); t != "" {
		tns = t
	}
	for _, el := range schema.children("element") {
		name := el.attr("name")
		if name == "" || el.attr("ref") != "" {
			continue // references resolve through their declaration
		}
		decl := &ElementDecl{
			Name:      name,
			Type:      resolveQNameIn(el, el.attr("type"), tns),
			Nillable:  el.attr("nillable") == "true",
			MinOccurs: el.attr("minOccurs"),
			MaxOccurs: el.attr("maxOccurs"),
		}
		if ct := el.child("complexType"); ct != nil {
			decl.Fields = complexTypeFields(ct, tns)
		}
		doc.elements["{"+tns+"}"+name] = decl
	}
	for _, ct := range schema.children("complexType") {
		name := ct.attr("name")
		if name == "" {
			continue
		}
		doc.types["{"+tns+"}"+name] = complexTypeFields(ct, tns)
	}
}

// complexTypeFields returns the depth-1 element children of a complexType
// (through sequence/choice/all groups), carrying the required/nillable and
// repeated cardinality facts the neutral model maps onto CLI flags.
func complexTypeFields(ct *xmlNode, tns string) []XSDField {
	var fields []XSDField
	var walk func(n *xmlNode)
	walk = func(n *xmlNode) {
		switch n.Name {
		case "complexType", "complexContent", "restriction", "extension",
			"sequence", "choice", "all":
			for _, c := range n.Children {
				walk(c)
			}
		case "element":
			name := n.attr("name")
			if name == "" {
				return
			}
			fields = append(fields, XSDField{
				Name:      name,
				Type:      xsdLocalType(resolveQNameIn(n, n.attr("type"), tns)),
				Nillable:  n.attr("nillable") == "true",
				MinOccurs: n.attr("minOccurs"),
				MaxOccurs: n.attr("maxOccurs"),
			})
		}
	}
	walk(ct)
	return fields
}

// xsdLocalType strips a type QName to the XSD local name the mapping table
// keys on ("{http://www.w3.org/2001/XMLSchema}dateTime" → "dateTime"); an
// unresolvable or schema-namespace-less type keeps its local name.
func xsdLocalType(qname string) string {
	if qname == "" {
		return ""
	}
	if strings.HasPrefix(qname, "{") {
		if i := strings.LastIndex(qname, "}"); i >= 0 {
			return qname[i+1:]
		}
	}
	if i := strings.LastIndex(qname, ":"); i >= 0 {
		return qname[i+1:]
	}
	return qname
}

// resolveQNameIn resolves a QName attribute value against the node's raw
// prefix and the document target namespace. The parser does not retain the
// in-scope xmlns maps, so the resolution rule is the pragmatic one real WSDL
// documents satisfy: a prefixed name keeps the prefix's local part and the
// prefix itself is used only when it matches a well-known namespace the
// document declares on the root; anything else resolves to the target
// namespace (the overwhelmingly common case for WSDL-owned QNames).
//
// The XSD namespace is recognized explicitly so xsd:string-type parts map by
// local name via xsdLocalType.
func resolveQNameIn(n *xmlNode, value, targetNamespace string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	prefix, local := "", value
	if i := strings.LastIndex(value, ":"); i >= 0 {
		prefix, local = value[:i], value[i+1:]
	}
	switch prefix {
	case "xsd", "xs":
		return "{http://www.w3.org/2001/XMLSchema}" + local
	case "soap", "soapenv", "env", "wsdl", "wsdlsoap", "wsoap", "whttp", "wsa":
		// Protocol namespaces never own data types; resolve to target ns.
		return "{" + targetNamespace + "}" + local
	case "":
		return "{" + targetNamespace + "}" + local
	default:
		// Unknown prefix (e.g. "tns"): resolve against the target namespace —
		// WSDL QName attributes reference the document's own components.
		return "{" + targetNamespace + "}" + local
	}
}
