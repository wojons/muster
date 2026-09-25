package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/request"
)

// This file is the GraphQL request builder: ONE endpoint, ONE POST, a query
// document plus a variables object (specs/PROTOCOLS.md §2 matrix row 5). The
// REST-style per-path/per-method builder is deliberately not reused — GraphQL
// has no paths, and the document is derived from the schema rather than from a
// URL template.
//
// Argument values are bound as GraphQL variables. The document contains the
// variable DECLARATIONS the call uses and nothing else: no caller value is ever
// string-interpolated into query text, so a value cannot change the document's
// shape.

// Selection limits. They bound the document the builder derives from the
// schema so a wide schema cannot produce an unbounded query.
const (
	// DefaultSelectionDepth is how many object levels the derived selection
	// recurses into (the root field level is depth 1).
	DefaultSelectionDepth = 2
	// DefaultSelectionFields caps how many fields are selected on one level.
	DefaultSelectionFields = 25
)

// SelectionOptions bounds the selection set the builder derives from a schema.
type SelectionOptions struct {
	// MaxDepth is the object recursion depth; 0 means DefaultSelectionDepth.
	MaxDepth int
	// MaxFields caps the fields selected per level; 0 means
	// DefaultSelectionFields.
	MaxFields int
	// Fields restricts the TOP-LEVEL selection to these field names, in the
	// order given — the per-operation selection for a schema with many fields.
	// Empty means every eligible field the schema declares. Field names must
	// exist on the operation's return type and must not require arguments; an
	// unknown name is an error, never a silent omission.
	Fields []string
}

// withDefaults fills unset limits.
func (o SelectionOptions) withDefaults() SelectionOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultSelectionDepth
	}
	if o.MaxFields <= 0 {
		o.MaxFields = DefaultSelectionFields
	}
	return o
}

// Request is one GraphQL-over-HTTP request: a single POST to a single endpoint
// carrying a query document and a variables object.
type Request struct {
	// Endpoint is the URL the request is posted to.
	Endpoint string
	// Query is the GraphQL query document.
	Query string
	// OperationName is the document's operation name, echoed in the body's
	// operationName member.
	OperationName string
	// Variables holds one entry per bound argument (always non-nil: GraphQL
	// clients send an object, and an argument-free call sends {}).
	Variables map[string]interface{}
	// OperationID is the ingested operation this request was built for.
	OperationID string
}

// graphqlBody is the wire shape of a GraphQL-over-HTTP request body.
type graphqlBody struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName,omitempty"`
}

// Body renders the JSON body: {"query": …,"variables": …,"operationName": …}.
func (r *Request) Body() ([]byte, error) {
	body, err := json.Marshal(graphqlBody{
		Query:         r.Query,
		Variables:     r.Variables,
		OperationName: r.OperationName,
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "graphql: encode request body")
	}
	return body, nil
}

// HTTPRequest assembles the single POST this Request describes. Assembly goes
// through pkg/request.Builder so proxy/TLS and header wiring match the rest of
// the HTTP path (specs/PROTOCOLS.md §3.5).
func (r *Request) HTTPRequest(ctx context.Context) (*http.Request, error) {
	if strings.TrimSpace(r.Endpoint) == "" {
		return nil, errors.New(errors.CodeValidation, "graphql: request has no endpoint")
	}
	body, err := r.Body()
	if err != nil {
		return nil, err
	}
	builder := request.NewBuilder(r.Endpoint, "", http.MethodPost)
	builder.JSONBody(string(body))
	builder.SetHeader("Content-Type", ContentTypeJSON)
	req, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "graphql: build HTTP request")
	}
	return req.WithContext(ctx), nil
}

// ContentTypeJSON is the content type every GraphQL-over-HTTP request carries.
const ContentTypeJSON = "application/json"

// Builder turns an ingested operation plus arguments into a Request, using the
// schema it was constructed from.
type Builder struct {
	schema *Schema
	sel    SelectionOptions
}

// NewBuilder creates a request builder over a schema.
func NewBuilder(s *Schema, sel SelectionOptions) (*Builder, error) {
	if s == nil {
		return nil, errors.New(errors.CodeValidation, "graphql: request builder needs a schema")
	}
	if len(s.Types) == 0 {
		return nil, errors.New(errors.CodeValidation, "graphql: request builder needs a schema with at least one type")
	}
	return &Builder{schema: s, sel: sel.withDefaults()}, nil
}

// Schema returns the schema the builder derives documents from.
func (b *Builder) Schema() *Schema { return b.schema }

// Selection returns the builder's (defaulted) selection options.
func (b *Builder) Selection() SelectionOptions { return b.sel }

// Build builds the GraphQL request for one ingested operation.
//
// Arguments are matched to the field's declared arguments by name: a missing
// required argument (non-null without a default) and an unknown argument are
// both errors before any bytes leave the process (specs/PROTOCOLS.md §4.2:
// client-side validation maps to CodeValidation). Omitted optional arguments are
// left out of the document entirely so the server applies its own default.
func (b *Builder) Build(op *protocol.Operation, args map[string]interface{}, endpoint string) (*Request, error) {
	if op == nil {
		return nil, errors.New(errors.CodeValidation, "graphql: nil operation")
	}
	opType, field, err := b.resolve(op)
	if err != nil {
		return nil, err
	}

	if endpoint == "" {
		endpoint = op.Source[SourceEndpoint]
	}

	variables := map[string]interface{}{}
	var decls []variableDecl
	var callArgs []string

	for i := range field.Args {
		arg := &field.Args[i]
		raw, provided := args[arg.Name]
		if !provided {
			if arg.Required() {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
					"graphql: missing required argument %q (GraphQL type %s)", arg.Name, arg.Type.String()))
			}
			// Optional and omitted: the server applies the declared default.
			continue
		}
		value, err := b.coerceValue(arg.Type, raw, arg.Name)
		if err != nil {
			return nil, err
		}
		variables[arg.Name] = value
		decls = append(decls, variableDecl{Name: arg.Name, Type: arg.Type.String()})
		callArgs = append(callArgs, fmt.Sprintf("%s: $%s", arg.Name, arg.Name))
	}

	if unknown := unknownArgs(field, args); len(unknown) > 0 {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: unknown argument(s) %s for operation %q (it takes %s)",
			strings.Join(unknown, ", "), op.ID, declaredArgList(field)))
	}

	opName := operationNameFor(field.Name, opType)
	doc, err := b.document(opName, opType, decls, field, callArgs)
	if err != nil {
		return nil, err
	}

	return &Request{
		Endpoint:      endpoint,
		Query:         doc,
		OperationName: opName,
		Variables:     variables,
		OperationID:   op.ID,
	}, nil
}

// variableDecl is one entry of a document's variable declaration list.
type variableDecl struct {
	Name string
	Type string
}

// resolve looks the operation's field up in the schema.
func (b *Builder) resolve(op *protocol.Operation) (OperationType, *Field, error) {
	opType := OperationType(op.Source[SourceOperationType])
	fieldName := op.Source[SourceField]
	if fieldName == "" {
		fieldName = op.ID
	}

	var root *Type
	switch opType {
	case OperationQuery:
		root = b.schema.QueryRoot()
	case OperationMutation:
		root = b.schema.MutationRoot()
	default:
		return "", nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: operation %q carries unsupported operation_type %q (only query and mutation are ingested)", op.ID, op.Source[SourceOperationType]))
	}
	if root == nil {
		return "", nil, errors.New(errors.CodeNotFound, fmt.Sprintf(
			"graphql: the schema exposes no %s root type for operation %q", opType, op.ID))
	}
	field := b.schema.OperationField(op)
	if field == nil {
		return "", nil, errors.New(errors.CodeNotFound, fmt.Sprintf(
			"graphql: field %s.%s (operation %q) is not in the schema", root.Name, fieldName, op.ID))
	}
	return opType, field, nil
}

// unknownArgs returns the caller-supplied argument names the field does not
// declare, in the caller's map iteration order (sorted for determinism).
func unknownArgs(field *Field, args map[string]interface{}) []string {
	var unknown []string
	for name := range args {
		if field.Arg(name) == nil {
			unknown = append(unknown, name)
		}
	}
	sortStrings(unknown)
	return unknown
}

// declaredArgList renders the field's declared arguments for error messages.
func declaredArgList(field *Field) string {
	if len(field.Args) == 0 {
		return "no arguments"
	}
	parts := make([]string, 0, len(field.Args))
	for i := range field.Args {
		parts = append(parts, fmt.Sprintf("%s: %s", field.Args[i].Name, field.Args[i].Type.String()))
	}
	return strings.Join(parts, ", ")
}

// coerceValue coerces a caller-supplied value onto a GraphQL input type.
//
// GraphQL's own input coercion rules apply: a single value is coerced to a list
// when the type is a list, enum membership is checked, input objects are
// validated field by field (unknown fields and missing required fields are
// errors), and the built-in scalars are parsed from the CLI's strings
// (JSON numbers arrive as float64). Values are returned as JSON-serializable Go
// values — they become the request's variables, never query text.
func (b *Builder) coerceValue(ref *TypeRef, value interface{}, path string) (interface{}, error) {
	if ref == nil {
		return value, nil
	}

	if ref.Kind == KindNonNull {
		if value == nil {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"graphql: %s is null but the type is non-null (%s)", path, ref.String()))
		}
		return b.coerceValue(ref.OfType, value, path)
	}

	if ref.IsList() {
		elem := ref.ListElement()
		if list, ok := value.([]interface{}); ok {
			out := make([]interface{}, 0, len(list))
			for i, item := range list {
				coerced, err := b.coerceValue(elem, item, fmt.Sprintf("%s[%d]", path, i))
				if err != nil {
					return nil, err
				}
				out = append(out, coerced)
			}
			return out, nil
		}
		// GraphQL coerces a single value to a one-element list.
		coerced, err := b.coerceValue(elem, value, path)
		if err != nil {
			return nil, err
		}
		return []interface{}{coerced}, nil
	}

	named := ref.Named()
	if named == nil {
		return value, nil
	}
	t := b.schema.Type(named.Name)
	if t == nil {
		return value, nil // undeclared type: pass the value through untouched
	}

	switch t.Kind {
	case KindEnum:
		return b.coerceEnum(t, value, path)
	case KindInputObject:
		return b.coerceInputObject(t, value, path)
	case KindScalar:
		if t.IsBuiltin {
			return coerceBuiltinScalar(t.Name, value, path)
		}
		return value, nil // custom scalar: the server defines its shape
	default:
		return value, nil
	}
}

// coerceEnum validates an enum argument against the enum's declared values.
func (b *Builder) coerceEnum(t *Type, value interface{}, path string) (interface{}, error) {
	name, ok := value.(string)
	if !ok {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: %s must be one of the %s enum values, got %T", path, t.Name, value))
	}
	if len(t.EnumValues) > 0 && !t.HasEnumValue(name) {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: %s value %q is not one of the %s values (%s)", path, name, t.Name, strings.Join(t.EnumValues, ", ")))
	}
	return name, nil
}

// coerceInputObject validates and coerces an input-object argument.
func (b *Builder) coerceInputObject(t *Type, value interface{}, path string) (interface{}, error) {
	obj, ok := value.(map[string]interface{})
	if !ok {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: %s must be a %s object (JSON object), got %T", path, t.Name, value))
	}

	for key := range obj {
		if t.InputField(key) == nil {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"graphql: %s has no field %q (it takes %s)", path, key, inputFieldList(t)))
		}
	}

	out := make(map[string]interface{}, len(obj))
	for i := range t.InputFields {
		field := &t.InputFields[i]
		raw, provided := obj[field.Name]
		if !provided {
			if field.Required() {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
					"graphql: %s.%s is required (GraphQL type %s)", path, field.Name, field.Type.String()))
			}
			// Omitted: the server applies the input field's declared default.
			continue
		}
		coerced, err := b.coerceValue(field.Type, raw, path+"."+field.Name)
		if err != nil {
			return nil, err
		}
		out[field.Name] = coerced
	}
	return out, nil
}

// inputFieldList renders an input object's field names for error messages.
func inputFieldList(t *Type) string {
	if len(t.InputFields) == 0 {
		return "no fields"
	}
	parts := make([]string, 0, len(t.InputFields))
	for i := range t.InputFields {
		parts = append(parts, fmt.Sprintf("%s: %s", t.InputFields[i].Name, t.InputFields[i].Type.String()))
	}
	return strings.Join(parts, ", ")
}

// coerceBuiltinScalar coerces a value onto one of GraphQL's built-in scalars,
// accepting the spellings a CLI produces.
func coerceBuiltinScalar(typeName string, value interface{}, path string) (interface{}, error) {
	switch typeName {
	case "Int":
		switch v := value.(type) {
		case int:
			return int64(v), nil
		case int64:
			return v, nil
		case float64:
			if v != float64(int64(v)) {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects an integer, got %v", path, v))
			}
			return int64(v), nil
		case string:
			var n int64
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err != nil || fmt.Sprintf("%d", n) != strings.TrimSpace(v) {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects an integer, got %q", path, v))
			}
			return n, nil
		default:
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects an integer, got %T", path, value))
		}
	case "Float":
		switch v := value.(type) {
		case float64:
			return v, nil
		case int64:
			return float64(v), nil
		case int:
			return float64(v), nil
		case string:
			var f float64
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%g", &f); err != nil {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects a number, got %q", path, v))
			}
			return f, nil
		default:
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects a number, got %T", path, value))
		}
	case "Boolean":
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects true or false, got %q", path, v))
		default:
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects true or false, got %T", path, value))
		}
	case "String", "ID":
		switch v := value.(type) {
		case string:
			return v, nil
		case int:
			return fmt.Sprintf("%d", v), nil
		case int64:
			return fmt.Sprintf("%d", v), nil
		case float64:
			if v == float64(int64(v)) {
				return fmt.Sprintf("%d", int64(v)), nil
			}
			return fmt.Sprintf("%v", v), nil
		case bool:
			return fmt.Sprintf("%t", v), nil
		default:
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf("graphql: %s expects a %s, got %T", path, typeName, value))
		}
	default:
		return value, nil
	}
}

// document renders the whole query document for one operation, in the shape
// GraphQL reference clients print (two-space indentation), e.g.
//
//	query UserQuery($id: ID!) {
//	  user(id: $id) {
//	    id
//	    name
//	  }
//	}
//
// callArgs renders the root field's argument list ("id: $id"); values never
// appear here, only variable references.
func (b *Builder) document(opName string, opType OperationType, decls []variableDecl, field *Field, callArgs []string) (string, error) {
	call := field.Name
	if len(callArgs) > 0 {
		call += "(" + strings.Join(callArgs, ", ") + ")"
	}

	fieldLines, err := b.renderFieldLines(field, call, 1, 1, true, map[string]bool{})
	if err != nil {
		return "", err
	}

	header := fmt.Sprintf("%s %s", opType, opName)
	if len(decls) > 0 {
		parts := make([]string, 0, len(decls))
		for _, d := range decls {
			parts = append(parts, fmt.Sprintf("$%s: %s", d.Name, d.Type))
		}
		header += "(" + strings.Join(parts, ", ") + ")"
	}

	lines := make([]string, 0, len(fieldLines)+2)
	lines = append(lines, header+" {")
	lines = append(lines, fieldLines...)
	lines = append(lines, "}")
	return strings.Join(lines, "\n"), nil
}

// renderFieldLines renders one selected field and, for object-typed fields, its
// sub-selection, as document lines at the given indent. call is the rendered
// argument list (only the operation's root field carries one). level is the
// selection level this field sits at (1 = the operation's root field), and
// allowList reports whether the caller's per-operation field allow-list applies
// to this field's return type.
//
// A field that takes only optional arguments is selected with no arguments: the
// derived document carries no values at all, so the server applies the declared
// defaults.
func (b *Builder) renderFieldLines(field *Field, call string, indent, level int, allowList bool, seen map[string]bool) ([]string, error) {
	prefix := strings.Repeat("  ", indent)
	named := field.Type.Named()
	sub := b.schema.Type(named.Name)

	if sub == nil || sub.IsKind(KindScalar, KindEnum) {
		return []string{prefix + call}, nil
	}
	if sub.Kind == KindUnion {
		if allowList && len(b.sel.Fields) > 0 {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"graphql: selection fields cannot be applied to the union type %s (field %s); selecting through a union needs inline fragments, which are out of scope",
				sub.Name, field.Name))
		}
		// Selecting through a union needs inline fragments, which the bounded
		// first row does not generate; __typename keeps the document valid and
		// the response actionable.
		return []string{
			prefix + call + " {",
			strings.Repeat("  ", indent+1) + "__typename",
			prefix + "}",
		}, nil
	}

	var subLines []string
	var err error
	if level < b.sel.MaxDepth {
		subLines, err = b.renderSelectionLines(sub, indent+1, level+1, allowList, seen)
	} else {
		subLines, err = b.boundaryLines(sub, indent+1)
	}
	if err != nil {
		return nil, err
	}
	if len(subLines) == 0 {
		// Only reachable for a type the schema does not describe: leave the
		// field bare rather than emitting an invalid empty selection.
		return []string{prefix + call}, nil
	}

	out := make([]string, 0, len(subLines)+2)
	out = append(out, prefix+call+" {")
	out = append(out, subLines...)
	out = append(out, prefix+"}")
	return out, nil
}

// renderSelectionLines renders the selection set of an object or interface type
// at the given indent; leaf types yield no lines. allowList applies the
// caller's per-operation field allow-list to this type's fields.
func (b *Builder) renderSelectionLines(t *Type, indent, level int, allowList bool, seen map[string]bool) ([]string, error) {
	prefix := strings.Repeat("  ", indent)

	switch t.Kind {
	case KindScalar, KindEnum:
		return nil, nil
	case KindUnion:
		return []string{prefix + "__typename"}, nil
	case KindObject, KindInterface:
		// continue
	default:
		return nil, nil
	}

	if seen[t.Name] {
		// A cycle (Foo → Bar → Foo) is cut here rather than recursed into.
		return []string{prefix + "__typename"}, nil
	}
	seen[t.Name] = true
	defer delete(seen, t.Name)

	candidates := b.selectableFields(t)

	// The per-operation field allow-list applies to the operation's own
	// selection (the level directly under the root field).
	if allowList && len(b.sel.Fields) > 0 {
		var err error
		candidates, err = b.allowListed(t, candidates)
		if err != nil {
			return nil, err
		}
	}
	if len(candidates) > b.sel.MaxFields {
		candidates = candidates[:b.sel.MaxFields]
	}

	var lines []string
	for _, field := range candidates {
		fieldLines, err := b.renderFieldLines(field, field.Name, indent, level, false, seen)
		if err != nil {
			return nil, err
		}
		lines = append(lines, fieldLines...)
	}
	if len(lines) == 0 {
		lines = append(lines, prefix+"__typename")
	}
	return lines, nil
}

// boundaryLines renders the selection of an object type at the depth boundary:
// its scalar and enum fields (never a deeper level), or __typename when it has
// none — an object-typed field must carry a non-empty selection.
func (b *Builder) boundaryLines(t *Type, indent int) ([]string, error) {
	prefix := strings.Repeat("  ", indent)
	lines := make([]string, 0, len(t.Fields))
	for i := range t.Fields {
		field := &t.Fields[i]
		if hasRequiredArg(field) {
			continue
		}
		named := field.Type.Named()
		sub := b.schema.Type(named.Name)
		if sub == nil || sub.IsKind(KindScalar, KindEnum) {
			lines = append(lines, prefix+field.Name)
		}
		if len(lines) >= b.sel.MaxFields {
			break
		}
	}
	if len(lines) == 0 {
		lines = append(lines, prefix+"__typename")
	}
	return lines, nil
}

// selectableFields returns the fields of an object/interface type the derived
// document may select: fields with no required arguments (the document carries
// no values for them), in declaration order.
func (b *Builder) selectableFields(t *Type) []*Field {
	out := make([]*Field, 0, len(t.Fields))
	for i := range t.Fields {
		field := &t.Fields[i]
		if hasRequiredArg(field) {
			continue
		}
		out = append(out, field)
	}
	return out
}

// allowListed filters the top-level selection to the caller's field allow-list,
// in the caller's order. Unknown names and names that need arguments are errors,
// never silent omissions.
func (b *Builder) allowListed(t *Type, candidates []*Field) ([]*Field, error) {
	allowed := make(map[string]*Field, len(candidates))
	for _, field := range candidates {
		allowed[field.Name] = field
	}

	out := make([]*Field, 0, len(b.sel.Fields))
	for _, name := range b.sel.Fields {
		field, ok := allowed[name]
		if !ok {
			if declared := t.Field(name); declared != nil && hasRequiredArg(declared) {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
					"graphql: selection field %s.%s needs arguments (%s), which the derived document does not carry",
					t.Name, name, declaredArgList(declared)))
			}
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"graphql: selection field %q is not on %s (selectable: %s)", name, t.Name, selectableNames(candidates)))
		}
		out = append(out, field)
	}
	return out, nil
}

// selectableNames renders selectable field names for error messages.
func selectableNames(fields []*Field) string {
	if len(fields) == 0 {
		return "none"
	}
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return strings.Join(names, ", ")
}

// hasRequiredArg reports whether the field declares a required argument.
func hasRequiredArg(field *Field) bool {
	for i := range field.Args {
		if field.Args[i].Required() {
			return true
		}
	}
	return false
}
