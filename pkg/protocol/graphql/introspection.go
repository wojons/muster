package graphql

import (
	"encoding/json"
	"fmt"

	"github.com/wojons/muster/pkg/errors"
)

// This file ingests GraphQL's introspection response shape into the package's
// neutral Schema. The wire shape below is the introspection __schema object, as
// returned for the standard introspection query.

// introspectionSchema is the subset of __schema this adapter consumes.
type introspectionSchema struct {
	QueryType        *introspectionNamedTypeRef `json:"queryType"`
	MutationType     *introspectionNamedTypeRef `json:"mutationType"`
	SubscriptionType *introspectionNamedTypeRef `json:"subscriptionType"`
	Types            []introspectionType        `json:"types"`
}

// introspectionType models one __Type entry.
type introspectionType struct {
	Kind          string                      `json:"kind"`
	Name          string                      `json:"name"`
	Description   string                      `json:"description"`
	Fields        []introspectionField        `json:"fields"`
	InputFields   []introspectionInputVal     `json:"inputFields"`
	EnumValues    []introspectionEnumValue    `json:"enumValues"`
	Interfaces    []introspectionNamedTypeRef `json:"interfaces"`
	PossibleTypes []introspectionNamedTypeRef `json:"possibleTypes"`
}

// introspectionField models one field of an OBJECT or INTERFACE type.
type introspectionField struct {
	Name              string                  `json:"name"`
	Description       string                  `json:"description"`
	Args              []introspectionInputVal `json:"args"`
	Type              *introspectionTypeRef   `json:"type"`
	IsDeprecated      bool                    `json:"isDeprecated"`
	DeprecationReason string                  `json:"deprecationReason"`
}

// introspectionInputVal models a field argument or input-object field.
type introspectionInputVal struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Type         *introspectionTypeRef `json:"type"`
	DefaultValue *string               `json:"defaultValue"`
}

// introspectionEnumValue models one value of an ENUM type.
type introspectionEnumValue struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	IsDeprecated      bool   `json:"isDeprecated"`
	DeprecationReason string `json:"deprecationReason"`
}

// introspectionNamedTypeRef models {"name": "Query"}.
type introspectionNamedTypeRef struct {
	Name string `json:"name"`
}

// introspectionTypeRef models __Type's recursive type reference. Exactly one of
// Name and OfType is set for a given entry.
type introspectionTypeRef struct {
	Kind   string                `json:"kind"`
	Name   *string               `json:"name"`
	OfType *introspectionTypeRef `json:"ofType"`
}

// toTypeRef converts an introspection type reference into the package's TypeRef.
// Named references carry their declared kind; a reference whose entry omits
// "kind" (some hand-recorded documents do) is kept as a named placeholder — the
// schema's own type table is the authority on kinds for everything the adapter
// does.
func (r *introspectionTypeRef) toTypeRef() (*TypeRef, error) {
	if r == nil {
		return nil, errors.New(errors.CodeValidation, "graphql: introspection type reference is null")
	}
	switch TypeKind(r.Kind) {
	case KindNonNull:
		inner, err := r.OfType.toTypeRef()
		if err != nil {
			return nil, err
		}
		return &TypeRef{Kind: KindNonNull, OfType: inner}, nil
	case KindList:
		inner, err := r.OfType.toTypeRef()
		if err != nil {
			return nil, err
		}
		return &TypeRef{Kind: KindList, OfType: inner}, nil
	}
	if r.Name == nil || *r.Name == "" {
		return nil, errors.New(errors.CodeValidation, "graphql: introspection type reference carries neither name nor wrapper")
	}
	kind := TypeKind(r.Kind)
	switch kind {
	case KindScalar, KindObject, KindInterface, KindUnion, KindEnum, KindInputObject:
		// recognized named kind
	default:
		kind = KindObject
	}
	return &TypeRef{Kind: kind, Name: *r.Name}, nil
}

// decodeIntrospectionSchema extracts the __schema object from a recorded
// introspection response body.
func decodeIntrospectionSchema(body []byte) (*introspectionSchema, []GraphQLError, error) {
	// The envelope may also BE a raw schema object (queryType/types at the root);
	// those fields are read separately so a document of either shape decodes.
	var envelope struct {
		Data *struct {
			Schema *introspectionSchema `json:"__schema"`
		} `json:"data"`
		Schema    *introspectionSchema `json:"__schema"`
		QueryType *introspectionNamedTypeRef
		Types     []introspectionType
		Errors    []GraphQLError `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, errors.Wrap(err, errors.CodeBadRequest, "graphql: decode introspection response")
	}

	switch {
	case envelope.Data != nil && envelope.Data.Schema != nil:
		return envelope.Data.Schema, envelope.Errors, nil
	case envelope.Schema != nil:
		return envelope.Schema, envelope.Errors, nil
	case len(envelope.Types) > 0:
		return &introspectionSchema{QueryType: envelope.QueryType, Types: envelope.Types}, envelope.Errors, nil
	default:
		return nil, envelope.Errors, nil
	}
}

// schemaFromIntrospection maps a decoded introspection __schema onto the
// package's neutral Schema.
func schemaFromIntrospection(is *introspectionSchema) (*Schema, error) {
	if is == nil {
		return nil, errors.New(errors.CodeValidation, "graphql: introspection response carries no __schema object")
	}

	s := NewSchema()
	if is.QueryType != nil {
		s.QueryType = is.QueryType.Name
	}
	if is.MutationType != nil {
		s.MutationType = is.MutationType.Name
	}
	if is.SubscriptionType != nil {
		s.SubscriptionType = is.SubscriptionType.Name
	}

	for _, it := range is.Types {
		if it.Name == "" {
			continue
		}
		t, err := introspectionTypeToType(it)
		if err != nil {
			return nil, err
		}
		s.AddType(t)
	}

	// Introspection always lists the roots explicitly; a document that omitted
	// them still names them through the conventional type names.
	if s.QueryType == "" && s.Type("Query") != nil {
		s.QueryType = "Query"
	}
	if s.MutationType == "" && s.Type("Mutation") != nil {
		s.MutationType = "Mutation"
	}
	if s.SubscriptionType == "" && s.Type("Subscription") != nil {
		s.SubscriptionType = "Subscription"
	}
	return s, nil
}

// introspectionTypeToType converts one __Type entry.
func introspectionTypeToType(it introspectionType) (*Type, error) {
	t := &Type{
		Kind:        TypeKind(it.Kind),
		Name:        it.Name,
		Description: it.Description,
		IsBuiltin:   isBuiltinTypeName(it.Name),
	}
	for _, iface := range it.Interfaces {
		if iface.Name != "" {
			t.Interfaces = append(t.Interfaces, iface.Name)
		}
	}
	for _, possible := range it.PossibleTypes {
		if possible.Name != "" {
			t.PossibleTypes = append(t.PossibleTypes, possible.Name)
		}
	}
	for _, ev := range it.EnumValues {
		t.EnumValues = append(t.EnumValues, ev.Name)
	}

	for _, f := range it.Fields {
		ref, err := f.Type.toTypeRef()
		if err != nil {
			return nil, err
		}
		field := Field{Name: f.Name, Description: f.Description, Type: ref}
		for _, a := range f.Args {
			arg, err := introspectionInputToValue(a)
			if err != nil {
				return nil, err
			}
			field.Args = append(field.Args, arg)
		}
		t.Fields = append(t.Fields, field)
	}

	for _, in := range it.InputFields {
		val, err := introspectionInputToValue(in)
		if err != nil {
			return nil, err
		}
		t.InputFields = append(t.InputFields, val)
	}
	return t, nil
}

// introspectionInputToValue converts a field argument / input-object field.
func introspectionInputToValue(in introspectionInputVal) (InputValue, error) {
	ref, err := in.Type.toTypeRef()
	if err != nil {
		return InputValue{}, err
	}
	val := InputValue{Name: in.Name, Description: in.Description, Type: ref}
	if in.DefaultValue != nil && *in.DefaultValue != "" {
		val.DefaultValue = *in.DefaultValue
		val.HasDefault = true
		parsed, perr := parseValueLiteralString(*in.DefaultValue)
		if perr != nil {
			return InputValue{}, errors.Wrap(perr, errors.CodeValidation,
				fmt.Sprintf("graphql: parse default value of %q (%s)", in.Name, *in.DefaultValue))
		}
		val.Default = parsed
	}
	return val, nil
}
