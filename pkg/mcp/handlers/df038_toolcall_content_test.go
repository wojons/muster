package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// DF-038. The agent consuming a tools/call result must be able to tell a
// client-side failure (bad arguments, missing credential, unresolvable
// operation) from an upstream API failure without parsing prose, and must
// receive the upstream's JSON as structured content instead of one flat text
// blob.

// df038ClassifiedError is a stand-in for a protocol adapter's classified
// failure: it carries the machine-readable marker the handler must render into
// the result. It is duck-typed (like the executor interface the handler
// already accepts) so adapters outside this package can supply markers without
// depending on a concrete error type.
type df038ClassifiedError struct {
	meta map[string]interface{}
}

func (e *df038ClassifiedError) Error() string { return "upstream rejected the request" }

func (e *df038ClassifiedError) ToolErrorMeta() map[string]interface{} { return e.meta }

// df038ResultJSON drives one tools/call through the real handler and returns
// the result exactly as it goes on the wire (marshaled JSON), so the assertions
// below pin the wire shape rather than a Go-side convenience.
func df038ResultJSON(t *testing.T, execErr error, executeResult interface{}) map[string]interface{} {
	t.Helper()

	h := NewCallToolHandler(&mockExecutor{executeResult: executeResult, executeErr: execErr})
	result, err := h.Handle(context.Background(), json.RawMessage(`{"name":"healthCheck","arguments":{}}`))
	require.NoError(t, err)

	raw, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	return parsed
}

// df038FirstText returns the first text content block of a tool result.
func df038FirstText(t *testing.T, parsed map[string]interface{}) string {
	t.Helper()

	blocks, ok := parsed["content"].([]interface{})
	require.True(t, ok, "content must be a JSON array, got %#v", parsed["content"])
	require.NotEmpty(t, blocks)
	block, ok := blocks[0].(map[string]interface{})
	require.True(t, ok, "content block must be an object, got %#v", blocks[0])
	require.Equal(t, "text", block["type"], "text content must remain present (additive change)")
	text, ok := block["text"].(string)
	require.True(t, ok, "text block must carry a string, got %#v", block["text"])
	return text
}

// df038ErrorMarker extracts structuredContent.error from a tool result.
func df038ErrorMarker(t *testing.T, parsed map[string]interface{}) map[string]interface{} {
	t.Helper()

	structured, ok := parsed["structuredContent"].(map[string]interface{})
	require.True(t, ok, "result must carry structuredContent, got %#v", parsed)
	marker, ok := structured["error"].(map[string]interface{})
	require.True(t, ok, "structuredContent must carry an error marker, got %#v", structured)
	return marker
}

// TestCallToolHandler_ClassifiedError_RendersMachineReadableMarker covers the
// typed-marker requirement: a classified failure keeps isError + text (backward
// compatibility) and adds a machine-readable class/code/reason marker.
func TestCallToolHandler_ClassifiedError_RendersMachineReadableMarker(t *testing.T) {
	classified := &df038ClassifiedError{meta: map[string]interface{}{
		"class":            "client_error",
		"code":             "missing_credential",
		"message":          "HTTP 401: {\"error\":\"unauthorized\"}",
		"status":           401,
		"credentialSource": "MUSTER_AUTH_TOKEN environment variable or the --auth-token-file flag",
		"upstreamBody":     map[string]interface{}{"error": "unauthorized"},
	}}

	parsed := df038ResultJSON(t, classified, nil)

	require.Equal(t, true, parsed["isError"], "isError:true must still be set on failures")
	require.Contains(t, df038FirstText(t, parsed), "Error: upstream rejected the request")

	marker := df038ErrorMarker(t, parsed)
	require.Equal(t, "client_error", marker["class"])
	require.Equal(t, "missing_credential", marker["code"])
	require.Equal(t, float64(401), marker["status"])
	require.Contains(t, marker["credentialSource"], "MUSTER_AUTH_TOKEN")
	upstream, ok := marker["upstreamBody"].(map[string]interface{})
	require.True(t, ok, "upstreamBody must stay structured, got %#v", marker["upstreamBody"])
	require.Equal(t, "unauthorized", upstream["error"])
}

// TestCallToolHandler_UnclassifiedError_StillMarked covers the fallback: a
// plain error from an adapter that does not classify still yields a marker that
// says so, instead of prose only.
func TestCallToolHandler_UnclassifiedError_StillMarked(t *testing.T) {
	parsed := df038ResultJSON(t, errors.New("tool not found"), nil)

	require.Equal(t, true, parsed["isError"])
	require.Contains(t, df038FirstText(t, parsed), "Error: tool not found")

	marker := df038ErrorMarker(t, parsed)
	require.Equal(t, "tool_error", marker["class"])
	require.Equal(t, "unclassified", marker["code"])
}

// TestCallToolHandler_AppErrorCodes_ClassifyClientVsUpstream covers the
// existing AppError convention (the gRPC/JSON-RPC adapters): argument and
// lookup failures are client-side, transport/upstream failures are not.
func TestCallToolHandler_AppErrorCodes_ClassifyClientVsUpstream(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantClass string
		wantCode  string
	}{
		{
			name:      "validation is client-side",
			err:       apperrors.New(apperrors.CodeValidation, "missing required argument"),
			wantClass: "client_error",
			wantCode:  "invalid_arguments",
		},
		{
			name:      "not found is client-side",
			err:       apperrors.New(apperrors.CodeNotFound, "operation not found"),
			wantClass: "client_error",
			wantCode:  "unknown_operation",
		},
		{
			name:      "database failure is upstream",
			err:       apperrors.New(apperrors.CodeDatabase, "connection reset"),
			wantClass: "upstream_error",
			wantCode:  "upstream_error",
		},
		{
			name:      "wrapped validation is client-side",
			err:       fmt.Errorf("execute: %w", apperrors.New(apperrors.CodeValidation, "bad args")),
			wantClass: "client_error",
			wantCode:  "invalid_arguments",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := df038ResultJSON(t, tc.err, nil)
			require.Equal(t, true, parsed["isError"])

			marker := df038ErrorMarker(t, parsed)
			require.Equal(t, tc.wantClass, marker["class"])
			require.Equal(t, tc.wantCode, marker["code"])
		})
	}
}

// TestCallToolHandler_JSONObjectResult_StructuredContent covers the response
// half: an upstream JSON object is preserved as structured content while the
// text block keeps the raw JSON (additive, so existing consumers still work).
func TestCallToolHandler_JSONObjectResult_StructuredContent(t *testing.T) {
	parsed := df038ResultJSON(t, nil, map[string]interface{}{
		"status": "ok",
		"count":  float64(2),
		"nested": map[string]interface{}{"a": "b"},
	})

	require.NotContains(t, parsed, "isError", "success must not set isError")

	structured, ok := parsed["structuredContent"].(map[string]interface{})
	require.True(t, ok, "JSON object result must be preserved as structured content, got %#v", parsed)
	require.Equal(t, "ok", structured["status"])
	require.Equal(t, float64(2), structured["count"])

	var textPayload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(df038FirstText(t, parsed)), &textPayload))
	require.Equal(t, structured, textPayload, "text content must carry the same JSON payload")
}

// TestCallToolHandler_NonObjectJSONResult_KeptStructured covers a JSON payload
// that is not an object: structuredContent must be a JSON object, so the
// payload is carried under "result" instead of being flattened to text only.
func TestCallToolHandler_NonObjectJSONResult_KeptStructured(t *testing.T) {
	parsed := df038ResultJSON(t, nil, []interface{}{
		map[string]interface{}{"id": float64(1)},
		map[string]interface{}{"id": float64(2)},
	})

	structured, ok := parsed["structuredContent"].(map[string]interface{})
	require.True(t, ok, "JSON array result must stay structured, got %#v", parsed)
	items, ok := structured["result"].([]interface{})
	require.True(t, ok, "non-object JSON is carried under result, got %#v", structured)
	require.Len(t, items, 2)
}

// TestCallToolHandler_NonStructuredResult_TextFallback covers an executor that
// reports its payload did NOT come from JSON: no structured content is
// advertised, and the text block carries the payload.
func TestCallToolHandler_NonStructuredResult_TextFallback(t *testing.T) {
	parsed := df038ResultJSON(t, nil, df038UnstructuredResult{})

	require.NotContains(t, parsed, "isError")
	require.NotContains(t, parsed, "structuredContent",
		"a non-JSON upstream payload must not be advertised as structured content")
	require.NotEmpty(t, df038FirstText(t, parsed))
}

// df038UnstructuredResult implements the same payload contract the MCP
// executors use to report "this value did not come from upstream JSON".
type df038UnstructuredResult struct{}

func (df038UnstructuredResult) StructuredPayload() (interface{}, bool) {
	return map[string]interface{}{"status": 200, "body": "plain text"}, false
}
