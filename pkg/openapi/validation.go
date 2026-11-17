package openapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// ValidationPolicy selects what a surface does when a loaded document fails
// openapi3 schema validation.
//
// The zero value is ValidationWarnAndContinue deliberately: a spec that is
// semantically imperfect but structurally loadable (GitHub's published v3 REST
// spec carries an invalid `example` on a string property) still describes
// thousands of usable operations, so refusing to start on it hides the whole
// API behind one cosmetic defect. Surfaces that want the strict pre-existing
// behaviour opt in explicitly.
type ValidationPolicy int

const (
	// ValidationWarnAndContinue reports every validation problem on the
	// caller's diagnostics writer and keeps the document (default).
	ValidationWarnAndContinue ValidationPolicy = iota
	// ValidationStrict returns the validation error to the caller, which
	// then refuses to serve the document.
	ValidationStrict
)

// String returns the policy name used in operator-facing text.
func (p ValidationPolicy) String() string {
	switch p {
	case ValidationWarnAndContinue:
		return "warn-and-continue"
	case ValidationStrict:
		return "strict"
	default:
		return fmt.Sprintf("unknown(%d)", int(p))
	}
}

// ValidationIssue is one schema-level problem extracted from a kin-openapi
// validation error.
//
// The validator reports such problems as a chain of context wrappers around a
// *openapi3.SchemaError (which renders the whole offending schema as a JSON
// blob). The wrappers carry the useful identity of the failure — which
// component, path, operation, and schema sub-field — so this struct lifts that
// into fields and String renders one audit-friendly line.
type ValidationIssue struct {
	// Section is the document section the failure was found in, from
	// openapi3.SectionValidationError (e.g. "components", "paths").
	Section string
	// Component is the component bucket, from
	// openapi3.ComponentValidationError (e.g. "schema", "parameter").
	Component string
	// Name is the component's map key (e.g. "actions-hosted-runner").
	Name string
	// Path is the path template when the failure is inside paths.
	Path string
	// Method is the uppercase HTTP method when the failure is inside an
	// operation.
	Method string
	// ValueKind names the schema sub-value that failed, e.g. "example" or
	// "default" (openapi3.SchemaValueError.ValueKind).
	ValueKind string
	// Field is the schema field that failed, e.g. "type" or "required"
	// (openapi3.SchemaError.SchemaField).
	Field string
	// Property is a dotted path to the offending property inside the
	// document's component schemas, when it can be located — e.g.
	// "nullable-actions-hosted-runner-pool-image.properties.display_name".
	// Empty when the property cannot be pinned down.
	Property string
	// Reason is the validator's reason, e.g. "value must be a string".
	Reason string
	// Value is the offending value, when the validator exposes it.
	Value any
}

// String renders the issue as one line naming the offending component and
// field, e.g.:
//
//	components/schema "actions-hosted-runner": invalid example: field "type": value must be a string (property nullable-actions-hosted-runner-pool-image.properties.display_name, value 20.04)
func (i ValidationIssue) String() string {
	var b strings.Builder
	switch {
	case i.Path != "":
		b.WriteString("path " + i.Path)
		if i.Method != "" {
			b.WriteString(" " + i.Method)
		}
	case i.Component != "":
		fmt.Fprintf(&b, "components/%s %q", i.Component, i.Name)
	case i.Section != "":
		b.WriteString(i.Section)
	}
	if b.Len() > 0 {
		b.WriteString(": ")
	}

	switch {
	case i.ValueKind != "" && i.Field != "":
		fmt.Fprintf(&b, "invalid %s: field %q: %s", i.ValueKind, i.Field, i.Reason)
	case i.ValueKind != "":
		fmt.Fprintf(&b, "invalid %s: %s", i.ValueKind, i.Reason)
	case i.Field != "":
		fmt.Fprintf(&b, "field %q: %s", i.Field, i.Reason)
	default:
		b.WriteString(i.Reason)
	}

	var details []string
	if i.Property != "" {
		details = append(details, "property "+i.Property)
	}
	if i.Value != nil {
		details = append(details, "value "+truncateValue(i.Value))
	}
	if len(details) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(details, ", "))
	}
	return b.String()
}

// ValidateDocument validates doc and applies policy.
//
// Under ValidationWarnAndContinue every problem is written to diagnostics as a
// "Warning:" line naming the offending schema and field, and nil is returned so
// the caller keeps going. Multi-error mode is enabled for that pass so an
// operator sees every defect in one startup instead of fixing them one restart
// at a time.
//
// Under ValidationStrict the raw validation error is returned unchanged
// (validator default fail-fast semantics — the historical behaviour) and
// nothing is written.
//
// The diagnostics writer is never stdout by convention at the call sites: the
// MCP server passes its stderr, because its stdout is the JSON-RPC wire.
func ValidateDocument(ctx context.Context, doc *openapi3.T, policy ValidationPolicy, diagnostics io.Writer) error {
	if doc == nil {
		return errors.New("validate spec: nil document")
	}
	var validateErr error
	if policy == ValidationStrict {
		// Strict mode must stay byte-for-byte equivalent to the historical
		// behaviour: kin-openapi's default fail-fast validation.
		validateErr = doc.Validate(ctx)
	} else {
		validateErr = doc.Validate(ctx, openapi3.EnableMultiError())
	}
	if validateErr == nil {
		return nil
	}
	if policy == ValidationStrict {
		return validateErr
	}
	WriteValidationWarning(diagnostics, doc, validateErr)
	return nil
}

// WriteValidationWarning writes the warn-and-continue report for a validation
// failure: a header line stating the policy (and how to get the strict
// behaviour instead), then one line per extracted issue.
//
// When the error chain carries no schema-level detail (a missing info section,
// an unsupported version, …) the raw error is reported instead, so a validation
// failure is never silent.
func WriteValidationWarning(w io.Writer, doc *openapi3.T, validateErr error) {
	if w == nil || validateErr == nil {
		return
	}
	issues := DescribeValidationErrors(doc, validateErr)
	_, _ = fmt.Fprintln(w, "Warning: OpenAPI spec failed schema validation; continuing "+
		"(warn-and-continue policy — pass -strict-validation or set "+
		"MUSTER_STRICT_VALIDATION=1 to refuse startup)")
	if len(issues) == 0 {
		_, _ = fmt.Fprintf(w, "Warning:   %s\n", singleLine(validateErr.Error()))
		return
	}
	for i, issue := range issues {
		if i == maxReportedIssues {
			// A real-world spec can carry hundreds of cosmetic defects (the
			// GitHub v3 REST spec has 253). Report the head of the list and
			// the true count: bounded stderr, nothing hidden.
			_, _ = fmt.Fprintf(w, "Warning:   … and %d more validation issues (not listed)\n", len(issues)-maxReportedIssues)
			break
		}
		_, _ = fmt.Fprintf(w, "Warning:   %s\n", issue)
	}
}

// maxReportedIssues bounds how many individual issues a warn-and-continue
// report prints. The count of the rest is still reported.
const maxReportedIssues = 10

// DescribeValidationErrors lifts a kin-openapi validation error into
// ValidationIssues.
//
// The walker understands the context wrappers the loader's validation
// produces (section / path / operation / component / schema-value) and stops at
// each *openapi3.SchemaError leaf, which is where the concrete reason lives.
// Repeated identical issues are reported once.
func DescribeValidationErrors(doc *openapi3.T, validateErr error) []ValidationIssue {
	if validateErr == nil {
		return nil
	}

	var issues []ValidationIssue
	seen := make(map[string]bool)
	add := func(issue ValidationIssue) {
		key := issue.String()
		if seen[key] {
			return
		}
		seen[key] = true
		issues = append(issues, issue)
	}

	type contextState struct {
		section   string
		component string
		name      string
		path      string
		method    string
		valueKind string
	}

	var walk func(err error, state contextState)
	walk = func(err error, state contextState) {
		if err == nil {
			return
		}

		switch e := err.(type) {
		case openapi3.MultiError:
			for _, inner := range e {
				walk(inner, state)
			}
			return
		case *openapi3.SectionValidationError:
			state.section = e.Section
			walk(e.Cause, state)
			return
		case *openapi3.PathValidationError:
			state.path = e.Path
			walk(e.Cause, state)
			return
		case *openapi3.OperationValidationError:
			state.method = e.Method
			walk(e.Cause, state)
			return
		case *openapi3.ComponentValidationError:
			state.component = e.Section
			state.name = e.Name
			walk(e.Cause, state)
			return
		}

		var valueErr *openapi3.SchemaValueError
		if errors.As(err, &valueErr) {
			state.valueKind = valueErr.ValueKind
			walk(valueErr.Cause, state)
			return
		}

		var schemaErr *openapi3.SchemaError
		if errors.As(err, &schemaErr) {
			add(ValidationIssue{
				Section:   state.section,
				Component: state.component,
				Name:      state.name,
				Path:      state.path,
				Method:    state.method,
				ValueKind: state.valueKind,
				Field:     schemaErr.SchemaField,
				Property:  locatePropertyPath(doc, schemaErr.Value),
				Reason:    schemaErr.Reason,
				Value:     schemaErr.Value,
			})
			return
		}

		if inner := errors.Unwrap(err); inner != nil && inner != err {
			walk(inner, state)
			return
		}

		// An unstructured leaf: keep the message so the failure is visible.
		add(ValidationIssue{
			Section:   state.section,
			Component: state.component,
			Name:      state.name,
			Path:      state.path,
			Method:    state.method,
			ValueKind: state.valueKind,
			Reason:    singleLine(err.Error()),
		})
	}

	walk(validateErr, contextState{})
	return issues
}

// maxLocateDepth bounds the component-schema walk used to name the offending
// property. Real documents nest a handful of levels; the bound keeps a
// pathological schema from turning a warning into a startup cost.
const maxLocateDepth = 4

// locatePropertyPath finds the component property that carries value as its
// example (or default) while declaring an incompatible type — the defect class
// that makes kin-openapi reject a document during validation. It returns a
// dotted path such as
// "nullable-actions-hosted-runner-pool-image.properties.display_name", or ""
// when no property matches.
//
// The search covers the whole components/schemas map because the validator
// attributes a failure reached through a $ref to the REFERRING component: on
// GitHub's v3 spec the bad example lives on
// nullable-actions-hosted-runner-pool-image.properties.display_name, yet the
// error names actions-hosted-runner.
func locatePropertyPath(doc *openapi3.T, value any) string {
	if doc == nil || value == nil || doc.Components == nil || len(doc.Components.Schemas) == 0 {
		return ""
	}

	names := make([]string, 0, len(doc.Components.Schemas))
	for name := range doc.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	visited := make(map[*openapi3.Schema]bool)
	for _, name := range names {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		if path := locateInSchema(ref.Value, name, 0, value, visited); path != "" {
			return path
		}
	}
	return ""
}

// locateInSchema walks one schema (properties and array items, bounded by
// maxLocateDepth) looking for a property whose example/default is value while
// its declared type cannot hold it.
func locateInSchema(schema *openapi3.Schema, path string, depth int, value any, visited map[*openapi3.Schema]bool) string {
	if schema == nil || depth > maxLocateDepth || visited[schema] {
		return ""
	}
	visited[schema] = true

	if exampleTypeMismatch(schema, value) {
		return path
	}

	propNames := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)
	for _, name := range propNames {
		ref := schema.Properties[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		child := path + ".properties." + name
		if found := locateInSchema(ref.Value, child, depth+1, value, visited); found != "" {
			return found
		}
	}

	if schema.Items != nil && schema.Items.Value != nil {
		if found := locateInSchema(schema.Items.Value, path+".items", depth+1, value, visited); found != "" {
			return found
		}
	}
	return ""
}

// exampleTypeMismatch reports whether schema carries value as its example (or
// default) while declaring a type that cannot hold a value of that JSON kind.
func exampleTypeMismatch(schema *openapi3.Schema, value any) bool {
	if schema == nil || schema.Type == nil || len(*schema.Type) != 1 {
		return false
	}
	if !valuesEqual(schema.Example, value) && !valuesEqual(schema.Default, value) {
		return false
	}
	return !kindMatches((*schema.Type)[0], value)
}

// kindMatches reports whether a declared OpenAPI type can hold v. "integer" is
// satisfied by an integral number (JSON has no integer type of its own).
func kindMatches(declared string, v any) bool {
	got := jsonKind(v)
	if declared == got {
		return true
	}
	if declared == "integer" {
		if f, ok := asFloat(v); ok {
			return f == math.Trunc(f)
		}
	}
	return false
}

// jsonKind maps a decoded JSON value onto an OpenAPI type name. Values whose
// kind is not modelled return "" so they never produce a false match.
func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return ""
	}
}

// asFloat converts a JSON/YAML numeric value to float64.
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// valuesEqual compares an example from the document with the value the
// validator reported. Numeric comparisons are value-based so a JSON float64
// and a YAML integer describing the same number still match.
func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	if fa, ok := asFloat(a); ok {
		fb, ok := asFloat(b)
		return ok && fa == fb
	}
	return reflect.DeepEqual(a, b)
}

// truncateValue renders a value for a single warning line, capped so one huge
// example cannot flood an operator's terminal.
func truncateValue(v any) string {
	s := singleLine(fmt.Sprintf("%v", v))
	const max = 80
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// singleLine collapses whitespace so a multi-line error renders as one line.
func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
