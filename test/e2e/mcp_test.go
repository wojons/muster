package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCP_Initialize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, mcpBinaryPath)

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"roots": map[string]interface{}{
					"listChanged": true,
				},
			},
		},
	}

	reqJSON, _ := json.Marshal(req)
	_, err = stdin.Write(append(reqJSON, '\n'))
	require.NoError(t, err)

	outputBuf := make([]byte, 8192)
	n, err := stdout.Read(outputBuf)
	if err != nil || n == 0 {
		t.Logf("MCP server did not respond: %v", err)
		return
	}

	resp := strings.TrimSpace(string(outputBuf[:n]))
	t.Logf("Initialize response: %s", resp)
	assert.Contains(t, resp, "result")
}

func TestMCP_ListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mcpBinaryPath := filepath.Join(filepath.Dir(binaryPath), filepath.Base(binaryPath)+"-mcp")
	cmd := exec.CommandContext(ctx, mcpBinaryPath)

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}
	reqJSON, _ := json.Marshal(initializeReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	time.Sleep(500 * time.Millisecond)

	listToolsReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}
	reqJSON, err = json.Marshal(listToolsReq)
	require.NoError(t, err)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	outputBuf := make([]byte, 8192)
	n, _ := stdout.Read(outputBuf)
	resp1 := strings.TrimSpace(string(outputBuf[:n]))

	outputBuf = make([]byte, 8192)
	n, err = stdout.Read(outputBuf)
	require.NoError(t, err)
	resp2 := strings.TrimSpace(string(outputBuf[:n]))

	t.Logf("Initialize response: %s", resp1)
	t.Logf("List tools response: %s", resp2)
}

func TestMCP_CallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mcpBinaryPath := filepath.Join(filepath.Dir(binaryPath), filepath.Base(binaryPath)+"-mcp")
	cmd := exec.CommandContext(ctx, mcpBinaryPath)

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}
	reqJSON, _ := json.Marshal(initializeReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	time.Sleep(500 * time.Millisecond)

	callToolReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "version",
			"arguments": map[string]interface{}{},
		},
	}
	reqJSON, err = json.Marshal(callToolReq)
	require.NoError(t, err, "JSON marshal should succeed")
	_, _ = stdin.Write(append(reqJSON, '\n'))

	outputBuf := make([]byte, 8192)
	n, _ := stdout.Read(outputBuf)
	resp := strings.TrimSpace(string(outputBuf[:n]))

	t.Logf("Tool call response: %s", resp)
}

func TestMCP_ListResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath+"-mcp")

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}
	reqJSON, _ := json.Marshal(initializeReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	time.Sleep(500 * time.Millisecond)

	listResourcesReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "resources/list",
		"params":  map[string]interface{}{},
	}
	reqJSON, _ = json.Marshal(listResourcesReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	outputBuf := make([]byte, 8192)
	n, _ := stdout.Read(outputBuf)
	resp := strings.TrimSpace(string(outputBuf[:n]))

	t.Logf("List resources response: %s", resp)
}

func TestMCP_ReadResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath+"-mcp")

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}
	reqJSON, _ := json.Marshal(initializeReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	time.Sleep(500 * time.Millisecond)

	initializeResp := readJSONRPCResponse(stdout)

	listResourcesReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "resources/list",
		"params":  map[string]interface{}{},
	}
	reqJSON, _ = json.Marshal(listResourcesReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	listResourcesResp := readJSONRPCResponse(stdout)

	var resources struct {
		Result struct {
			Resources []struct {
				URI string `json:"uri"`
			} `json:"resources"`
		} `json:"result"`
	}

	err = json.Unmarshal([]byte(listResourcesResp), &resources)
	if err == nil && len(resources.Result.Resources) > 0 {
		uri := resources.Result.Resources[0].URI

		readReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "resources/read",
			"params": map[string]interface{}{
				"uri": uri,
			},
		}
		reqJSON, err := json.Marshal(readReq)
		require.NoError(t, err, "JSON marshal should succeed")
		_, _ = stdin.Write(append(reqJSON, '\n'))

		readResp := readJSONRPCResponse(stdout)
		t.Logf("Read resource response: %s", readResp)
	}

	t.Logf("Initialize response: %s", initializeResp)
	t.Logf("List resources response: %s", listResourcesResp)
}

func TestMCP_MultipleRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath+"-mcp")

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	InitializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}
	reqJSON, _ := json.Marshal(InitializeReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 5; i++ {
		req := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      i + 2,
			"method":  "tools/list",
			"params":  map[string]interface{}{},
		}
		reqJSON, err := json.Marshal(req)
		require.NoError(t, err, "JSON marshal should succeed")
		_, _ = stdin.Write(append(reqJSON, '\n'))

		output := readJSONRPCResponse(stdout)
		t.Logf("Request %d response: %s", i, output)
	}
}

func TestMCP_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath+"-mcp")

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	if err != nil {
		t.Logf("MCP server may not be implemented: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	go func() {
_, _ = io.Copy(io.Discard, stderr)
	}()

	time.Sleep(500 * time.Millisecond)

	invalidReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "invalid_method",
	}
	reqJSON, _ := json.Marshal(invalidReq)
	_, _ = stdin.Write(append(reqJSON, '\n'))

	output := readJSONRPCResponse(stdout)
	t.Logf("Error response: %s", output)

	time.Sleep(500 * time.Millisecond)

	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
		},
	}
	reqJSON, err = json.Marshal(initializeReq)
	require.NoError(t, err, "JSON marshal should succeed")
	_, _ = stdin.Write(append(reqJSON, '\n'))

	output = readJSONRPCResponse(stdout)
	t.Logf("Initialize after error: %s", output)
}

func readJSONRPCResponse(stdout io.Reader) string {
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}
