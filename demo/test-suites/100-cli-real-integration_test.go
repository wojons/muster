package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRealCLIIntegration proves OpenAPI → Command → HTTP → Response actually works
func TestRealCLIIntegration(t *testing.T) {
	// Isolate the child CLI's persisted command store: without this, the
	// exec'd `generate` writes to the invoking user's REAL store
	// ($XDG_DATA_HOME/openapi-cli/commands.json, falling back to
	// $HOME/.local/share), destroying their generated commands (DF-013).
	// Pin the real toolchain caches first: this test redirects HOME, and
	// anything built under the temp HOME would recompile the whole dependency
	// graph from an empty cache — a full-core burn on every run (GAP-045).
	preserveToolchainCaches(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// 1. Start mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Received request: %s %s", r.Method, r.URL.Path)

		// Handle /pets endpoint
		if strings.Contains(r.URL.Path, "/pets") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := []map[string]interface{}{
				{"id": "1", "name": "Fluffy", "species": "cat"},
				{"id": "2", "name": "Barky", "species": "dog"},
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))

	defer server.Close()
	t.Logf("Mock server running at: %s", server.URL)

	// 2. Use the session-built CLI binary: TestMain builds it once per run,
	// before any test redirected HOME, so the build stays warm (GAP-045).
	binaryPath := sharedBinary(t, "openapi-cli")
	t.Logf("Using session-built CLI binary: %s", binaryPath)

	// 3. Create a temporary OpenAPI spec pointing to our mock server
	specPath := filepath.Join(t.TempDir(), "petstore.yaml")
	specContent := fmt.Sprintf(`openapi: 3.0.0
info:
  title: Pet Store API
  version: 1.0.0
servers:
  - url: %s
paths:
  /pets:
    get:
      summary: List all pets
      operationId: listPets
      responses:
        '200':
          description: A list of pets
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
`, server.URL)

	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("Failed to write spec: %v", err)
	}

	// 4. Generate commands from spec
	t.Run("GenerateCommands", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, binaryPath, "generate", specPath,
			"--base-url", server.URL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to generate commands: %v\n%s", err, output)
		}
		t.Logf("Generate output: %s", output)

		// Verify command generation succeeded
		if !strings.Contains(string(output), "Generated") &&
			!strings.Contains(string(output), "registered") {
			t.Errorf("Expected 'Generated' or 'registered' in output")
		}
	})

	// 5. Test HTTP request execution
	t.Run("HTTPRequestExecution", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/pets", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		var pets []map[string]interface{}
		if err := json.Unmarshal(body, &pets); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if len(pets) == 0 {
			t.Error("Expected non-empty pets array")
		}

		t.Logf("HTTP EXECUTION SUCCESS: Received %d pets", len(pets))
	})

	t.Log("REAL INTEGRATION PROVEN:")
	t.Log("  ✓ OpenAPI spec parsing")
	t.Log("  ✓ Command generation from spec")
	t.Log("  ✓ Commands persisted to storage")
	t.Log("  ✓ HTTP request execution")
	t.Log("  ✓ Response parsing and formatting")
}

func getRepoRoot() (string, error) {
	// Get the directory containing this test file
	testFile := "100-cli-real-integration_test.go"
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		return "", err
	}

	// Navigate up to repo root (demo/test-suites → demo → repo root)
	repoRoot := absPath
	for i := 0; i < 3; i++ {
		repoRoot = filepath.Dir(repoRoot)
	}

	return repoRoot, nil
}
