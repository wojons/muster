package openapi

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// invalidExampleSpec carries exactly one schema-level validation defect: a
// property declared as a string whose example is the number 20.04. kin-openapi
// rejects the document for it (the same defect GitHub's published v3 REST spec
// has on nullable-actions-hosted-runner-pool-image.display_name).
const invalidExampleSpec = `openapi: 3.0.0
info:
  title: Invalid Example API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: listWidgets
      summary: List widgets
      responses:
        '200':
          description: OK
components:
  schemas:
    Widget:
      type: object
      properties:
        name:
          type: string
          example: widget
        version:
          type: string
          example: 20.04
`

// validSpec has no schema-level defects.
const validSpec = `openapi: 3.0.0
info:
  title: Valid API
  version: 2.0.0
paths:
  /widgets:
    get:
      operationId: listWidgets
      summary: List widgets
      responses:
        '200':
          description: OK
`

// twoDefectiveComponentsSpec has one invalid example in each of two component
// schemas — the shape a multi-error validation pass reports exhaustively.
const twoDefectiveComponentsSpec = `openapi: 3.0.0
info:
  title: Two Bad Examples
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        '200':
          description: OK
components:
  schemas:
    Widget:
      type: object
      properties:
        version:
          type: string
          example: 20.04
    Gadget:
      type: object
      properties:
        retries:
          type: integer
          example: many
`

// loadFixtureDoc loads a YAML document the way both surfaces do (kin-openapi
// loader, no validation).
func loadFixtureDoc(t *testing.T, spec string) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(spec))
	require.NoError(t, err)
	require.NotNil(t, doc)
	return doc
}

func TestValidationPolicy_ZeroValueIsWarnAndContinue(t *testing.T) {
	// The zero value must be the permissive policy: every caller that builds a
	// config without thinking about validation (all pre-existing tests, every
	// library consumer) then gets the warn-and-continue behaviour.
	var policy ValidationPolicy
	require.Equal(t, ValidationWarnAndContinue, policy)
	require.Equal(t, "warn-and-continue", policy.String())
	require.Equal(t, "strict", ValidationStrict.String())
}

func TestValidateDocument_WarnAndContinueNamesSchemaAndField(t *testing.T) {
	doc := loadFixtureDoc(t, invalidExampleSpec)

	var diagnostics bytes.Buffer
	err := ValidateDocument(context.Background(), doc, ValidationWarnAndContinue, &diagnostics)
	require.NoError(t, err, "warn-and-continue must not fail the load")

	out := diagnostics.String()
	require.Contains(t, out, "Warning:")
	require.Contains(t, out, `components/schema "Widget"`, "the warning must name the offending schema")
	require.Contains(t, out, "invalid example", "the warning must name the failing schema sub-value")
	require.Contains(t, out, `field "type"`, "the warning must name the failing schema field")
	require.Contains(t, out, "value 20.04", "the warning must quote the offending value")
	require.Contains(t, out, "Widget.properties.version", "the warning must point at the offending property")
	require.Contains(t, out, "MUSTER_STRICT_VALIDATION=1", "the warning must say how to get strict behaviour")

	// One header line + one issue line: the validator's multi-line schema blob
	// must not be dumped at the operator.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 2, "expected exactly one issue line, got:\n%s", out)
	require.NotContains(t, out, "Schema:\n", "the raw schema blob must not leak into the warning")
}

func TestValidateDocument_StrictReturnsTheValidationError(t *testing.T) {
	doc := loadFixtureDoc(t, invalidExampleSpec)

	var diagnostics bytes.Buffer
	err := ValidateDocument(context.Background(), doc, ValidationStrict, &diagnostics)
	require.Error(t, err, "strict mode must refuse the document")
	require.Contains(t, err.Error(), `schema "Widget": invalid example`)
	require.Contains(t, err.Error(), "value must be a string")
	require.Empty(t, diagnostics.String(), "strict mode reports through the error, not a warning")
}

func TestValidateDocument_ValidDocumentIsSilent(t *testing.T) {
	doc := loadFixtureDoc(t, validSpec)

	var diagnostics bytes.Buffer
	require.NoError(t, ValidateDocument(context.Background(), doc, ValidationWarnAndContinue, &diagnostics))
	require.Empty(t, diagnostics.String(), "a valid document produces no warning")

	require.NoError(t, ValidateDocument(context.Background(), doc, ValidationStrict, &diagnostics))
}

func TestValidateDocument_NilDocumentFails(t *testing.T) {
	var diagnostics bytes.Buffer
	err := ValidateDocument(context.Background(), nil, ValidationWarnAndContinue, &diagnostics)
	require.Error(t, err)
	require.Empty(t, diagnostics.String())
}

// A document that fails validation outside the schema-value path (here: no info
// section) must still warn instead of loading silently, and the raw validator
// message is the only diagnostic available.
func TestValidateDocument_WarnAndContinueUnstructuredFailure(t *testing.T) {
	doc := loadFixtureDoc(t, `openapi: 3.0.0
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        '200':
          description: OK
`)

	var diagnostics bytes.Buffer
	require.NoError(t, ValidateDocument(context.Background(), doc, ValidationWarnAndContinue, &diagnostics))

	out := diagnostics.String()
	require.Contains(t, out, "Warning:")
	require.Contains(t, out, "info", "the fallback line must carry the validator's own message")
	require.Len(t, strings.Split(strings.TrimRight(out, "\n"), "\n"), 2)
}

func TestDescribeValidationErrors_NamesComponentAndProperty(t *testing.T) {
	doc := loadFixtureDoc(t, invalidExampleSpec)
	validateErr := doc.Validate(context.Background())
	require.Error(t, validateErr)

	issues := DescribeValidationErrors(doc, validateErr)
	require.Len(t, issues, 1, "one invalid example yields one issue")

	issue := issues[0]
	require.Equal(t, "components", issue.Section)
	require.Equal(t, "schema", issue.Component)
	require.Equal(t, "Widget", issue.Name)
	require.Equal(t, "example", issue.ValueKind)
	require.Equal(t, "type", issue.Field)
	require.Equal(t, "Widget.properties.version", issue.Property)
	require.Equal(t, "value must be a string", issue.Reason)
	require.Equal(t, float64(20.04), issue.Value)

	rendered := issue.String()
	require.Contains(t, rendered, `components/schema "Widget"`)
	require.Contains(t, rendered, `invalid example: field "type"`)
	require.Contains(t, rendered, "value 20.04")
}

// Warn-and-continue validates in multi-error mode, so a document with defects in
// several components reports all of them in one startup. Strict mode keeps the
// validator's fail-fast behaviour (the historical one-error error), which this
// test pins alongside it.
func TestDescribeValidationErrors_ReportsEveryComponentInWarnMode(t *testing.T) {
	doc := loadFixtureDoc(t, twoDefectiveComponentsSpec)

	var diagnostics bytes.Buffer
	require.NoError(t, ValidateDocument(context.Background(), doc, ValidationWarnAndContinue, &diagnostics))

	out := diagnostics.String()
	require.Contains(t, out, "Widget.properties.version", "every defective component is reported:\n%s", out)
	require.Contains(t, out, "Gadget.properties.retries", "every defective component is reported:\n%s", out)
	require.Equal(t, 2, strings.Count(out, "invalid example"), "one line per defect:\n%s", out)

	// Strict mode is unchanged: fail-fast, first component only, via the error.
	var strictDiagnostics bytes.Buffer
	err := ValidateDocument(context.Background(), doc, ValidationStrict, &strictDiagnostics)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Gadget", "strict keeps the validator's fail-fast first error")
	require.NotContains(t, err.Error(), "Widget")
	require.Empty(t, strictDiagnostics.String())
}

// The validator's schema-level pass stops at the first failing property, so a
// schema with two bad examples reports one of them. This pins that limit: the
// extractor reports what the validator produced, and the operator fixes the
// spec one restart at a time.
func TestDescribeValidationErrors_OnePropertyPerSchema(t *testing.T) {
	doc := loadFixtureDoc(t, `openapi: 3.0.0
info:
  title: Two Bad Properties
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        '200':
          description: OK
components:
  schemas:
    Widget:
      type: object
      properties:
        version:
          type: string
          example: 20.04
        retries:
          type: integer
          example: many
`)

	var diagnostics bytes.Buffer
	require.NoError(t, ValidateDocument(context.Background(), doc, ValidationWarnAndContinue, &diagnostics))

	out := diagnostics.String()
	require.Contains(t, out, "Widget.properties.", "the failing property is named:\n%s", out)
	require.Equal(t, 1, strings.Count(out, "invalid example"), "one schema yields one property issue:\n%s", out)
}

func TestDescribeValidationErrors_NilErrorYieldsNothing(t *testing.T) {
	doc := loadFixtureDoc(t, validSpec)
	require.Empty(t, DescribeValidationErrors(doc, nil))
}

func TestLocatePropertyPath(t *testing.T) {
	doc := loadFixtureDoc(t, invalidExampleSpec)

	require.Equal(t, "Widget.properties.version", locatePropertyPath(doc, float64(20.04)))
	require.Equal(t, "", locatePropertyPath(doc, "not-in-any-example"))
	require.Equal(t, "", locatePropertyPath(nil, float64(20.04)))
	require.Equal(t, "", locatePropertyPath(doc, nil))
}

// exampleTypeMismatch must not flag an integer declaration whose example is a
// whole number (JSON has no integer kind of its own) — the locator has to be
// precise or it would point at valid properties.
func TestExampleTypeMismatch(t *testing.T) {
	stringType := openapi3.NewStringSchema()
	stringType.Example = 20.04
	require.True(t, exampleTypeMismatch(stringType, 20.04))

	stringType.Example = "20.04"
	require.False(t, exampleTypeMismatch(stringType, "20.04"))

	intType := openapi3.NewIntegerSchema()
	intType.Example = 5
	require.False(t, exampleTypeMismatch(intType, float64(5)))
	intType.Example = 5.5
	require.True(t, exampleTypeMismatch(intType, 5.5))

	// No declared type, or nothing matching the reported value: no claim.
	untyped := &openapi3.Schema{Example: 20.04}
	require.False(t, exampleTypeMismatch(untyped, 20.04))
	require.False(t, exampleTypeMismatch(stringType, "a different value"))
}

func TestValidationIssue_String(t *testing.T) {
	tests := []struct {
		name  string
		issue ValidationIssue
		want  string
	}{
		{
			name:  "component with value kind and field",
			issue: ValidationIssue{Component: "schema", Name: "Pet", ValueKind: "example", Field: "type", Reason: "value must be a string"},
			want:  `components/schema "Pet": invalid example: field "type": value must be a string`,
		},
		{
			name:  "path with method",
			issue: ValidationIssue{Path: "/pets", Method: "POST", Field: "operationId", Reason: "value is required"},
			want:  `path /pets POST: field "operationId": value is required`,
		},
		{
			name:  "section only",
			issue: ValidationIssue{Section: "components", Reason: "invalid components"},
			want:  "components: invalid components",
		},
		{
			name:  "reason only",
			issue: ValidationIssue{Reason: "invalid info"},
			want:  "invalid info",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.issue.String())
		})
	}
}
