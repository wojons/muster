package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// QA-MUSTER-041: form-encoded request bodies must be parsed, not dropped.
//
// Before this fix extractRequestBodyProperties read application/json only, so
// 611 of the 612 operations in Stripe's spec3 (which declare ONLY
// application/x-www-form-urlencoded) produced zero body properties and the
// generated CLI/MCP surface was argument-free.
// =============================================================================

const qa041FormSpecJSON = `{
  "openapi": "3.0.0",
  "info": {"title": "Billing", "version": "1.0.0"},
  "paths": {
    "/v1/subscriptions": {
      "post": {
        "operationId": "createSubscription",
        "summary": "Create subscription",
        "requestBody": {
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "properties": {
                  "customer": {"type": "string"},
                  "quantity": {"type": "integer"},
                  "items": {"type": "array", "items": {"type": "string"}}
                },
                "required": ["customer"]
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/v1/subscriptions/{id}": {
      "delete": {
        "operationId": "cancelSubscription",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {"content": {"application/x-www-form-urlencoded": {}}},
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

const qa041JSONWinsSpecJSON = `{
  "openapi": "3.0.0",
  "info": {"title": "Both", "version": "1.0.0"},
  "paths": {
    "/things": {
      "post": {
        "operationId": "createThing",
        "requestBody": {
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {"type": "object", "properties": {"form_only": {"type": "string"}}}
            },
            "application/json": {
              "schema": {"type": "object", "properties": {"json_only": {"type": "string"}}}
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

const qa041XMLOnlySpecJSON = `{
  "openapi": "3.0.0",
  "info": {"title": "XML", "version": "1.0.0"},
  "paths": {
    "/things": {
      "post": {
        "operationId": "createThingXml",
        "requestBody": {
          "content": {"application/xml": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

func qa041Ops(t *testing.T, spec string) []*Operation {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(spec))
	require.NoError(t, err)
	ops, err := NewParser().ExtractOperations(doc)
	require.NoError(t, err)
	return ops
}

func qa041Find(t *testing.T, ops []*Operation, method, path string) *Operation {
	t.Helper()
	for _, op := range ops {
		if op.Method == method && op.Path == path {
			return op
		}
	}
	require.FailNow(t, "operation not found", "%s %s", method, path)
	return nil
}

func TestQA041_FormEncodedBody_Parsed(t *testing.T) {
	ops := qa041Ops(t, qa041FormSpecJSON)
	op := qa041Find(t, ops, "POST", "/v1/subscriptions")

	assert.Equal(t, ContentTypeForm, op.BodyContentType,
		"a urlencoded body must be recognized as the operation's body media type")
	assert.Empty(t, op.UnsupportedBodyType)

	props := make(map[string]RequestBodyProperty, len(op.RequestBody))
	for _, p := range op.RequestBody {
		props[p.Name] = p
	}
	require.Len(t, props, 3, "all three urlencoded properties must be extracted")
	assert.Equal(t, "string", props["customer"].Type)
	assert.Equal(t, "integer", props["quantity"].Type)
	assert.Equal(t, "array", props["items"].Type)
	assert.True(t, props["customer"].Required, "the required list must be honoured")
	assert.False(t, props["quantity"].Required)
}

func TestQA041_FormEncodedBody_SchemaLessStillTyped(t *testing.T) {
	ops := qa041Ops(t, qa041FormSpecJSON)
	op := qa041Find(t, ops, "DELETE", "/v1/subscriptions/{id}")

	// Stripe declares empty urlencoded bodies on its GETs and DELETEs. The
	// media type is still known (so the -b escape hatch exists) but there are
	// no properties, and it is NOT an unsupported body type.
	assert.Equal(t, ContentTypeForm, op.BodyContentType)
	assert.Empty(t, op.RequestBody)
	assert.Empty(t, op.UnsupportedBodyType)
}

func TestQA041_JSONBodyWinsWhenBothDeclared(t *testing.T) {
	ops := qa041Ops(t, qa041JSONWinsSpecJSON)
	op := qa041Find(t, ops, "POST", "/things")

	assert.Equal(t, ContentTypeJSON, op.BodyContentType)
	require.Len(t, op.RequestBody, 1)
	assert.Equal(t, "json_only", op.RequestBody[0].Name)
}

func TestQA041_UnsupportedBodyMediaTypeReported(t *testing.T) {
	ops := qa041Ops(t, qa041XMLOnlySpecJSON)
	op := qa041Find(t, ops, "POST", "/things")

	assert.Empty(t, op.BodyContentType)
	assert.Equal(t, "application/xml", op.UnsupportedBodyType,
		"a body media type the generator cannot express must be reported, not silently dropped")
}
