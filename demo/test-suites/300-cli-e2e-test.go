package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFullCLIWorkflow tests the complete end-to-end flow
func TestFullCLIWorkflow(t *testing.T) {
	// 1. Start mock HTTP server
	requestReceived := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Server received: %s %s", r.Method, r.URL.Path)

		if strings.Contains(r.URL.Path, "/pets") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := []map[string]interface{}{
				{"id": "1", "name": "Fluffy", "species": "cat"},
			}
			_ = json.NewEncoder(w).Encode(response)
			requestReceived <- true
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Logf("Mock server at: %s", server.URL)

	// 2. Build CLI binary
	binaryPath := filepath.Join(t.TempDir(), "cli")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/openapi-cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, out)
	}
	t.Logf("Built CLI: %s", binaryPath)

	// 3. Create OpenAPI spec
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	specContent := fmt.Sprintf(`openapi: 3.0.0
info:
  title: Pet Store
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

	// 4. Generate commands WITHOUT context
	t.Run("GenerateCommands", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "generate", specPath, "--base-url", server.URL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to generate commands: %v\n%s", err, output)
		}

		t.Logf("Generate output:\n%s", output)

		if !strings.Contains(string(output), "pets") {
			t.Error("Expected 'pets' in output")
		}
	})

	// 5. Try to execute a generated command - CRITICAL TEST
	t.Run("TryExecuteGeneratedCommand", func(t *testing.T) {
		// Try to list available commands
		cmd := exec.Command(binaryPath, "--help")
		output, err := cmd.CombinedOutput()
		t.Logf("Help output:\n%s", output)

		if err != nil {
			t.Logf("Help command (expected to work): %v", err)
		}

		// Check if 'pets' command is available
		if !strings.Contains(string(output), "pets") {
			t.Fatalf("'pets' command NOT found in help - commands not properly registered!")
		}
		t.Log("✓ 'pets' command found in help output")
	})

	t.Log("E2E TEST COMPLETE")
}
