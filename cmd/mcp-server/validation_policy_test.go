package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/openapi"
)

// invalidExampleSpec has exactly one schema-level validation defect: the
// `version` property is declared as a string but its example is the number
// 20.04 — the same defect GitHub's published v3 REST spec has on
// nullable-actions-hosted-runner-pool-image.display_name.
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

// writeInvalidExampleSpec writes the fixture spec into a temp dir and returns
// its path.
func writeInvalidExampleSpec(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "invalid-example.yaml")
	require.NoError(t, os.WriteFile(path, []byte(invalidExampleSpec), 0o644))
	return path
}

// mcpHandshake is the client conversation used by the stdio tests: initialize,
// the initialized notification, then tools/list.
const mcpHandshake = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"qa","version":"1.0.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
`

// jsonrpcResponse is the subset of a JSON-RPC response these tests read.
type jsonrpcResponse struct {
	ID     any             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// syncBuffer is a mutex-protected bytes.Buffer: mcp.Server writes responses
// from its own writer goroutine while the test reads, so a plain buffer would
// both race and truncate.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForResponses waits until the server has written at least n response lines.
// mcp.Server.Run returns once its handlers are done, but the dedicated stdout
// writer is deliberately outside that wait (QA-MUSTER-19-R2), so the last
// response can land a moment later.
func waitForResponses(t *testing.T, buf *syncBuffer, n int) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := buf.String()
		if strings.Count(strings.TrimRight(out, "\n"), "\n")+1 >= n && strings.TrimSpace(out) != "" {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d JSON-RPC responses, got: %q", n, out)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// parseJSONRPCLines decodes newline-delimited JSON-RPC responses.
func parseJSONRPCLines(t *testing.T, out string) []jsonrpcResponse {
	t.Helper()
	var responses []jsonrpcResponse
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var resp jsonrpcResponse
		require.NoError(t, json.Unmarshal([]byte(line), &resp), "stdout must carry only JSON-RPC: %q", line)
		responses = append(responses, resp)
	}
	return responses
}

func TestValidationPolicyFor(t *testing.T) {
	tests := []struct {
		name         string
		flagExplicit bool
		strictFlag   bool
		env          string
		want         openapi.ValidationPolicy
	}{
		{name: "default is warn-and-continue", want: openapi.ValidationWarnAndContinue},
		{name: "env 1 opts in", env: "1", want: openapi.ValidationStrict},
		{name: "env true opts in", env: "TRUE", want: openapi.ValidationStrict},
		{name: "env yes opts in", env: " yes ", want: openapi.ValidationStrict},
		{name: "env on opts in", env: "on", want: openapi.ValidationStrict},
		{name: "env 0 stays permissive", env: "0", want: openapi.ValidationWarnAndContinue},
		{name: "env garbage stays permissive", env: "maybe", want: openapi.ValidationWarnAndContinue},
		{
			name: "explicit flag wins over env", flagExplicit: true, strictFlag: true, env: "0",
			want: openapi.ValidationStrict,
		},
		{
			name: "explicit false overrides env opt-in", flagExplicit: true, strictFlag: false, env: "1",
			want: openapi.ValidationWarnAndContinue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, validationPolicyFor(tt.flagExplicit, tt.strictFlag, tt.env))
		})
	}
}

func TestResolveValidationPolicy_EnvAndFlag(t *testing.T) {
	origStrict := *strictSpecFlag
	defer func() { *strictSpecFlag = origStrict }()

	*strictSpecFlag = false
	t.Setenv("MUSTER_STRICT_VALIDATION", "")
	require.Equal(t, openapi.ValidationWarnAndContinue, resolveValidationPolicy(),
		"warn-and-continue is the default policy")

	t.Setenv("MUSTER_STRICT_VALIDATION", "1")
	require.Equal(t, openapi.ValidationStrict, resolveValidationPolicy(),
		"MUSTER_STRICT_VALIDATION=1 opts into strict")

	// A flag value alone is not enough here: the flag is not parsed in this
	// test process, so explicitness comes from the env var only. The flag's
	// own effect is covered by validationPolicyFor and the binary tests.
	*strictSpecFlag = true
	require.Equal(t, openapi.ValidationStrict, resolveValidationPolicy())
}

func TestLoadConfig_DefaultValidationPolicyIsWarnAndContinue(t *testing.T) {
	t.Setenv("MUSTER_STRICT_VALIDATION", "")
	cfg := loadConfig()
	require.Equal(t, openapi.ValidationWarnAndContinue, cfg.ValidationPolicy)
	require.NotNil(t, cfg.ValidationDiagnostics, "production diagnostics go to a real writer, never nil")
}

// The regression test the defect needs: a spec with one invalid example must
// start, register its tools, and answer initialize + tools/list — while the
// warning names the schema and field on the diagnostics writer, never on the
// JSON-RPC wire.
func TestLoadAndRegisterTools_InvalidExampleWarnsAndServes(t *testing.T) {
	specPath := writeInvalidExampleSpec(t)

	var diagnostics bytes.Buffer
	config := &MCPServerConfig{
		SpecPath:              specPath,
		BaseURL:               "http://localhost:8080",
		ValidationDiagnostics: &diagnostics,
	}

	var stdout, stderr syncBuffer
	server := mcp.NewServer(strings.NewReader(mcpHandshake), &stdout, &stderr)

	count, err := loadAndRegisterTools(server, config)
	require.NoError(t, err, "warn-and-continue must not refuse the document")
	require.Equal(t, 1, count, "the one operation still becomes a tool")

	warning := diagnostics.String()
	require.Contains(t, warning, "Warning:", "a validation problem must be reported, not swallowed")
	require.Contains(t, warning, `components/schema "Widget"`, "the warning must name the offending schema")
	require.Contains(t, warning, "Widget.properties.version", "the warning must name the offending property")
	require.Contains(t, warning, `field "type"`, "the warning must name the failing schema field")
	require.Contains(t, warning, "value 20.04")
	require.Contains(t, warning, "MUSTER_STRICT_VALIDATION=1", "the warning must document the strict opt-in")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, server.Run(ctx))

	// The client conversation survives the warning: initialize and tools/list
	// both answered, and stdout carries nothing but JSON-RPC.
	out := waitForResponses(t, &stdout, 2)
	responses := parseJSONRPCLines(t, out)
	require.Len(t, responses, 2, "expected initialize + tools/list responses, got: %q", out)

	// Handlers run concurrently, so responses are not guaranteed in request
	// order: address them by id.
	initResp := responseByID(t, responses, 1)
	listResp := responseByID(t, responses, 2)
	require.Nil(t, initResp.Error)
	require.Nil(t, listResp.Error)

	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	require.NoError(t, json.Unmarshal(initResp.Result, &initResult))
	require.NotEmpty(t, initResult.ProtocolVersion)
	require.Equal(t, "openapi-mcp", initResult.ServerInfo.Name)

	var listResult struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(listResp.Result, &listResult))
	require.Len(t, listResult.Tools, count, "tools/list must expose every converted operation")
	require.NotContains(t, out, "Warning:", "warnings must never reach the JSON-RPC wire")
}

// responseByID returns the response carrying the given JSON-RPC id, failing the
// test when it is absent.
func responseByID(t *testing.T, responses []jsonrpcResponse, id float64) jsonrpcResponse {
	t.Helper()
	for _, resp := range responses {
		if got, ok := resp.ID.(float64); ok && got == id {
			return resp
		}
	}
	t.Fatalf("no JSON-RPC response with id %v in %+v", id, responses)
	return jsonrpcResponse{}
}

func TestLoadAndRegisterTools_InvalidExampleStrictRefuses(t *testing.T) {
	specPath := writeInvalidExampleSpec(t)

	var diagnostics bytes.Buffer
	config := &MCPServerConfig{
		SpecPath:              specPath,
		ValidationPolicy:      openapi.ValidationStrict,
		ValidationDiagnostics: &diagnostics,
	}

	var stdout, stderr bytes.Buffer
	server := mcp.NewServer(strings.NewReader(mcpHandshake), &stdout, &stderr)

	count, err := loadAndRegisterTools(server, config)
	require.Error(t, err, "strict mode must refuse a document that fails validation")
	require.Zero(t, count)
	require.Contains(t, err.Error(), "validate spec")
	require.Contains(t, err.Error(), `schema "Widget": invalid example`)
	require.Empty(t, diagnostics.String(), "strict refuses through the error, not a warning")
	require.Empty(t, stdout.String(), "a refused spec must not write to the JSON-RPC wire")
}

// A document that cannot be PARSED at all stays fatal under both policies: the
// policy only governs schema-level validation.
func TestLoadAndRegisterTools_UnparseableSpecStaysFatal(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "broken.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte("this is not yaml or openapi"), 0o644))

	config := &MCPServerConfig{SpecPath: specPath}
	_, err := loadAndRegisterTools(nil, config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "load spec")
}

// The same fixture driven through the real binary: the server starts (rc=0),
// warns on stderr, and answers the handshake on stdout. Strict mode is proven
// next to it with -strict-validation.
func TestBinary_InvalidExampleWarnsAndServes(t *testing.T) {
	binaryPath, err := buildMCPServer()
	require.NoError(t, err, "failed to build MCP server binary")
	defer func() { _ = os.Remove(binaryPath) }()

	specPath := writeInvalidExampleSpec(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-spec", specPath)
	cmd.Stdin = strings.NewReader(mcpHandshake)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "the server must start on a spec with one invalid example")

	require.Contains(t, stderr.String(), "Warning:", "the warning goes to stderr")
	require.Contains(t, stderr.String(), `components/schema "Widget"`)
	require.Contains(t, stderr.String(), "Widget.properties.version")
	require.NotContains(t, stdout.String(), "Warning:", "stdout is the JSON-RPC wire only")

	responses := parseJSONRPCLines(t, stdout.String())
	require.Len(t, responses, 2, "initialize and tools/list must both be answered; stderr: %s", stderr.String())
	require.Nil(t, responses[0].Error)
	require.Nil(t, responses[1].Error)
}

func TestBinary_InvalidExampleStrictRefuses(t *testing.T) {
	binaryPath, err := buildMCPServer()
	require.NoError(t, err, "failed to build MCP server binary")
	defer func() { _ = os.Remove(binaryPath) }()

	specPath := writeInvalidExampleSpec(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-strict-validation", "-spec", specPath)
	cmd.Stdin = strings.NewReader(mcpHandshake)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.Error(t, err, "-strict-validation must restore the refuse-to-start behaviour")
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.NotZero(t, exitErr.ExitCode())

	require.Contains(t, stderr.String(), "Fatal error:", "the refusal is reported on stderr")
	require.Contains(t, stderr.String(), `schema "Widget"`)
	require.Empty(t, stdout.String(), "a refused spec writes nothing to the JSON-RPC wire")
}

// MUSTER_STRICT_VALIDATION=1 is the documented non-flag form of the same
// opt-in; prove it on the binary as well.
func TestBinary_InvalidExampleStrictViaEnv(t *testing.T) {
	binaryPath, err := buildMCPServer()
	require.NoError(t, err, "failed to build MCP server binary")
	defer func() { _ = os.Remove(binaryPath) }()

	specPath := writeInvalidExampleSpec(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-spec", specPath)
	cmd.Env = append(os.Environ(), "MUSTER_STRICT_VALIDATION=1")
	cmd.Stdin = strings.NewReader(mcpHandshake)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.Error(t, err, "MUSTER_STRICT_VALIDATION=1 must restore the refuse-to-start behaviour")
	require.Contains(t, stderr.String(), "Fatal error:")
	require.Empty(t, stdout.String())
}
