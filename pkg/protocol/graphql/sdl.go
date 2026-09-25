package graphql

import (
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
)

// This file ingests GraphQL SDL (the schema definition language / type-system
// document form) into the package's neutral Schema, using the shared tokenizer
// in document.go.
//
// Supported definitions: schema, type, interface, union, enum, input, scalar,
// extend (of type/interface/input/enum), and directive definitions (parsed for
// their arguments and then discarded — directive semantics are out of scope for
// the bounded first row). Descriptions (both """block""" and "single-line") are
// attached to the definition that follows them.

// ParseSDL ingests an SDL document into a Schema.
//
// The result is the same neutral structure the introspection path produces:
// type kinds, fields, arguments, input fields, enum values, interfaces,
// possible types and root type names.
func ParseSDL(src string) (*Schema, error) {
	ts, err := newTokenStream(src)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeValidation, "graphql: tokenize SDL document")
	}
	s := NewSchema()

	for !ts.atEOF() {
		if err := parseSDLDefinition(ts, s); err != nil {
			return nil, err
		}
	}

	// A document without a schema block uses GraphQL's default root type
	// names, exactly as a server would.
	if s.QueryType == "" {
		if t := s.Type("Query"); t.IsKind(KindObject) {
			s.QueryType = "Query"
		}
	}
	if s.MutationType == "" {
		if t := s.Type("Mutation"); t.IsKind(KindObject) {
			s.MutationType = "Mutation"
		}
	}
	if s.SubscriptionType == "" {
		if t := s.Type("Subscription"); t.IsKind(KindObject) {
			s.SubscriptionType = "Subscription"
		}
	}

	// SDL states the interface → implementer relation only on the implementer
	// (`type User implements Node`); derive the reverse so the ingested schema
	// matches what introspection reports.
	derivePossibleTypes(s)
	return s, nil
}

// derivePossibleTypes records every implementer on the interfaces it declares.
func derivePossibleTypes(s *Schema) {
	for _, t := range s.List() {
		if !t.IsKind(KindObject, KindInterface) {
			continue
		}
		for _, ifaceName := range t.Interfaces {
			iface := s.Type(ifaceName)
			if iface == nil || !iface.IsKind(KindInterface) {
				continue
			}
			iface.PossibleTypes = appendUnique(iface.PossibleTypes, t.Name)
		}
	}
}

// parseSDLDefinition parses one top-level definition.
func parseSDLDefinition(ts *parserTokenStream, s *Schema) error {
	desc := parseDescription(ts)

	// `extend type X`, `extend input X`, ...: the extension merges into the
	// named type (creating it when the document extends a type it never
	// declared, which is legal in SDL when the base document is elsewhere).
	extend := false
	if ts.atName("extend") {
		extend = true
		if err := ts.advance(); err != nil {
			return err
		}
	}

	if !ts.init || ts.curToken().kind != tokName {
		return ts.errorf("expected a type-system definition but found %s", ts.curToken().describe())
	}
	keyword := ts.curToken().val

	switch keyword {
	case "schema":
		if extend {
			return ts.errorf("schema definitions cannot be extended")
		}
		return parseSchemaDefinition(ts, s)
	case "type":
		return parseObjectDefinition(ts, s, desc, KindObject, extend)
	case "interface":
		return parseObjectDefinition(ts, s, desc, KindInterface, extend)
	case "input":
		return parseInputDefinition(ts, s, desc, extend)
	case "enum":
		return parseEnumDefinition(ts, s, desc, extend)
	case "union":
		return parseUnionDefinition(ts, s, desc, extend)
	case "scalar":
		return parseScalarDefinition(ts, s, desc, extend)
	case "directive":
		return skipDirectiveDefinition(ts)
	default:
		return ts.errorf("unsupported SDL definition %q", keyword)
	}
}

// parseDescription consumes an optional description string.
func parseDescription(ts *parserTokenStream) string {
	if ts.init && ts.curToken().kind == tokString {
		desc := ts.curToken().val
		if err := ts.advance(); err != nil {
			return desc
		}
		return strings.TrimSpace(desc)
	}
	return ""
}

// parseSchemaDefinition parses `schema { query: Q mutation: M }`.
func parseSchemaDefinition(ts *parserTokenStream, s *Schema) error {
	if err := ts.advance(); err != nil { // consume "schema"
		return err
	}
	if err := skipDirectives(ts); err != nil {
		return err
	}
	if err := ts.expectPunct("{"); err != nil {
		return err
	}
	for !ts.atPunct("}") {
		if ts.atEOF() {
			return ts.errorf("unterminated schema definition")
		}
		root, err := ts.expectName("root operation type (query, mutation, subscription)")
		if err != nil {
			return err
		}
		if err := ts.expectPunct(":"); err != nil {
			return err
		}
		typeName, err := ts.expectName("root type name")
		if err != nil {
			return err
		}
		switch root {
		case "query":
			s.QueryType = typeName
		case "mutation":
			s.MutationType = typeName
		case "subscription":
			s.SubscriptionType = typeName
		default:
			return ts.errorf("unknown root operation type %q", root)
		}
	}
	return ts.expectPunct("}")
}

// parseObjectDefinition parses `type Name implements I & J { fields }` and the
// interface form of the same shape.
func parseObjectDefinition(ts *parserTokenStream, s *Schema, desc string, kind TypeKind, extend bool) error {
	if err := ts.advance(); err != nil { // consume "type" / "interface"
		return err
	}
	name, err := ts.expectName("type name")
	if err != nil {
		return err
	}

	t := s.Type(name)
	if t == nil || !extend {
		if t != nil && !extend && len(t.Fields) > 0 {
			return errors.New(errors.CodeValidation, fmt.Sprintf("graphql: type %q is defined twice", name))
		}
		t = &Type{Kind: kind, Name: name, Description: desc, IsBuiltin: isBuiltinTypeName(name)}
		s.AddType(t)
	}
	if desc != "" && t.Description == "" {
		t.Description = desc
	}

	if ts.atName("implements") {
		if err := ts.advance(); err != nil {
			return err
		}
		for {
			iface, err := ts.expectName("interface name")
			if err != nil {
				return err
			}
			t.Interfaces = appendUnique(t.Interfaces, iface)
			if !ts.atPunct("&") {
				break
			}
			if err := ts.advance(); err != nil {
				return err
			}
		}
	}
	if err := skipDirectives(ts); err != nil {
		return err
	}
	if !ts.atPunct("{") {
		return nil // a legal, field-less declaration
	}

	fields, err := parseFieldsDefinition(ts)
	if err != nil {
		return err
	}
	for _, f := range fields {
		if existing := t.Field(f.Name); existing != nil {
			if extend {
				*existing = f
				continue
			}
			return errors.New(errors.CodeValidation, fmt.Sprintf("graphql: field %s.%s is defined twice", t.Name, f.Name))
		}
		t.Fields = append(t.Fields, f)
	}
	return nil
}

// parseFieldsDefinition parses `{ name(args): Type ... }`.
func parseFieldsDefinition(ts *parserTokenStream) ([]Field, error) {
	if err := ts.expectPunct("{"); err != nil {
		return nil, err
	}
	var fields []Field
	for !ts.atPunct("}") {
		if ts.atEOF() {
			return nil, ts.errorf("unterminated fields definition")
		}
		desc := parseDescription(ts)
		name, err := ts.expectName("field name")
		if err != nil {
			return nil, err
		}
		args, err := parseArgumentsDefinition(ts)
		if err != nil {
			return nil, err
		}
		if err := ts.expectPunct(":"); err != nil {
			return nil, err
		}
		ref, err := parseTypeRefFromTokens(ts)
		if err != nil {
			return nil, err
		}
		if err := skipDirectives(ts); err != nil {
			return nil, err
		}
		fields = append(fields, Field{Name: name, Description: desc, Args: args, Type: ref})
	}
	return fields, ts.expectPunct("}")
}

// parseArgumentsDefinition parses an optional `( name: Type = default ... )`.
func parseArgumentsDefinition(ts *parserTokenStream) ([]InputValue, error) {
	if !ts.atPunct("(") {
		return nil, nil
	}
	if err := ts.advance(); err != nil {
		return nil, err
	}
	var args []InputValue
	for !ts.atPunct(")") {
		if ts.atEOF() {
			return nil, ts.errorf("unterminated arguments definition")
		}
		val, err := parseInputValueDefinition(ts)
		if err != nil {
			return nil, err
		}
		args = append(args, val)
	}
	return args, ts.expectPunct(")")
}

// parseInputValueDefinition parses one argument or input-object field,
// including an optional default value literal.
func parseInputValueDefinition(ts *parserTokenStream) (InputValue, error) {
	desc := parseDescription(ts)
	name, err := ts.expectName("argument name")
	if err != nil {
		return InputValue{}, err
	}
	if err := ts.expectPunct(":"); err != nil {
		return InputValue{}, err
	}
	ref, err := parseTypeRefFromTokens(ts)
	if err != nil {
		return InputValue{}, err
	}
	val := InputValue{Name: name, Description: desc, Type: ref}
	if ts.atPunct("=") {
		if err := ts.advance(); err != nil {
			return InputValue{}, err
		}
		val.HasDefault = true
		parsed, err := parseValueLiteral(ts)
		if err != nil {
			return InputValue{}, err
		}
		val.Default = parsed
		val.DefaultValue = formatValueLiteral(parsed)
	}
	if err := skipDirectives(ts); err != nil {
		return InputValue{}, err
	}
	return val, nil
}

// parseInputDefinition parses `input Name { fields }`.
func parseInputDefinition(ts *parserTokenStream, s *Schema, desc string, extend bool) error {
	if err := ts.advance(); err != nil { // consume "input"
		return err
	}
	name, err := ts.expectName("input type name")
	if err != nil {
		return err
	}
	t := s.Type(name)
	if t == nil || !extend {
		t = &Type{Kind: KindInputObject, Name: name, Description: desc}
		s.AddType(t)
	}
	if desc != "" && t.Description == "" {
		t.Description = desc
	}
	if err := skipDirectives(ts); err != nil {
		return err
	}
	if !ts.atPunct("{") {
		return nil
	}
	if err := ts.advance(); err != nil {
		return err
	}
	for !ts.atPunct("}") {
		if ts.atEOF() {
			return ts.errorf("unterminated input fields definition")
		}
		val, err := parseInputValueDefinition(ts)
		if err != nil {
			return err
		}
		if existing := t.InputField(val.Name); existing != nil {
			*existing = val
			continue
		}
		t.InputFields = append(t.InputFields, val)
	}
	return ts.expectPunct("}")
}

// parseEnumDefinition parses `enum Name { A B }`.
func parseEnumDefinition(ts *parserTokenStream, s *Schema, desc string, extend bool) error {
	if err := ts.advance(); err != nil { // consume "enum"
		return err
	}
	name, err := ts.expectName("enum name")
	if err != nil {
		return err
	}
	t := s.Type(name)
	if t == nil || !extend {
		t = &Type{Kind: KindEnum, Name: name, Description: desc}
		s.AddType(t)
	}
	if desc != "" && t.Description == "" {
		t.Description = desc
	}
	if err := skipDirectives(ts); err != nil {
		return err
	}
	if !ts.atPunct("{") {
		return nil
	}
	if err := ts.advance(); err != nil {
		return err
	}
	for !ts.atPunct("}") {
		if ts.atEOF() {
			return ts.errorf("unterminated enum values definition")
		}
		_ = parseDescription(ts)
		val, err := ts.expectName("enum value")
		if err != nil {
			return err
		}
		if err := skipDirectives(ts); err != nil {
			return err
		}
		t.EnumValues = appendUnique(t.EnumValues, val)
	}
	return ts.expectPunct("}")
}

// parseUnionDefinition parses `union Name = A | B`.
func parseUnionDefinition(ts *parserTokenStream, s *Schema, desc string, extend bool) error {
	if err := ts.advance(); err != nil { // consume "union"
		return err
	}
	name, err := ts.expectName("union name")
	if err != nil {
		return err
	}
	t := s.Type(name)
	if t == nil || !extend {
		t = &Type{Kind: KindUnion, Name: name, Description: desc}
		s.AddType(t)
	}
	if err := skipDirectives(ts); err != nil {
		return err
	}
	if !ts.atPunct("=") {
		return nil
	}
	if err := ts.advance(); err != nil {
		return err
	}
	for {
		member, err := ts.expectName("union member type")
		if err != nil {
			return err
		}
		t.PossibleTypes = appendUnique(t.PossibleTypes, member)
		if !ts.atPunct("|") {
			break
		}
		if err := ts.advance(); err != nil {
			return err
		}
	}
	return skipDirectives(ts)
}

// parseScalarDefinition parses `scalar Name`.
func parseScalarDefinition(ts *parserTokenStream, s *Schema, desc string, extend bool) error {
	if err := ts.advance(); err != nil { // consume "scalar"
		return err
	}
	name, err := ts.expectName("scalar name")
	if err != nil {
		return err
	}
	if s.Type(name) == nil || !extend {
		s.AddType(&Type{
			Kind:        KindScalar,
			Name:        name,
			Description: desc,
			IsBuiltin:   isBuiltinTypeName(name),
		})
	}
	return skipDirectives(ts)
}

// skipDirectiveDefinition parses and discards `directive @name(args) on LOCATIONS`.
func skipDirectiveDefinition(ts *parserTokenStream) error {
	if err := ts.advance(); err != nil { // consume "directive"
		return err
	}
	if err := ts.expectPunct("@"); err != nil {
		return err
	}
	if _, err := ts.expectName("directive name"); err != nil {
		return err
	}
	if _, err := parseArgumentsDefinition(ts); err != nil {
		return err
	}
	if ts.atName("repeatable") {
		if err := ts.advance(); err != nil {
			return err
		}
	}
	if _, err := ts.expectName("on"); err != nil {
		return err
	}
	for {
		if _, err := ts.expectName("directive location"); err != nil {
			return err
		}
		if !ts.atPunct("|") {
			return nil
		}
		if err := ts.advance(); err != nil {
			return err
		}
	}
}

// skipDirectives parses and discards zero or more `@name(args)` applications.
// Directive semantics are out of scope for the bounded first row; the arguments
// are still parsed so a directive carrying a complex literal cannot derail the
// rest of the document.
func skipDirectives(ts *parserTokenStream) error {
	for ts.atPunct("@") {
		if err := ts.advance(); err != nil {
			return err
		}
		if _, err := ts.expectName("directive name"); err != nil {
			return err
		}
		if ts.atPunct("(") {
			if err := ts.advance(); err != nil {
				return err
			}
			for !ts.atPunct(")") {
				if ts.atEOF() {
					return ts.errorf("unterminated directive arguments")
				}
				if _, err := ts.expectName("directive argument name"); err != nil {
					return err
				}
				if err := ts.expectPunct(":"); err != nil {
					return err
				}
				if _, err := parseValueLiteral(ts); err != nil {
					return err
				}
			}
			if err := ts.expectPunct(")"); err != nil {
				return err
			}
		}
	}
	return nil
}

// appendUnique appends value when it is not already present.
func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

// formatValueLiteral renders a parsed default value back into a compact GraphQL
// literal (used when a default arrives through SDL rather than introspection, so
// both ingestion paths report defaults in the same spelling).
func formatValueLiteral(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case bool:
		if val {
			return "true"
		}
		return "false"
	case string:
		return fmt.Sprintf("%q", val)
	case enumLiteral:
		return string(val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%v", val)
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, formatValueLiteral(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sortStrings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+": "+formatValueLiteral(val[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// sortStrings sorts a small string slice in place (avoids pulling sort into
// every caller's import list; the slices here are tiny).
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
