// Package graphql implements the GraphQL protocol adapter for Muster's
// protocol adapter seam (specs/PROTOCOLS.md §1, board row PROTO-005).
//
// Discovery (stage 1) ingests a GraphQL schema from either an introspection
// response (a live introspection query against an endpoint, or a recorded
// introspection JSON document on disk) or a local SDL document, and maps
// every Query and Mutation field onto the protocol-neutral Operation model.
// Execution (stage 4) sends ONE HTTP POST to ONE endpoint carrying a query
// document plus a variables object — GraphQL has no per-operation paths, so
// the REST-style request builder is deliberately not reused (specs/PROTOCOLS.md
// §2 matrix row 5). Transport assembly goes through pkg/request.Builder and
// pkg/client so tracing/retry/rate-limit wiring matches the HTTP path.
//
// Values are never interpolated into the query document: every argument value
// is bound as a GraphQL variable and the document declares the variables the
// call actually uses (a value can never change the shape of the document).
//
// Deliberate limits (bounded first row, documented in the CLI help):
//
//   - Subscriptions are NOT ingested. The Subscription root is recognized and
//     skipped; live subscriptions are the follow-up row (specs/PROTOCOLS.md
//     §2 row 6), which must land the bounded-collect contract of §4.5(4).
//   - Selection sets are derived from the schema, never hand-written: leaf
//     scalar/enum fields, recursion into object/interface fields up to a depth
//     cap, union fields selected as __typename only. Fields whose arguments
//     include a required (non-null, no default) argument are skipped because
//     the document carries no value for them. An explicit per-operation field
//     allow-list is supported; a raw selection string is not.
//   - SDL ingestion covers the type system minus directive semantics:
//     schema/type/extend/interface/union/enum/input/scalar definitions and
//     descriptions. Union and interface members are recorded (they inform
//     selection) but inline fragments are not generated.
//   - Custom scalars carry no JSON mapping of their own; they map onto the
//     neutral "string" type with the GraphQL type name preserved in the
//     operation's source stamp and document.
//   - Introspection disabled by the server (common production hardening)
//     surfaces as an actionable error naming the SDL fallback — never as an
//     empty command set.
package graphql

import (
	"fmt"
	"strings"
)

// TypeKind mirrors GraphQL's introspection __TypeKind. LIST and NON_NULL are
// wrapper kinds that appear only inside a TypeRef.
type TypeKind string

const (
	KindScalar      TypeKind = "SCALAR"
	KindObject      TypeKind = "OBJECT"
	KindInterface   TypeKind = "INTERFACE"
	KindUnion       TypeKind = "UNION"
	KindEnum        TypeKind = "ENUM"
	KindInputObject TypeKind = "INPUT_OBJECT"
	KindList        TypeKind = "LIST"
	KindNonNull     TypeKind = "NON_NULL"
)

// TypeRef is a GraphQL type reference: a named type optionally wrapped in
// NON_NULL / LIST wrappers, as GraphQL describes a type (User, User!, [User!]!,
// [Role]).
type TypeRef struct {
	Kind   TypeKind
	Name   string   // set for named kinds
	OfType *TypeRef // set for LIST and NON_NULL
}

// String renders the reference in GraphQL type syntax ("[User!]!").
func (t *TypeRef) String() string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case KindNonNull:
		return t.OfType.String() + "!"
	case KindList:
		return "[" + t.OfType.String() + "]"
	default:
		return t.Name
	}
}

// Named returns the innermost named type reference, unwrapping NON_NULL and
// LIST wrappers. Nil when the reference carries no named type.
func (t *TypeRef) Named() *TypeRef {
	for cur := t; cur != nil; cur = cur.OfType {
		switch cur.Kind {
		case KindNonNull, KindList:
			continue
		default:
			return cur
		}
	}
	return nil
}

// NamedType returns the name of the innermost named type ("" when none).
func (t *TypeRef) NamedType() string {
	if named := t.Named(); named != nil {
		return named.Name
	}
	return ""
}

// IsNonNull reports whether the reference is a non-null wrapper.
func (t *TypeRef) IsNonNull() bool {
	return t != nil && t.Kind == KindNonNull
}

// IsList reports whether the reference is a list after unwrapping an optional
// non-null wrapper.
func (t *TypeRef) IsList() bool {
	cur := t
	if cur.IsNonNull() {
		cur = cur.OfType
	}
	return cur != nil && cur.Kind == KindList
}

// ListElement returns the element reference of a list (unwrapping the list and
// any non-null element wrapper), or nil when the reference is not a list.
func (t *TypeRef) ListElement() *TypeRef {
	cur := t
	if cur.IsNonNull() {
		cur = cur.OfType
	}
	if cur == nil || cur.Kind != KindList {
		return nil
	}
	return cur.OfType
}

// InputValue is a field argument or an input-object field.
type InputValue struct {
	Name        string
	Description string
	Type        *TypeRef

	// DefaultValue is the GraphQL literal spelling of the default
	// (introspection's defaultValue / SDL's `= literal`). HasDefault
	// distinguishes an explicit `null` default from no default at all;
	// Default holds the parsed literal when it could be parsed.
	DefaultValue string
	HasDefault   bool
	Default      interface{}
}

// Required reports whether the value must be supplied by the caller: non-null
// with no default. A non-null argument carrying a default may be omitted and
// the server applies the default.
func (v *InputValue) Required() bool {
	if v == nil || v.Type == nil {
		return false
	}
	return v.Type.IsNonNull() && !v.HasDefault
}

// Field is an output field of an OBJECT or INTERFACE type.
type Field struct {
	Name        string
	Description string
	Args        []InputValue
	Type        *TypeRef
}

// Arg returns the named argument, or nil.
func (f *Field) Arg(name string) *InputValue {
	for i := range f.Args {
		if f.Args[i].Name == name {
			return &f.Args[i]
		}
	}
	return nil
}

// Type is a named type in the schema.
type Type struct {
	Kind          TypeKind
	Name          string
	Description   string
	Fields        []Field
	InputFields   []InputValue
	EnumValues    []string
	Interfaces    []string
	PossibleTypes []string

	// IsBuiltin marks GraphQL's built-in scalars and the introspection
	// meta-types, which are never turned into commands.
	IsBuiltin bool
}

// Field returns the named output field, or nil.
func (t *Type) Field(name string) *Field {
	if t == nil {
		return nil
	}
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

// InputField returns the named input field, or nil.
func (t *Type) InputField(name string) *InputValue {
	if t == nil {
		return nil
	}
	for i := range t.InputFields {
		if t.InputFields[i].Name == name {
			return &t.InputFields[i]
		}
	}
	return nil
}

// HasEnumValue reports whether the value is declared on the enum.
func (t *Type) HasEnumValue(value string) bool {
	if t == nil {
		return false
	}
	for _, v := range t.EnumValues {
		if v == value {
			return true
		}
	}
	return false
}

// IsKind reports whether the type exists and has one of the given kinds.
func (t *Type) IsKind(kinds ...TypeKind) bool {
	if t == nil {
		return false
	}
	for _, k := range kinds {
		if t.Kind == k {
			return true
		}
	}
	return false
}

// Schema is the ingested GraphQL schema: the subset of a schema's type system
// the adapter needs to build typed commands and query documents, in ONE shape
// regardless of whether the schema arrived as an introspection response or an
// SDL document. Both ingestion paths fill this same structure, which is what
// lets their operation lists be compared in tests.
type Schema struct {
	// Root type names. Empty when the schema does not declare that root
	// (spec default names apply for SDL that omits a schema block).
	QueryType        string
	MutationType     string
	SubscriptionType string

	Types map[string]*Type
	Order []string // declaration order, as the document listed the types

	// Source is where the schema was ingested from (file path, http(s) URL,
	// or the graphql:// introspection target) and SourceFormat which ingestion
	// path produced it. Both are recorded in discovery output.
	Source       string
	SourceFormat string
}

// Ingestion formats recorded on Schema.SourceFormat and on every operation's
// source stamp.
const (
	FormatSDL                = "sdl"
	FormatIntrospectionJSON  = "introspection-json"
	FormatIntrospectionQuery = "introspection-query"
)

// builtinScalarNames are GraphQL's five built-in scalars.
var builtinScalarNames = []string{"String", "Int", "Float", "Boolean", "ID"}

// NewSchema creates an empty schema that already knows GraphQL's built-in
// scalars. They are always resolvable (an SDL document usually does not declare
// them, while introspection always lists them), so seeding them here keeps the
// two ingestion paths structurally identical.
func NewSchema() *Schema {
	s := &Schema{Types: make(map[string]*Type)}
	for _, name := range builtinScalarNames {
		s.AddType(&Type{Kind: KindScalar, Name: name, IsBuiltin: true})
	}
	return s
}

// AddType registers a type, keeping first-registration declaration order.
func (s *Schema) AddType(t *Type) {
	if t == nil || t.Name == "" {
		return
	}
	if _, exists := s.Types[t.Name]; !exists {
		s.Order = append(s.Order, t.Name)
	}
	s.Types[t.Name] = t
}

// Type returns the named type, or nil.
func (s *Schema) Type(name string) *Type {
	if s == nil {
		return nil
	}
	return s.Types[name]
}

// QueryRoot returns the query root object type, or nil.
func (s *Schema) QueryRoot() *Type { return s.rootType(s.QueryType) }

// MutationRoot returns the mutation root object type, or nil.
func (s *Schema) MutationRoot() *Type { return s.rootType(s.MutationType) }

// SubscriptionRoot returns the subscription root object type, or nil.
func (s *Schema) SubscriptionRoot() *Type { return s.rootType(s.SubscriptionType) }

// List returns the schema's types in declaration order.
func (s *Schema) List() []*Type {
	if s == nil {
		return nil
	}
	out := make([]*Type, 0, len(s.Order))
	for _, name := range s.Order {
		out = append(out, s.Types[name])
	}
	return out
}

func (s *Schema) rootType(name string) *Type {
	if name == "" {
		return nil
	}
	t := s.Type(name)
	if t == nil || t.Kind != KindObject {
		return nil
	}
	return t
}

// NeutralType maps a GraphQL type reference onto the neutral, JSON-schema-style
// type vocabulary protocol.Parameter.Type carries (specs/PROTOCOLS.md §1.3 —
// the same vocabulary pkg/mcp uses for MCP input schemas).
//
// Mapping: a list maps to "array" regardless of its element type (the element
// type is preserved in the operation's source stamp and in the generated
// document), String/ID and custom scalars map to "string" (custom scalars have
// no JSON-schema type of their own — the documented fallback), Int to
// "integer", Float to "number", Boolean to "boolean", enums and input objects
// to "string" and "object".
func (s *Schema) NeutralType(ref *TypeRef) string {
	named := ref.Named()
	if named == nil {
		return "string"
	}
	typeName := named.Name

	if ref.IsList() {
		return "array"
	}

	t := s.Type(typeName)
	if t == nil {
		// A reference to a type the document never declared: fall back to the
		// scalar vocabulary rather than inventing a schema.
		return builtinScalarJSONType(typeName)
	}

	switch t.Kind {
	case KindEnum:
		return "string"
	case KindInputObject, KindObject, KindInterface, KindUnion:
		return "object"
	case KindScalar:
		if t.IsBuiltin {
			return builtinScalarJSONType(typeName)
		}
		return "string" // custom scalar: documented string fallback
	default:
		return builtinScalarJSONType(typeName)
	}
}

// builtinScalarJSONType maps GraphQL's built-in scalar names onto JSON-schema
// types; unknown names fall back to "string".
func builtinScalarJSONType(name string) string {
	switch name {
	case "Int":
		return "integer"
	case "Float":
		return "number"
	case "Boolean":
		return "boolean"
	case "String", "ID":
		return "string"
	default:
		return "string"
	}
}

// EnumValues returns the enum type's values as []interface{} for the neutral
// Parameter.Enum field (nil for non-enum types).
func (s *Schema) EnumValues(ref *TypeRef) []interface{} {
	named := ref.Named()
	if named == nil {
		return nil
	}
	t := s.Type(named.Name)
	if !t.IsKind(KindEnum) {
		return nil
	}
	out := make([]interface{}, 0, len(t.EnumValues))
	for _, v := range t.EnumValues {
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// builtinTypeNames are the types GraphQL defines itself; a schema may declare
// them (introspection always lists them) but they never become commands.
func isBuiltinTypeName(name string) bool {
	switch name {
	case "String", "Int", "Float", "Boolean", "ID":
		return true
	}
	return strings.HasPrefix(name, "__")
}

// firstSentence returns the first sentence of a description, for command
// summaries (empty in, empty out).
func firstSentence(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	// A blank line or a sentence end terminates the summary.
	if idx := strings.IndexAny(desc, "\n"); idx >= 0 {
		desc = strings.TrimSpace(desc[:idx])
	}
	if idx := strings.Index(desc, ". "); idx >= 0 {
		desc = desc[:idx+1]
	}
	return desc
}

// upperFirst capitalizes the first rune of a GraphQL name (GraphQL names are
// [_A-Za-z][_0-9A-Za-z]*, so this is always safe).
func upperFirst(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// operationNameFor derives the GraphQL operation name used in the request's
// operationName member and in the document's operation header. It is
// deterministic and derived from the ingested operation, never from an
// argument value.
func operationNameFor(field string, opType OperationType) string {
	return fmt.Sprintf("%s%s", upperFirst(field), strings.ToUpper(string(opType))[:1]+string(opType)[1:])
}
