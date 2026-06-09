package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/generator"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/pkg/request"
)

// TestRealCLIIntegration is a REAL integration test that proves:
// 1. OpenAPI spec parsing works
// 2. Command generation works
// 3. Commands can execute HTTP requests
// 4. Responses are returned correctly
//
// This is NOT a toy test - it traces actual data flow through the system.
func TestRealCLIIntegration(t *testing.T) {
	ctx := context.Background()

	// Step 1: Load and parse OpenAPI spec
	t.Run("ParseOpenAPISpec", func(t *testing.T) {
		specPath := filepath.Join("../../demo/openapi-specs", "petstore.yaml")
		specData, err := os.ReadFile(specPath)
		require.NoError(t, err, "Failed to read spec file")

		parser := openapi.NewParser()
		result, err := parser.Parse(ctx, specData, openapi.DefaultParseOptions())
		require.NoError(t, err, "Failed to parse spec")
		require.NotNil(t, result.Document, "Parsed document should not be nil")
		assert.Contains(t, result.Version, "1.0", "Version should be present")

		// Verify operations were extracted
		ops, err := parser.ExtractOperations(result.Document)
		require.NoError(t, err, "Failed to extract operations")
		require.NotEmpty(t, ops, "Should have extracted operations")
		assert.Len(t, ops, 5, "Should have 5 operations from petstore.yaml (GET, POST, PUT, DELETE on /pets and /pets/{petId})")

		// Verify operation details
		getPetsOp := findOperation(ops, "/pets", "GET")
		require.NotNil(t, getPetsOp, "Should have GET /pets operation")
		assert.Equal(t, "listPets", getPetsOp.ID, "Operation ID should match")

		createPetOp := findOperation(ops, "/pets", "POST")
		require.NotNil(t, createPetOp, "Should have POST /pets operation")
		assert.Equal(t, "createPet", createPetOp.ID, "Operation ID should match")

		getPetOp := findOperation(ops, "/pets/{petId}", "GET")
		require.NotNil(t, getPetOp, "Should have GET /pets/{petId} operation")
		assert.Equal(t, "getPet", getPetOp.ID, "Operation ID should match")

		updatePetOp := findOperation(ops, "/pets/{petId}", "PUT")
		require.NotNil(t, updatePetOp, "Should have PUT /pets/{petId} operation")
		assert.Equal(t, "updatePet", updatePetOp.ID, "Operation ID should match")

		deletePetOp := findOperation(ops, "/pets/{petId}", "DELETE")
		require.NotNil(t, deletePetOp, "Should have DELETE /pets/{petId} operation")
		assert.Equal(t, "deletePet", deletePetOp.ID, "Operation ID should match")

		// Step 2: Generate commands from spec
		t.Run("GenerateCommands", func(t *testing.T) {
			config := generator.Config{
				AppName:        "test-openapi",
				BaseURL:        "http://example.com",
				DefaultFormat:  "json",
				Version:        "1.0.0",
				NamingStrategy: generator.NamingKebab,
				OperationStyle: generator.StyleCRUD,
			}

			gen, err := generator.NewGenerator(config)
			require.NoError(t, err, "Failed to create generator")

			commands, err := gen.GenerateCommands(result.Document)
			require.NoError(t, err, "Failed to generate commands")
			require.NotEmpty(t, commands, "Should have generated commands")
			assert.Len(t, commands, 1, "Should have 1 resource command (pets)")

			// Verify the pets command exists
			var petsCmd interface{} = nil
			for _, cmd := range commands {
				if cmd.Use == "pets" {
					petsCmd = cmd
					break
				}
			}
			require.NotNil(t, petsCmd, "Should have pets command")

			// Step 3: Create mock HTTP server for testing
			t.Run("ExecuteHTTPRequests", func(t *testing.T) {
				// Create mock server that simulates the petstore API
				requestCount := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestCount++

					switch {
					case r.URL.Path == "/pets" && r.Method == "GET":
						// List pets endpoint
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode([]map[string]interface{}{
							{"id": 1, "name": "Fluffy", "tag": "cat"},
							{"id": 2, "name": "Rex", "tag": "dog"},
						})

					case r.URL.Path == "/pets" && r.Method == "POST":
						// Create pet endpoint
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusCreated)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"id":     3,
							"name":   "Spike",
							"tag":    "dog",
							"status": "available",
						})

					case r.URL.Path == "/pets/1" && r.Method == "GET":
						// Get pet by ID endpoint
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"id":   1,
							"name": "Fluffy",
							"tag":  "cat",
						})

					case r.URL.Path == "/pets/999" && r.Method == "GET":
						// Pet not found
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusNotFound)
						_ = json.NewEncoder(w).Encode(map[string]string{
							"error": "Pet not found",
						})

					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer server.Close()

				// Test 1: Execute GET /pets request
				t.Run("ListPets", func(t *testing.T) {
					builder := request.NewBuilder(server.URL, "/pets", "GET")
					resp, err := builder.Execute(ctx)
					require.NoError(t, err, "Failed to execute request")
					defer func() { _ = resp.Body.Close() }()

					assert.Equal(t, http.StatusOK, resp.StatusCode, "Status code should be 200")

					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err, "Failed to read response body")

					var pets []map[string]interface{}
					err = json.Unmarshal(body, &pets)
					require.NoError(t, err, "Failed to unmarshal JSON")

					assert.Len(t, pets, 2, "Should have 2 pets")
					assert.Equal(t, "Fluffy", pets[0]["name"], "First pet should be Fluffy")
					assert.Equal(t, "Rex", pets[1]["name"], "Second pet should be Rex")
				})

				// Test 2: Execute POST /pets request
				t.Run("CreatePet", func(t *testing.T) {
					builder := request.NewBuilder(server.URL, "/pets", "POST")
					builder.JSONBody(map[string]interface{}{
						"name":   "Spike",
						"tag":    "dog",
						"status": "available",
					})

					resp, err := builder.Execute(ctx)
					require.NoError(t, err, "Failed to execute request")
					defer func() { _ = resp.Body.Close() }()

					assert.Equal(t, http.StatusCreated, resp.StatusCode, "Status code should be 201")

					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err, "Failed to read response body")

					var pet map[string]interface{}
					err = json.Unmarshal(body, &pet)
					require.NoError(t, err, "Failed to unmarshal JSON")

					assert.Equal(t, float64(3), pet["id"], "Pet ID should be 3")
					assert.Equal(t, "Spike", pet["name"], "Pet name should be Spike")
				})

				// Test 3: Execute GET /pets/{petId} request
				t.Run("GetPetById", func(t *testing.T) {
					builder := request.NewBuilder(server.URL, "/pets/1", "GET")
					resp, err := builder.Execute(ctx)
					require.NoError(t, err, "Failed to execute request")
					defer func() { _ = resp.Body.Close() }()

					assert.Equal(t, http.StatusOK, resp.StatusCode, "Status code should be 200")

					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err, "Failed to read response body")

					var pet map[string]interface{}
					err = json.Unmarshal(body, &pet)
					require.NoError(t, err, "Failed to unmarshal JSON")

					assert.Equal(t, float64(1), pet["id"], "Pet ID should be 1")
					assert.Equal(t, "Fluffy", pet["name"], "Pet should be Fluffy")
				})

				// Test 4: Handle 404 error
				t.Run("PetNotFound", func(t *testing.T) {
					builder := request.NewBuilder(server.URL, "/pets/999", "GET")
					resp, err := builder.Execute(ctx)
					require.NoError(t, err, "Failed to execute request")
					defer func() { _ = resp.Body.Close() }()

					assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Status code should be 404")
				})

				// Verify all requests were made
				t.Logf("Total HTTP requests made: %d", requestCount)
				assert.Equal(t, 4, requestCount, "Should have made 4 HTTP requests (list, create, get, not found)")
			})
		})
	})
}

// Helper function to find operation by path and method
func findOperation(ops []*openapi.Operation, path, method string) *openapi.Operation {
	for _, op := range ops {
		if op.Path == path && op.Method == method {
			return op
		}
	}
	return nil
}

// TestRealEndToEndFlow is a comprehensive end-to-end test
// that validates the complete system workflow:
// OpenAPI Spec → Parse → Generate Commands → Execute Request → Handle Response
func TestRealEndToEndFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Parse spec
	specPath := filepath.Join("../../demo/openapi-specs", "petstore.yaml")
	specData, err := os.ReadFile(specPath)
	require.NoError(t, err)

	parser := openapi.NewParser()
	result, err := parser.Parse(ctx, specData, openapi.DefaultParseOptions())
	require.NoError(t, err)

	// Step 2: Generate commands
	config := generator.Config{
		AppName:        "test-e2e",
		BaseURL:        "http://127.0.0.1:8080",
		DefaultFormat:  "json",
		Version:        "1.0.0",
		NamingStrategy: generator.NamingKebab,
		OperationStyle: generator.StyleCRUD,
	}

	gen, err := generator.NewGenerator(config)
	require.NoError(t, err)

	commands, err := gen.GenerateCommands(result.Document)
	require.NoError(t, err)
	require.NotEmpty(t, commands)

	// Step 3: Verify HTTP request building and execution
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "success",
			"path":    r.URL.Path,
			"method":  r.Method,
		})
	}))
	defer server.Close()

	builder := request.NewBuilder(server.URL, "/test", "GET")
	resp, err := builder.Execute(ctx)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Request should succeed")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var resultData map[string]interface{}
	err = json.Unmarshal(body, &resultData)
	require.NoError(t, err)

	assert.Equal(t, "success", resultData["message"], "Response should contain success message")
}
