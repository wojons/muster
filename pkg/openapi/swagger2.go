package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// Swagger2Version is the value of the "swagger" root field in a
// Swagger/OpenAPI 2.0 document.
const Swagger2Version = "2.0"

// isSwagger2Document reports whether the raw document bytes carry the
// Swagger 2.0 root marker ("swagger": "2.0"). Detection is tolerant of
// JSON and YAML input and only inspects the two root fields it needs; a
// document that is neither valid JSON nor valid YAML returns false, so
// Parse falls through to the 3.x loader and reports its own error.
func isSwagger2Document(data []byte) bool {
	swagger, openapi, ok := swagger2RootFields(data)
	if !ok {
		return false
	}
	return swagger == Swagger2Version && openapi == ""
}

// swagger2RootFields decodes the document's two version-bearing root
// fields, JSON first and YAML second (JSON is a subset of YAML, so the
// YAML pass also covers documents the JSON pass rejected, such as one
// with an unquoted numeric version). An unquoted YAML `swagger: 2.0`
// decodes as a number and is normalized to the "2.0" string.
func swagger2RootFields(data []byte) (swagger string, openapi string, ok bool) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return "", "", false
		}
	}
	swagger, _ = root["swagger"].(string)
	if f, isNum := root["swagger"].(float64); isNum && f == 2 {
		swagger = "2.0"
	}
	openapi, _ = root["openapi"].(string)
	return swagger, openapi, true
}

// decodeSwagger2Document unmarshals raw document bytes into the
// kin-openapi openapi2 model. JSON decodes directly. YAML cannot: the
// openapi2 types implement encoding/json unmarshalers that yaml.v3 does
// not drive (measured: yaml.v3 feeds raw strings into openapi3.Types and
// fails on `type: integer`), so YAML is bridged through generic maps to
// JSON bytes first — the same shape kin-openapi's own YAML support uses
// via oasdiff/yaml's YAMLToJSON, kept self-contained here.
func decodeSwagger2Document(data []byte) (*openapi2.T, error) {
	var doc2 openapi2.T
	if err := json.Unmarshal(data, &doc2); err == nil {
		return &doc2, nil
	}
	var generic map[string]interface{}
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return nil, fmt.Errorf("decode Swagger 2.0 document: not valid JSON or YAML")
	}
	// yaml.v3 hands back interface-keyed mappings whenever a YAML key is
	// not a string (response status codes like `200:` are the common
	// Swagger 2.0 case), and encoding/json refuses those. Convert the
	// whole graph to JSON-marshalable values.
	normalized, err := yamlToJSONValue(generic)
	if err != nil {
		return nil, fmt.Errorf("decode Swagger 2.0 document: %w", err)
	}
	rootMap, ok := normalized.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("decode Swagger 2.0 document: document root is not a mapping")
	}
	generic = rootMap
	// Normalize an unquoted YAML `swagger: 2.0` (a float after decoding)
	// to the string the openapi2 model expects.
	if f, isNum := generic["swagger"].(float64); isNum && f == 2 {
		generic["swagger"] = Swagger2Version
	}
	asJSON, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("decode Swagger 2.0 document: %w", err)
	}
	if err := json.Unmarshal(asJSON, &doc2); err != nil {
		return nil, fmt.Errorf("decode Swagger 2.0 document: %w", err)
	}
	return &doc2, nil
}

// yamlToJSONValue converts a yaml.v3-decoded value into a
// JSON-marshalable value: interface-keyed mappings (which YAML produces
// whenever a key is not a string — response status codes like `200:` are
// the common Swagger 2.0 case) become string-keyed maps, recursively.
func yamlToJSONValue(v interface{}) (interface{}, error) {
	switch node := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(node))
		for key, child := range node {
			converted, err := yamlToJSONValue(child)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		return out, nil
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(node))
		for key, child := range node {
			s, err := yamlKeyToString(key)
			if err != nil {
				return nil, err
			}
			converted, err := yamlToJSONValue(child)
			if err != nil {
				return nil, err
			}
			out[s] = converted
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(node))
		for i, child := range node {
			converted, err := yamlToJSONValue(child)
			if err != nil {
				return nil, err
			}
			out[i] = converted
		}
		return out, nil
	default:
		return v, nil
	}
}

// yamlKeyToString stringifies a YAML mapping key for JSON marshaling.
func yamlKeyToString(key interface{}) (string, error) {
	switch k := key.(type) {
	case string:
		return k, nil
	case int:
		return strconv.Itoa(k), nil
	case float64:
		return strconv.FormatFloat(k, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(k), nil
	default:
		return "", fmt.Errorf("unsupported YAML key type %T", key)
	}
}

// convertSwagger2ToV3 ingests a raw Swagger 2.0 document (JSON or YAML)
// and returns the equivalent OpenAPI 3.x document, using kin-openapi's
// openapi2 model and openapi2conv.ToV3 conversion. The conversion covers
// definitions → components/schemas, parameters (including in:body →
// requestBody and in:formData → form request bodies), securityDefinitions
// → components/securitySchemes, and host/basePath/schemes → servers.
// The returned document is ref-resolved and validated, so a structurally
// broken 2.0 document fails here with the conversion error rather than
// degrading silently downstream.
//
// The recover is part of the contract, not decoration: kin-openapi
// v0.149.0 panics on some structurally malformed 2.0 documents (measured:
// ToV3PathItem dereferences a null paths entry at openapi2_conv.go:123).
// Parse is a boundary for untrusted input, so a conversion panic becomes
// an error — the document is rejected loudly, and the CLI keeps running.
func convertSwagger2ToV3(ctx context.Context, data []byte) (doc3 *openapi3.T, err error) {
	defer func() {
		if r := recover(); r != nil {
			doc3, err = nil, fmt.Errorf("convert Swagger 2.0 document to OpenAPI 3.x: malformed document: %v", r)
		}
	}()

	doc2, err := decodeSwagger2Document(data)
	if err != nil {
		return nil, err
	}
	if doc2.Swagger != Swagger2Version {
		return nil, fmt.Errorf("not a Swagger 2.0 document: swagger field is %q", doc2.Swagger)
	}
	converted, err := openapi2conv.ToV3(doc2)
	if err != nil {
		return nil, fmt.Errorf("convert Swagger 2.0 document to OpenAPI 3.x: %w", err)
	}
	// Validation parity with the 3.x path: LoadFromData does not validate,
	// and muster accepts a 3.x document with an empty paths section (zero
	// operations) — generate owns the loud refusal for that case. A
	// converted document that HAS paths is validated, so structurally
	// broken 2.0 content still fails loudly here. (An empty 2.0 paths map
	// is dropped by openapi2conv and would fail 3.0's required-paths check
	// — that tolerance gap is why validation is conditional.)
	if len(doc2.Paths) > 0 {
		if err := converted.Validate(ctx); err != nil {
			return nil, fmt.Errorf("validate converted Swagger 2.0 document: %w", err)
		}
	}
	return converted, nil
}
