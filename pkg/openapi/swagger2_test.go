package openapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// swagger2ToOps is the shared measurement harness for the PROTO-006 tests:
// parse raw document bytes and return the extracted operations.
func swagger2ToOps(t *testing.T, data []byte) (*ParseResult, []*Operation) {
	t.Helper()
	p := NewParser()
	result, err := p.Parse(context.Background(), data, DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, result)
	ops, err := p.ExtractOperations(result.Document)
	require.NoError(t, err)
	return result, ops
}

func findOp(t *testing.T, ops []*Operation, id string) *Operation {
	t.Helper()
	for _, op := range ops {
		if op.ID == id {
			return op
		}
	}
	t.Fatalf("operation %q not found", id)
	return nil
}

func TestParser_Parse_Swagger2Petstore(t *testing.T) {
	data, err := os.ReadFile("../../demo/openapi-specs/petstore-v2.json")
	require.NoError(t, err, "petstore-v2.json fixture should load")

	result, ops := swagger2ToOps(t, data)

	// The document must convert, and the conversion must be REPORTED —
	// the discovery output states explicitly that a 2.0 document was
	// converted (PROTO-006 acceptance 3).
	assert.True(t, result.ConvertedFromSwagger2,
		"Parse should report the Swagger 2.0 conversion")
	assert.Equal(t, "3.0.3", result.Document.OpenAPI,
		"converted document should carry the 3.x model version")
	assert.Equal(t, "1.0.7", result.Version,
		"info.version should survive the conversion")

	// host/basePath/schemes → servers (PROTO-006 acceptance 2).
	require.NotEmpty(t, result.Document.Servers)
	assert.Equal(t, "https://petstore.swagger.io/v2", result.Document.Servers[0].URL)

	// Every operation in the 2.0 document survives conversion.
	assert.Len(t, ops, 6)

	// definitions/schemas → components/schemas.
	require.NotNil(t, result.Document.Components)
	for _, name := range []string{"Pet", "Category", "Tag"} {
		assert.Contains(t, result.Document.Components.Schemas, name,
			"definition %s should convert to a component schema", name)
	}

	// securityDefinitions → components/securitySchemes.
	require.NotNil(t, result.Document.Components.SecuritySchemes)
	apiKey, ok := result.Document.Components.SecuritySchemes["api_key"]
	require.True(t, ok, "apiKey security definition should convert")
	require.NotNil(t, apiKey.Value)
	assert.Equal(t, "apiKey", apiKey.Value.Type)
	assert.Equal(t, "header", apiKey.Value.In)

	// Parameters (PROTO-006 acceptance 2): path, query (with enum/default),
	// in:body, and formData all convert with their schemas attached.
	pet := findOp(t, ops, "getPetById")
	require.Len(t, pet.Parameters, 1)
	assert.Equal(t, Parameter{
		In:          "path",
		Name:        "petId",
		Type:        "integer",
		Required:    true,
		Description: "ID of pet to return",
	}, pet.Parameters[0])

	byStatus := findOp(t, ops, "findPetsByStatus")
	require.Len(t, byStatus.Parameters, 1)
	assert.Equal(t, "query", byStatus.Parameters[0].In)
	assert.Equal(t, "status", byStatus.Parameters[0].Name)
	assert.Equal(t, "string", byStatus.Parameters[0].Type)
	assert.True(t, byStatus.Parameters[0].Required)
	assert.Len(t, byStatus.Parameters[0].Enum, 3)
	assert.Equal(t, "available", byStatus.Parameters[0].Default)

	updatePet := findOp(t, ops, "updatePet")
	assert.Equal(t, "https://petstore.swagger.io/v2", updatePet.ServerURL)
	// in:body with a $ref schema converts to a requestBody whose JSON
	// properties are extracted per-property (the pre-PROTO-006 behavior
	// silently produced zero body properties for 2.0 documents).
	names := make(map[string]RequestBodyProperty, len(updatePet.RequestBody))
	for _, prop := range updatePet.RequestBody {
		names[prop.Name] = prop
	}
	for _, want := range []string{"id", "name", "photoUrls", "tags", "status", "category"} {
		assert.Contains(t, names, want, "body property %q should be extracted from the converted 2.0 requestBody", want)
	}
	assert.True(t, names["name"].Required, "required definitions field should surface as a required body property")
	assert.Equal(t, "string", names["name"].Type)

	form := findOp(t, ops, "updatePetWithForm")
	// formData parameters convert into a request body with content type
	// application/x-www-form-urlencoded (measured openapi2conv behavior).
	// muster's HasMultiPart gate covers multipart/form-data only, so this
	// is false — exactly as for a native 3.x document declaring the same
	// form body, which is what the parity test below pins.
	assert.False(t, form.HasMultiPart)
	// The conversion itself is proven at the document level: the formData
	// name/status parameters became properties of the form request body.
	formBody := result.Document.Paths.Map()["/pet/{petId}"].Post.RequestBody
	require.NotNil(t, formBody)
	require.NotNil(t, formBody.Value)
	formMT := formBody.Value.Content.Get("application/x-www-form-urlencoded")
	require.NotNil(t, formMT, "formData parameters should convert to a urlencoded request body")
	require.NotNil(t, formMT.Schema)
	require.NotNil(t, formMT.Schema.Value)
	assert.Contains(t, formMT.Schema.Value.Properties, "name")
	assert.Contains(t, formMT.Schema.Value.Properties, "status")
}

// TestParser_Parse_Swagger2ParityWithHandUpgraded3 is the conversion parity
// cell the PROTOCOLS.md gate names: operations extracted from the converted
// 2.0 document equal the operations extracted from the same document
// hand-upgraded to 3.x.
func TestParser_Parse_Swagger2ParityWithHandUpgraded3(t *testing.T) {
	v2Data, err := os.ReadFile("../../demo/openapi-specs/petstore-v2.json")
	require.NoError(t, err)
	v3Data, err := os.ReadFile("../../demo/openapi-specs/petstore-v2-as-v3.json")
	require.NoError(t, err)

	_, v2Ops := swagger2ToOps(t, v2Data)
	v3Parser := NewParser()
	v3Result, err := v3Parser.Parse(context.Background(), v3Data, DefaultParseOptions())
	require.NoError(t, err)
	assert.False(t, v3Result.ConvertedFromSwagger2,
		"a native 3.x document must not be reported as converted")
	v3Ops, err := v3Parser.ExtractOperations(v3Result.Document)
	require.NoError(t, err)

	type opKey struct{ method, path string }
	v2Map := make(map[opKey]*Operation, len(v2Ops))
	for _, op := range v2Ops {
		v2Map[opKey{op.Method, op.Path}] = op
	}
	v3Map := make(map[opKey]*Operation, len(v3Ops))
	for _, op := range v3Ops {
		v3Map[opKey{op.Method, op.Path}] = op
	}
	require.Len(t, v3Map, len(v3Ops))
	assert.Len(t, v2Map, len(v3Map), "both documents should yield the same operation set")

	for key, want := range v3Map {
		got, ok := v2Map[key]
		require.True(t, ok, "converted 2.0 document is missing operation %v", key)
		assert.Equal(t, want.ID, got.ID)
		assert.Equal(t, want.Summary, got.Summary)
		assert.Equal(t, want.Tags, got.Tags)
		assert.Equal(t, want.Parameters, got.Parameters,
			"parameters differ for %v between converted 2.0 and hand-upgraded 3.x", key)
		assert.ElementsMatch(t, want.RequestBody, got.RequestBody,
			"request body properties differ for %v between converted 2.0 and hand-upgraded 3.x", key)
		assert.Equal(t, want.HasMultiPart, got.HasMultiPart)
		assert.Equal(t, want.ServerURL, got.ServerURL)
	}
}

func TestIsSwagger2Document(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "json swagger 2.0",
			data: `{"swagger": "2.0", "info": {"title": "t", "version": "1"}}`,
			want: true,
		},
		{
			name: "yaml swagger 2.0",
			data: "swagger: \"2.0\"\ninfo:\n  title: t\n  version: \"1\"\n",
			want: true,
		},
		{
			name: "openapi 3 json",
			data: `{"openapi": "3.0.0", "info": {"title": "t", "version": "1"}}`,
			want: false,
		},
		{
			name: "openapi 3 yaml",
			data: "openapi: 3.0.0\ninfo:\n  title: t\n",
			want: false,
		},
		{
			name: "no version fields",
			data: `{"info": {"title": "t"}}`,
			want: false,
		},
		{
			name: "garbage is not swagger 2",
			data: "not json or yaml at all: [",
			want: false,
		},
		{
			name: "empty",
			data: "",
			want: false,
		},
		// The cases below pin the corners the streaming root-field scan
		// (PERF-001) must keep identical to the previous map decode.
		{
			// JSON number 2 decodes as float64(2), which the numeric
			// normalization turns into the "2.0" string.
			name: "json numeric swagger 2",
			data: `{"swagger": 2, "info": {"title": "t", "version": "1"}}`,
			want: true,
		},
		{
			name: "json numeric swagger 2.0",
			data: `{"swagger": 2.0, "info": {"title": "t", "version": "1"}}`,
			want: true,
		},
		{
			name: "json numeric swagger other value",
			data: `{"swagger": 3, "info": {"title": "t", "version": "1"}}`,
			want: false,
		},
		{
			// An empty openapi string is not a 3.x declaration: the previous
			// implementation's type assertion yielded "" and a swagger 2.0
			// document with it still counted as 2.0.
			name: "swagger 2 with empty openapi",
			data: `{"swagger": "2.0", "openapi": "", "info": {"title": "t"}}`,
			want: true,
		},
		{
			// A non-string openapi value is not a declaration either.
			name: "swagger 2 with non-string openapi",
			data: `{"swagger": "2.0", "openapi": 3, "info": {"title": "t"}}`,
			want: true,
		},
		{
			// openapi is the last root key and the document carries trailing
			// garbage: the streaming scan settles on non-empty openapi before
			// it can reject the trailing bytes, and the verdict is the same
			// false the map decode produced (there it was the YAML arm that
			// failed and returned no fields at all).
			name: "openapi last with trailing garbage",
			data: `{"info": {"title": "t"}, "openapi": "3.0.3"} trailing garbage`,
			want: false,
		},
		{
			// yaml.v3 accepts the flow mapping and ignores the trailing
			// words, so the pre-PERF-001 YAML fallback arm reported this
			// document as Swagger 2.0. The streaming scan rejects it as JSON
			// (trailing data), falls back, and lands on the same verdict.
			name: "swagger 2 with trailing garbage",
			data: `{"swagger": "2.0", "info": {"title": "t"}} trailing garbage`,
			want: true,
		},
		{
			// Root scalars preceded by a large nested value exercise the
			// skip path (the version fields are the last keys).
			name: "version fields after nested junk",
			data: `{"paths": {"/a": {"get": {"responses": {"200": {"description": "ok"}}}}}, "servers": [{"url": "u"}, {"url": "v"}], "swagger": "2.0"}`,
			want: true,
		},
		{
			name: "json array root",
			data: `[{"swagger": "2.0"}]`,
			want: false,
		},
		{
			name: "json string root",
			data: `"swagger: 2.0"`,
			want: false,
		},
		{
			name: "json null root",
			data: `null`,
			want: false,
		},
		{
			name: "truncated json object",
			data: `{"swagger": "2.0"`,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSwagger2Document([]byte(tt.data)))
		})
	}
}

// referenceSwagger2RootFields is the implementation PERF-001 replaced, kept
// verbatim as the behavioural oracle for the streaming scan: it decodes the
// whole document into a map and reads the two root scalars.
func referenceSwagger2RootFields(data []byte) (swagger string, openapi string, ok bool) {
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

// referenceIsSwagger2Document is the pre-PERF-001 verdict, derived from the
// reference decode exactly as isSwagger2Document derives its own.
func referenceIsSwagger2Document(data []byte) bool {
	swagger, openapi, ok := referenceSwagger2RootFields(data)
	if !ok {
		return false
	}
	return swagger == Swagger2Version && openapi == ""
}

// TestSwagger2RootFieldsParityWithReference is the unit-level half of the
// "no behaviour change" guarantee: over every fixture in the repository, the
// production-size benchmark document, and a table of edge inputs, the
// streaming scan must reach the SAME verdict as the map decode it replaced.
func TestSwagger2RootFieldsParityWithReference(t *testing.T) {
	var corpus [][2]string // name, path

	// Every spec fixture in the repository, at any depth under demo/.
	err := filepath.WalkDir("../../demo", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".json", ".yaml", ".yml":
			corpus = append(corpus, [2]string{"demo:" + path, path})
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, corpus, "fixture walk found no demo specs to compare")
	t.Logf("compared %d demo fixtures against the reference decode", len(corpus))

	for _, tc := range corpus {
		data, err := os.ReadFile(tc[1])
		require.NoError(t, err, "read fixture %s", tc[1])
		assert.Equal(t, referenceIsSwagger2Document(data), isSwagger2Document(data),
			"verdict changed for fixture %s", tc[0])
	}

	// The production-size document the PERF-001 benchmarks use.
	prod := productionSpec()
	assert.Equal(t, referenceIsSwagger2Document(prod), isSwagger2Document(prod),
		"verdict changed for the production-size 3.x document")

	// Edge inputs: each is a shape the streaming scan handles differently
	// from a whole-document decode internally, so each must still agree.
	edges := []struct {
		name string
		data string
	}{
		{"numeric swagger json", `{"swagger": 2.0, "paths": {}}`},
		{"numeric swagger json int", `{"swagger": 2, "paths": {}}`},
		{"yaml swagger numeric", "swagger: 2.0\ninfo:\n  title: t\n"},
		{"yaml swagger string", "swagger: \"2.0\"\ninfo:\n  title: t\n"},
		{"openapi first", `{"openapi": "3.0.3"}`},
		{"openapi last", `{"swagger": "2.0", "info": {}, "openapi": "3.0.3"}`},
		{"openapi last trailing", `{"info": {}, "openapi": "3.0.3"} trailing garbage`},
		{"swagger trailing", `{"swagger": "2.0"} trailing garbage`},
		{"empty openapi", `{"swagger": "2.0", "openapi": ""}`},
		{"nonstring openapi", `{"swagger": "2.0", "openapi": 3}`},
		{"nested junk before fields", `{"x": {"y": [1, {"z": null}, "s", true]}, "swagger": "2.0"}`},
		{"json array", `[1, 2, 3]`},
		{"json scalar", `42`},
		{"json null", `null`},
		{"blank", "   \n	 "},
		{"empty", ""},
		{"invalid", "{{{"},
		{"truncated", `{"swagger": "2.0", "x": {"y":`},
	}
	for _, edge := range edges {
		assert.Equal(t, referenceIsSwagger2Document([]byte(edge.data)), isSwagger2Document([]byte(edge.data)),
			"verdict changed for edge %q", edge.name)
	}
}

// TestSwagger2RootFieldsStreamingValues pins the field VALUES the streaming
// scan reports for well-formed documents (the verdict above only observes
// whether swagger=="2.0" and openapi=="") plus the numeric normalization that
// the YAML fallback and the JSON scan must share.
func TestSwagger2RootFieldsStreamingValues(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		wantSwagger string
		wantOpenAPI string
		wantOK      bool
	}{
		{
			name:        "openapi first",
			data:        `{"openapi": "3.0.3", "info": {"title": "t"}}`,
			wantOpenAPI: "3.0.3",
			wantOK:      true,
		},
		{
			name:        "openapi last",
			data:        `{"info": {"title": "t"}, "openapi": "3.1.0"}`,
			wantOpenAPI: "3.1.0",
			wantOK:      true,
		},
		{
			name:        "numeric swagger json",
			data:        `{"swagger": 2.0, "info": {"title": "t"}}`,
			wantSwagger: Swagger2Version,
			wantOK:      true,
		},
		{
			name:        "numeric swagger yaml",
			data:        "swagger: 2.0\ninfo:\n  title: t\n",
			wantSwagger: Swagger2Version,
			wantOK:      true,
		},
		{
			name:        "both fields",
			data:        `{"swagger": "2.0", "openapi": "3.0.3"}`,
			wantSwagger: Swagger2Version,
			wantOpenAPI: "3.0.3",
			wantOK:      true,
		},
		{
			name:   "not json or yaml",
			data:   "key: [unterminated",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			swagger, openapi, ok := swagger2RootFields([]byte(tt.data))
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantSwagger, swagger)
			assert.Equal(t, tt.wantOpenAPI, openapi)
		})
	}
}

// TestSwagger2RootFieldsDuplicateRootKeys documents the one input class where
// the streaming scan deliberately diverges from the map decode: encoding/json
// resolves duplicate root keys last-one-wins, while the scan settles on the
// first non-empty "openapi" it meets and stops reading. Duplicate root keys
// are not a valid OpenAPI document (the document is ambiguous by construction),
// and honouring them would mean reading the whole document — the exact cost
// PERF-001 removes. The verdict difference needs a 2.0 swagger field plus a
// duplicated openapi field whose LAST value is empty.
func TestSwagger2RootFieldsDuplicateRootKeys(t *testing.T) {
	data := []byte(`{"swagger": "2.0", "openapi": "3.0.3", "openapi": ""}`)
	assert.True(t, referenceIsSwagger2Document(data),
		"premise: the map decode treats the document as Swagger 2.0 (last openapi wins)")
	assert.False(t, isSwagger2Document(data),
		"the streaming scan settles on the first non-empty openapi")
}

func TestConvertSwagger2ToV3_RejectsStructurallyBrokenDocument(t *testing.T) {
	// A 2.0 document whose paths break the 3.x schema must fail LOUDLY at
	// conversion — the measured pre-PROTO-006 defect was silent acceptance
	// with lossy degradation.
	data := []byte(`{"swagger": "2.0", "info": {"title": "t", "version": "1"}, "paths": {"broken"}}`)
	_, err := convertSwagger2ToV3(context.Background(), data)
	assert.Error(t, err, "structurally broken 2.0 document must fail conversion")
}

func TestParser_Parse_Swagger2BrokenPathsFailsLoudly(t *testing.T) {
	p := NewParser()
	data := []byte(`{"swagger": "2.0", "info": {"title": "t", "version": "1"}, "paths": {"broken"}}`)
	result, err := p.Parse(context.Background(), data, DefaultParseOptions())
	assert.Error(t, err, "a broken 2.0 document must be rejected, not silently lossy-parsed")
	assert.Nil(t, result)
}

// TestParser_Parse_OpenAPI3Unchanged pins the frozen surface: a 3.x
// document takes the original loader path and is NOT reported as converted.
func TestParser_Parse_OpenAPI3Unchanged(t *testing.T) {
	data := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Three", "version": "2.0.0"},
		"servers": [{"url": "https://api.example.com"}],
		"paths": {"/health": {"get": {"operationId": "getHealth", "responses": {"200": {"description": "OK"}}}}}
	}`)
	p := NewParser()
	result, err := p.Parse(context.Background(), data, DefaultParseOptions())
	require.NoError(t, err)
	assert.False(t, result.ConvertedFromSwagger2)
	assert.Equal(t, "Three", result.Document.Info.Title)
	assert.Equal(t, "https://api.example.com", result.Document.Servers[0].URL)
}

// TestParser_Parse_Swagger2WithoutPaths keeps a corner of the input space
// honest at the parser level: a 2.0 document with no paths converts (to a
// document with zero operations) — the loud refusal lives in generate,
// which owns the "never an empty command set" contract.
func TestParser_Parse_Swagger2WithoutPaths(t *testing.T) {
	data := []byte(`{
		"swagger": "2.0",
		"info": {"title": "Empty", "version": "1.0.0"},
		"host": "example.com",
		"basePath": "/",
		"paths": {}
	}`)
	p := NewParser()
	result, err := p.Parse(context.Background(), data, DefaultParseOptions())
	require.NoError(t, err)
	assert.True(t, result.ConvertedFromSwagger2)
	ops, err := p.ExtractOperations(result.Document)
	require.NoError(t, err)
	assert.Empty(t, ops)
}
