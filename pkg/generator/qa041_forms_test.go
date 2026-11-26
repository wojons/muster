package generator

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/openapi"
)

// =============================================================================
// QA-MUSTER-041: form-encoded and multipart request bodies.
//
// The defect: 611 of the 612 operations in Stripe's spec3 declare ONLY an
// application/x-www-form-urlencoded body. The generator read application/json
// only, so every one of those commands exposed nothing but --auth/--base-url/
// -H/-o and sent Content-Length: 0 (CLI and MCP alike).
//
// The fix: per-property flags + the -b/--body escape hatch for form bodies, the
// body sent as urlencoded with its own Content-Type, and MCP schema parity.
// =============================================================================

// qa041FormSpecJSON is the form-encoded fixture: a Stripe-shaped POST whose
// body is urlencoded (required string, integer, array, object), plus a DELETE
// that declares an empty urlencoded body (Stripe does this on its 32 DELETEs)
// and a GET whose parameters are plain query params.
func qa041FormSpecJSON() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "Billing", "version": "1.0.0"},
  "paths": {
    "/v1/customers": {
      "post": {
        "operationId": "createCustomer",
        "summary": "Create customer",
        "requestBody": {
          "required": true,
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "properties": {
                  "email": {"type": "string", "description": "Customer email"},
                  "name": {"type": "string"},
                  "balance": {"type": "integer"},
                  "active": {"type": "boolean"},
                  "tags": {"type": "array", "items": {"type": "string"}},
                  "metadata": {"type": "object"}
                },
                "required": ["email"]
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      },
      "get": {
        "operationId": "listCustomers",
        "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/v1/customers/{id}": {
      "delete": {
        "operationId": "deleteCustomer",
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {"content": {"application/x-www-form-urlencoded": {}}},
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

// qa041MultipartSpecJSON is the multipart fixture: a binary file plus two text
// fields, one of them required.
func qa041MultipartSpecJSON() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "Documents", "version": "1.0.0"},
  "paths": {
    "/documents": {
      "post": {
        "operationId": "uploadDocument",
        "summary": "Upload document",
        "requestBody": {
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "file": {"type": "string", "format": "binary"},
                  "title": {"type": "string", "description": "Document title"},
                  "alt_text": {"type": "string"}
                },
                "required": ["file", "title"]
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

// qa041HeaderPropSpecJSON declares a form body property named `header`, which
// collides with the CLI's own -H/--header flag (Stripe's post-quotes really
// declares header/footer).
func qa041HeaderPropSpecJSON() string {
	return `{
  "openapi": "3.0.0",
  "info": {"title": "Quotes", "version": "1.0.0"},
  "paths": {
    "/v1/quotes": {
      "post": {
        "operationId": "createQuote",
        "requestBody": {
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "properties": {"header": {"type": "string"}, "footer": {"type": "string"}}
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
}

func qa041Generate(t *testing.T, spec, baseURL string) []*cobra.Command {
	t.Helper()
	gen, err := NewGenerator(Config{AppName: "muster", BaseURL: baseURL})
	require.NoError(t, err)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(spec))
	require.NoError(t, err)
	commands, err := gen.GenerateCommands(doc)
	require.NoError(t, err)
	return commands
}

type qa041Capture struct {
	method      string
	path        string
	rawQuery    string
	contentType string
	body        string
}

func qa041Server(t *testing.T) (*httptest.Server, *qa041Capture) {
	t.Helper()
	capture := &qa041Capture{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capture.method = r.Method
		capture.path = r.URL.Path
		capture.rawQuery = r.URL.RawQuery
		capture.contentType = r.Header.Get("Content-Type")
		capture.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": "cus_1"}`))
	}))
	t.Cleanup(ts.Close)
	return ts, capture
}

// --- (1) flag surface: --help shows the body properties and -b/--body -------

func TestQA041_FormEncoded_BodyFlagsGenerated(t *testing.T) {
	commands := qa041Generate(t, qa041FormSpecJSON(), "https://api.example.com")
	cmd, _, err := commands[0].Find([]string{"create-customer"})
	require.NoError(t, err)

	for _, name := range []string{"email", "name", "balance", "active", "tags", "metadata"} {
		require.NotNil(t, cmd.Flag(name), "form body property %q must be a flag (was: silently dropped)", name)
	}
	// QA-MUSTER-036: `email` is a schema-required body property, so its
	// usage string now carries the (required) marker shown in the Flags
	// table (enforcement stays in the run handler, DF-MUSTER-5).
	require.Equal(t, "Customer email (required)", cmd.Flag("email").Usage)

	bodyFlag := cmd.Flag(RawBodyFlagName)
	require.NotNil(t, bodyFlag, "-b/--body escape hatch must exist for a form body")
	assert.Equal(t, "b", bodyFlag.Shorthand)
	assert.Contains(t, bodyFlag.Usage, "application/x-www-form-urlencoded",
		"the escape hatch must name the media type so JSON is not pasted into a form op")
	assert.NotContains(t, bodyFlag.Usage, "Raw JSON")

	// Required body properties are enforced by the handler (so --body can
	// satisfy them), never by cobra.
	require.Empty(t, cmd.Flag("email").Annotations[cobra.BashCompOneRequiredFlag])
}

func TestQA041_FormEncoded_NonWriteMethodsGetNoBodyFlags(t *testing.T) {
	commands := qa041Generate(t, qa041FormSpecJSON(), "https://api.example.com")

	listCmd, _, err := commands[0].Find([]string{"list-customers"})
	require.NoError(t, err)
	require.Nil(t, listCmd.Flag(RawBodyFlagName), "GET must not expose --body")

	// Stripe declares an EMPTY urlencoded body on its DELETEs; turning that
	// into body flags would send payloads to an endpoint that reads the path.
	deleteCmd, _, err := commands[0].Find([]string{"delete-customer"})
	require.NoError(t, err)
	require.Nil(t, deleteCmd.Flag(RawBodyFlagName), "DELETE must not expose --body for an empty form body")
}

// --- (2) wire behavior: Content-Type + non-empty urlencoded body ------------

func TestQA041_FormEncoded_PerPropertyFlags_SendURLEncoded(t *testing.T) {
	ts, capture := qa041Server(t)
	commands := qa041Generate(t, qa041FormSpecJSON(), ts.URL)
	cmd, _, err := commands[0].Find([]string{"create-customer"})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("email", "foo@example.com"))
	require.NoError(t, cmd.Flags().Set("name", "Foo Bar"))
	require.NoError(t, cmd.Flags().Set("balance", "500"))
	require.NoError(t, cmd.Flags().Set("active", "true"))
	require.NoError(t, cmd.Flags().Set("tags", `["a","b"]`))
	require.NoError(t, cmd.Flags().Set("metadata", `{"owner":"qa"}`))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Equal(t, "POST", capture.method)
	assert.Equal(t, "/v1/customers", capture.path)
	assert.Equal(t, "application/x-www-form-urlencoded", capture.contentType)
	require.NotEmpty(t, capture.body, "the body must not be empty (the defect sent Content-Length: 0)")

	values, err := neturl.ParseQuery(capture.body)
	require.NoError(t, err)
	assert.Equal(t, "foo@example.com", values.Get("email"))
	assert.Equal(t, "Foo Bar", values.Get("name"))
	assert.Equal(t, "500", values.Get("balance"))
	assert.Equal(t, "true", values.Get("active"))
	assert.Equal(t, []string{"a", "b"}, values["tags"], "arrays repeat the key")
	assert.Equal(t, "qa", values.Get("metadata[owner]"), "objects use bracket notation")
	assert.Empty(t, capture.rawQuery, "body properties must not leak into the query string")
}

func TestQA041_FormEncoded_BareInvocationFailsLoudly(t *testing.T) {
	ts, capture := qa041Server(t)
	commands := qa041Generate(t, qa041FormSpecJSON(), ts.URL)
	cmd, _, err := commands[0].Find([]string{"create-customer"})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// A required form property with no value and no --body: the handler must
	// name what is missing instead of firing a bodyless request.
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--email")
	assert.Empty(t, capture.body, "no request body should have been sent")
}

func TestQA041_FormEncoded_RawBody_Verbatim(t *testing.T) {
	ts, capture := qa041Server(t)
	commands := qa041Generate(t, qa041FormSpecJSON(), ts.URL)
	cmd, _, err := commands[0].Find([]string{"create-customer"})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	raw := "email=raw%40example.com&name=Raw+Body&metadata[owner]=qa"
	require.NoError(t, cmd.Flags().Set(RawBodyFlagName, raw))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Equal(t, raw, capture.body, "--body must be sent verbatim (not re-encoded)")
	assert.Equal(t, "application/x-www-form-urlencoded", capture.contentType)
}

func TestQA041_FormEncoded_RawBody_RejectsJSONAndEmpty(t *testing.T) {
	commands := qa041Generate(t, qa041FormSpecJSON(), "https://api.example.com")
	cmd, _, err := commands[0].Find([]string{"create-customer"})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// A JSON payload on a form operation is a mistake, not something to send.
	require.NoError(t, cmd.Flags().Set(RawBodyFlagName, `{"email":"foo@example.com"}`))
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "urlencoded")

	require.NoError(t, cmd.Flags().Set(RawBodyFlagName, "  "))
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err, "an empty --body must be rejected, not sent as Content-Length: 0")
}

// --- (3) multipart: same per-property treatment, plus fields without a file -

func TestQA041_Multipart_BodyFlagsAndSchema(t *testing.T) {
	commands := qa041Generate(t, qa041MultipartSpecJSON(), "https://api.example.com")
	cmd, _, err := commands[0].Find([]string{"upload-document"})
	require.NoError(t, err)

	require.NotNil(t, cmd.Flag("file"), "multipart operations keep the --file flag")
	require.NotNil(t, cmd.Flag("title"), "non-file multipart field must be a flag")
	require.Equal(t, "Document title", cmd.Flag("title").Usage)
	require.NotNil(t, cmd.Flag("alt_text"))
}

func TestQA041_Multipart_FieldsWithoutFileAreSent(t *testing.T) {
	ts, capture := qa041Server(t)
	commands := qa041Generate(t, qa041MultipartSpecJSON(), ts.URL)
	cmd, _, err := commands[0].Find([]string{"upload-document"})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("title", "Quarterly report"))
	require.NoError(t, cmd.Flags().Set("alt_text", "chart"))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Contains(t, capture.contentType, "multipart/form-data")
	assert.Contains(t, capture.body, `name="title"`)
	assert.Contains(t, capture.body, "Quarterly report")
	assert.Contains(t, capture.body, `name="alt_text"`)
}

// --- (4) a body property that collides with a CLI control flag -------------

func TestQA041_ReservedFlagName_RenamedAndStillSent(t *testing.T) {
	ts, capture := qa041Server(t)
	commands := qa041Generate(t, qa041HeaderPropSpecJSON(), ts.URL)
	cmd, _, err := commands[0].Find([]string{"create-quote"})
	require.NoError(t, err)

	// The CLI owns --header (-H). Registering the property there panics pflag
	// in the reloaded CLI ("flag redefined: header"), so it is renamed and the
	// spec property name is stamped on the flag.
	require.NotNil(t, cmd.Flag("header-2"), "colliding body property must be renamed")
	require.Equal(t, []string{"header"}, cmd.Flag("header-2").Annotations[AnnotationBodyProperty])
	require.NotNil(t, cmd.Flag("footer"))

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("header-2", "H1"))
	require.NoError(t, cmd.Flags().Set("footer", "F1"))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	values, err := neturl.ParseQuery(capture.body)
	require.NoError(t, err)
	assert.Equal(t, "H1", values.Get("header"), "the renamed flag must fill the spec's body key")
	assert.Equal(t, "F1", values.Get("footer"))
}

// --- (5) unsupported media types are reported, not silently dropped --------

func TestQA041_UnsupportedBodyType_WarnedAtGeneration(t *testing.T) {
	ops := []*openapi.Operation{
		{Method: http.MethodPost, Path: "/things", UnsupportedBodyType: "application/xml"},
		{Method: http.MethodGet, Path: "/things"},
	}
	assert.Equal(t, []string{"POST /things (application/xml)"}, unsupportedBodyOperations(ops))

	// The command still exists (the rest of the spec stays usable) but nobody
	// can believe it carries a body.
	commands := qa041Generate(t, `{
      "openapi": "3.0.0",
      "info": {"title": "XML", "version": "1.0.0"},
      "paths": {"/things": {"post": {"operationId": "createThing",
        "requestBody": {"content": {"application/xml": {"schema": {"type": "object",
          "properties": {"name": {"type": "string"}}}}}},
        "responses": {"200": {"description": "OK"}}}}}
    }`, "https://api.example.com")
	cmd, _, err := commands[0].Find([]string{"create-thing"})
	require.NoError(t, err)
	require.Nil(t, cmd.Flag(RawBodyFlagName), "an unsupported media type gets no escape hatch")
	require.Nil(t, cmd.Flag("name"), "and no per-property flags")
}

// --- (6) JSON bodies keep their historical behavior ------------------------

func TestQA041_JSONBody_Unchanged(t *testing.T) {
	ts, capture := qa041Server(t)
	commands := qa041Generate(t, rawBodySpecJSON(), ts.URL)
	cmd, _, err := commands[0].Find([]string{"create-task"})
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Flags().Set("title", "typed"))
	require.NoError(t, cmd.RunE(cmd, []string{}))

	assert.Equal(t, "application/json", capture.contentType)
	assert.JSONEq(t, `{"title":"typed"}`, capture.body)
	assert.True(t, strings.HasPrefix(capture.body, "{"), "JSON bodies stay JSON")
}
