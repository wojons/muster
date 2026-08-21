# E2E Test Playbook

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## End-to-End Test Scenarios

```go
//go:build e2e

package e2e

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

var (
    binaryPath string
    apiURL     = "http://localhost:8080"
)

func TestMain(m *testing.M) {
    // Build binary before tests
    var err error
    binaryPath, err = buildBinary()
    if err != nil {
        panic(err)
    }
    defer os.Remove(binaryPath)
    
    os.Exit(m.Run())
}

func buildBinary() (string, error) {
    tmpDir := os.TempDir()
    binPath := filepath.Join(tmpDir, "muster-e2e")
    
    cmd := exec.Command("go", "build", "-o", binPath, "./cmd/openapi-cli")
    cmd.Dir = "../.." // Adjust to project root
    
    if err := cmd.Run(); err != nil {
        return "", err
    }
    
    return binPath, nil
}
```

---

## CLI Testing with exec

```go
func TestCLI_VersionCommand(t *testing.T) {
    cmd := exec.Command(binaryPath, "version")
    output, err := cmd.CombinedOutput()
    
    require.NoError(t, err)
    assert.Contains(t, string(output), "muster version")
}

func TestCLI_ConfigCommand(t *testing.T) {
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")
    
    // Write test config
    err := os.WriteFile(configPath, []byte("log_level: debug"), 0644)
    require.NoError(t, err)
    
    cmd := exec.Command(binaryPath, "config", "show", "--config", configPath)
    output, err := cmd.CombinedOutput()
    
    require.NoError(t, err)
    assert.Contains(t, string(output), "debug")
}

func TestCLI_RequestCommand(t *testing.T) {
    // Start mock server
    server := httptest.NewServer(http.HandlerFunc(
        func(w http.ResponseWriter, r *http.Request) {
            json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
        }),
    )
    defer server.Close()
    
    cmd := exec.Command(binaryPath, "request", "GET", server.URL+"/api/test")
    output, err := cmd.CombinedOutput()
    
    require.NoError(t, err)
    assert.Contains(t, string(output), "ok")
}
```

---

## MCP Testing

```go
func TestMCP_Initialize(t *testing.T) {
    // Start MCP server
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, binaryPath+"-mcp")
    stdin, err := cmd.StdinPipe()
    require.NoError(t, err)
    
    stdout, err := cmd.StdoutPipe()
    require.NoError(t, err)
    
    require.NoError(t, cmd.Start())
    defer cmd.Process.Kill()
    
    // Send initialize request
    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      1,
        "method":  "initialize",
        "params": map[string]interface{}{
            "protocolVersion": "2024-11-05",
        },
    }
    
    reqJSON, _ := json.Marshal(req)
    stdin.Write(append(reqJSON, '\n'))
    
    // Read response
    reader := bufio.NewReader(stdout)
    resp, err := reader.ReadString('\n')
    require.NoError(t, err)
    
    var result map[string]interface{}
    json.Unmarshal([]byte(resp), &result)
    
    assert.Equal(t, float64(1), result["id"])
    assert.NotNil(t, result["result"])
}
```

---

## Golden File Testing

```go
func TestCLI_Output_Golden(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        golden   string
        update   bool
    }{
        {
            name:   "version output",
            args:   []string{"version"},
            golden: "testdata/golden/version.txt",
        },
        {
            name:   "help output",
            args:   []string{"--help"},
            golden: "testdata/golden/help.txt",
        },
    }
    
    flagUpdate := flag.Bool("update", false, "Update golden files")
    flag.Parse()
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := exec.Command(binaryPath, tt.args...)
            output, err := cmd.CombinedOutput()
            require.NoError(t, err)
            
            if *flagUpdate {
                os.WriteFile(tt.golden, output, 0644)
                return
            }
            
            expected, err := os.ReadFile(tt.golden)
            require.NoError(t, err)
            
            assert.Equal(t, string(expected), string(output))
        })
    }
}
```

---

## Full API Flow Testing

```go
func TestE2E_FullAPIFlow(t *testing.T) {
    // Setup: Create temp directory and config
    tmpDir := t.TempDir()
    configFile := filepath.Join(tmpDir, "muster.yaml")
    
    config := `
api:
  base_url: https://api.example.com
  timeout: 30s
auth:
  type: bearer
  token: test-token
`
    require.NoError(t, os.WriteFile(configFile, []byte(config), 0600))
    
    // Test 1: Configure profile
    cmd := exec.Command(binaryPath, "config", "set", "profile.default", "--config", configFile)
    output, err := cmd.CombinedOutput()
    require.NoError(t, err, string(output))
    
    // Test 2: Make authenticated request
    cmd = exec.Command(binaryPath, "request", "GET", "/users", "--config", configFile)
    output, err = cmd.CombinedOutput()
    require.NoError(t, err, string(output))
    
    // Test 3: Cache response
    cmd = exec.Command(binaryPath, "cache", "list", "--config", configFile)
    output, err = cmd.CombinedOutput()
    require.NoError(t, err, string(output))
    assert.Contains(t, string(output), "/users")
}
```

---

## Running E2E Tests

```bash
# Run all E2E tests
go test -v -tags=e2e ./cmd/e2e/...

# Run specific test
go test -v -tags=e2e -run TestCLI_Version ./cmd/e2e/...

# Update golden files
go test -v -tags=e2e -update ./cmd/e2e/...

# With custom binary
e2e_BINARY=/path/to/muster go test -v -tags=e2e ./cmd/e2e/...
```
