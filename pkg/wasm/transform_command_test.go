// Package wasm_test — regression coverage for the WASI stdin/stdout plugin
// contract.
//
// REVIEW-MUSTER-6: the runtime executes every plugin as a WASI *command*
// (pkg/wasm/module.go, runWASMModule) — the response body goes to stdin and
// stdout replaces it. A module that exports only a helper function and no
// `_start` instantiates cleanly, executes nothing and yields an EMPTY body
// with a nil error, so the interceptor silently dropped real response bodies.
// These tests pin the contract the runtime actually implements: a command
// filter that reads stdin and writes a deterministic, non-empty stdout.
//
// axiom:trace work_item=AC-007 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-3
package wasm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/wasm"
)

const (
	// The demo plugin: a real WASI command filter (hand-written WAT source).
	transformWatSource  = "../../demo/wasm-modules/transform.wat"
	transformWasmBinary = "../../demo/wasm-modules/transform.wasm"

	// The exact envelope the filter wraps every non-empty body in.
	transformEnvelopePrefix = `{"wasm":true,"payload":`
	transformEnvelopeSuffix = `}`
)

// expectedTransformBody is the byte-exact body the plugin must produce for
// the given input.
func expectedTransformBody(input string) []byte {
	return []byte(transformEnvelopePrefix + input + transformEnvelopeSuffix)
}

// compileTransformPlugin compiles the committed WAT source into a temp
// directory, proving the source (not just the shipped binary) is a real
// command filter. Skips when wat2wasm is unavailable.
func compileTransformPlugin(t *testing.T) string {
	t.Helper()

	wat2wasm, err := exec.LookPath("wat2wasm")
	if err != nil {
		t.Skip("wat2wasm not installed: skipping the WAT-source compilation arm")
	}

	out := filepath.Join(t.TempDir(), "transform-from-wat.wasm")
	combined, err := exec.Command(wat2wasm, transformWatSource, "-o", out).CombinedOutput()
	if err != nil {
		t.Fatalf("wat2wasm %s: %v\n%s", transformWatSource, err, combined)
	}
	return out
}

// transformWithPlugin loads pluginPath into a fresh interceptor and returns
// the transformed body.
func transformWithPlugin(t *testing.T, pluginPath, body, contentType string) []byte {
	t.Helper()

	i := wasm.NewInterceptor()
	defer func() { _ = i.Close(context.Background()) }()

	require.NoError(t, i.LoadPlugin(context.Background(), pluginPath))
	result, err := i.TransformResponseBody(context.Background(), []byte(body), contentType, pluginPath)
	require.NoError(t, err)
	return result
}

// TestInterceptor_TransformResponseBody_StdinStdoutPlugin is the regression
// test for REVIEW-MUSTER-6: a stdin/stdout WASI plugin must produce a
// NON-EMPTY body for non-empty input, byte-equal to the documented transform.
func TestInterceptor_TransformResponseBody_StdinStdoutPlugin(t *testing.T) {
	const body = `{"hello":"world"}`

	t.Run("committed_binary_transforms_body_byte_exact", func(t *testing.T) {
		require.FileExists(t, transformWasmBinary,
			"the demo plugin binary must be committed (build it with demo/wasm-modules/build.sh)")

		got := transformWithPlugin(t, transformWasmBinary, body, "application/json")

		require.NotEmpty(t, got,
			"REVIEW-MUSTER-6 regression: the WASI plugin returned an EMPTY body for non-empty input")
		require.True(t, bytes.Equal(expectedTransformBody(body), got),
			"want %q, got %q", expectedTransformBody(body), got)
	})

	t.Run("wat_source_compiles_to_the_same_transform", func(t *testing.T) {
		plugin := compileTransformPlugin(t)

		got := transformWithPlugin(t, plugin, body, "application/json")

		require.NotEmpty(t, got)
		require.True(t, bytes.Equal(expectedTransformBody(body), got),
			"the WAT source must itself be a command filter: want %q, got %q",
			expectedTransformBody(body), got)
	})

	t.Run("result_is_not_a_pass_through", func(t *testing.T) {
		require.FileExists(t, transformWasmBinary)

		got := transformWithPlugin(t, transformWasmBinary, body, "application/json")

		require.NotEmpty(t, got,
			"REVIEW-MUSTER-6 regression: an emptied body is not a transformed body")
		require.NotEqual(t, body, string(got),
			"the plugin must transform the body, not echo it unchanged")
	})

	t.Run("json_array_body_yields_valid_json_envelope", func(t *testing.T) {
		require.FileExists(t, transformWasmBinary)

		// The live Task Forge returns a JSON array; the envelope must stay
		// valid JSON for array bodies too (it is what the demo pipes through).
		got := transformWithPlugin(t, transformWasmBinary, `[]`, "application/json")

		require.True(t, bytes.Equal(expectedTransformBody(`[]`), got),
			"want %q, got %q", expectedTransformBody(`[]`), got)
		assert.True(t, json.Valid(got), "transformed body must be valid JSON: %q", got)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(got, &parsed))
		assert.Equal(t, true, parsed["wasm"])
	})

	t.Run("multi_kilobyte_body_round_trips", func(t *testing.T) {
		require.FileExists(t, transformWasmBinary)

		// Exercise the read loop with a body larger than one small read.
		big := `{"items":["` + strings.Repeat("abcdefghij", 300) + `"]}`
		got := transformWithPlugin(t, transformWasmBinary, big, "application/json")

		require.NotEmpty(t, got)
		require.True(t, bytes.Equal(expectedTransformBody(big), got),
			"payload of %d bytes must round-trip exactly (got %d bytes)", len(big), len(got))
	})
}

// TestInterceptor_TransformResponseBody_EmptyInputStaysEmpty pins the other
// half of the contract: an empty body must never turn into output.
func TestInterceptor_TransformResponseBody_EmptyInputStaysEmpty(t *testing.T) {
	require.FileExists(t, transformWasmBinary)

	t.Run("interceptor_passes_empty_body_through", func(t *testing.T) {
		i := wasm.NewInterceptor()
		defer func() { _ = i.Close(context.Background()) }()
		require.NoError(t, i.LoadPlugin(context.Background(), transformWasmBinary))

		result, err := i.TransformResponseBody(context.Background(), []byte{}, "application/json", transformWasmBinary)
		require.NoError(t, err)
		assert.Empty(t, result, "an empty body must stay empty")
	})

	t.Run("module_writes_nothing_for_empty_stdin", func(t *testing.T) {
		ctx := context.Background()
		mgr := wasm.NewModuleManager(nil)
		lm, err := mgr.Load(ctx, transformWasmBinary)
		require.NoError(t, err)

		out, err := mgr.Transform(ctx, lm, []byte{})
		require.NoError(t, err)
		assert.Empty(t, out, "the command filter must not emit an envelope for empty stdin")
	})

	t.Run("module_transform_is_byte_exact", func(t *testing.T) {
		ctx := context.Background()
		mgr := wasm.NewModuleManager(nil)
		lm, err := mgr.Load(ctx, transformWasmBinary)
		require.NoError(t, err)

		const body = `{"name":"test","value":42}`
		out, err := mgr.Transform(ctx, lm, []byte(body))
		require.NoError(t, err)
		require.True(t, bytes.Equal(expectedTransformBody(body), out),
			"want %q, got %q", expectedTransformBody(body), out)
	})
}
