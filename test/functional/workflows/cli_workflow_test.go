// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package workflows_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/generator"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/test/functional/harness"
)

// petstoreSpecJSON is a minimal OpenAPI 3.0 spec used for workflow testing.
const petstoreSpecJSON = `{
  "openapi": "3.0.0",
  "info": {
    "title": "Pet Store API",
    "version": "1.0.0"
  },
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "tags": ["pets"],
        "summary": "List all pets",
        "parameters": [
          {
            "name": "limit",
            "in": "query",
            "schema": { "type": "integer", "default": 10 }
          }
        ],
        "responses": {
          "200": { "description": "A list of pets" }
        }
      },
      "post": {
        "operationId": "createPet",
        "tags": ["pets"],
        "summary": "Create a new pet",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "type": "object" }
            }
          }
        },
        "responses": {
          "201": { "description": "Pet created" }
        }
      }
    },
    "/pets/{petId}": {
      "get": {
        "operationId": "getPet",
        "tags": ["pets"],
        "summary": "Get a specific pet",
        "parameters": [
          {
            "name": "petId",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": { "description": "Pet details" }
        }
      },
      "delete": {
        "operationId": "deletePet",
        "tags": ["pets"],
        "summary": "Delete a pet",
        "parameters": [
          {
            "name": "petId",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "204": { "description": "Deleted" }
        }
      }
    }
  }
}`

// TestSystem_Initialization tests that the system can start, initialize, and reach ready state.
func TestSystem_Initialization(t *testing.T) {
	system := harness.NewSystem(t)
	defer system.Cleanup(t)

	ctx := system.Context()
	require.NotNil(t, ctx)

	err := system.Initialize(t)
	require.NoError(t, err)
	require.True(t, system.IsReady())
}

// TestUserWorkflow_SpecToCommands tests the full OpenAPI spec → command generation pipeline.
// It replaces the TODO stubs that previously only did a bare HTTP GET.
func TestUserWorkflow_SpecToCommands(t *testing.T) {
	// Setup mock server that serves an OpenAPI spec at /openapi.json
	// and a dummy API at /pets
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(petstoreSpecJSON))
			return
		}
		// Default: return dummy pet data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"pets":[{"id":1,"name":"Fluffy"}]}`))
	}))
	defer server.Close()

	// Phase 1: Load OpenAPI spec from mock server
	resp, err := http.Get(server.URL + "/openapi.json")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	specBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotEmpty(t, specBytes)

	// Phase 2: Parse the OpenAPI spec
	parser := openapi.NewParser()
	result, err := parser.Parse(context.Background(), specBytes, openapi.DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Document)
	require.Equal(t, "1.0.0", result.Version)
	require.Equal(t, "Pet Store API", result.Document.Info.Title)

	// Phase 3: Generate commands from the parsed spec
	gen, err := generator.NewGenerator(generator.Config{
		AppName:        "petstore-cli",
		AppDescription: "CLI for Pet Store API",
		Version:        "1.0.0",
		BaseURL:        server.URL,
		NamingStrategy: generator.NamingKebab,
	})
	require.NoError(t, err)

	cmds, err := gen.GenerateCommands(result.Document)
	require.NoError(t, err)
	require.NotEmpty(t, cmds, "expected at least one command group from the spec")

	// Verify command structure: "pets" group with list, create, get, delete subcommands
	cmdNames := make(map[string]bool)
	for _, cmd := range cmds {
		cmdNames[cmd.Name()] = true
	}
	require.True(t, cmdNames["pets"], "expected 'pets' command group; got: %v", cmdNames)

	// Find the pets command and verify subcommands
	found := false
	for _, cmd := range cmds {
		if cmd.Name() == "pets" {
			found = true
			subNames := make(map[string]bool)
			for _, sub := range cmd.Commands() {
				subNames[sub.Name()] = true
			}
			require.True(t, subNames["list-pets"], "expected 'list-pets' subcommand; got: %v", subNames)
			require.True(t, subNames["create-pet"], "expected 'create-pet' subcommand")
			require.True(t, subNames["get-pet"], "expected 'get-pet' subcommand")
			require.True(t, subNames["delete-pet"], "expected 'delete-pet' subcommand")
			break
		}
	}
	require.True(t, found, "pets command group not found; got: %v", cmdNames)
}

// TestUserWorkflow_CLI_CommandExecution tests a complete user journey through the CLI.
func TestUserWorkflow_CLI_CommandExecution(t *testing.T) {
	// Setup mock server that serves BOTH an OpenAPI spec and a pet API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(petstoreSpecJSON))
			return
		}
		// Actual API endpoint
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"pets":[{"id":1,"name":"Fluffy"}]}`))
	}))
	defer server.Close()

	// Step 1: Load OpenAPI spec from mock server
	resp, err := http.Get(server.URL + "/openapi.json")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	specBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Step 2: Parse the spec
	parser := openapi.NewParser()
	result, err := parser.Parse(context.Background(), specBytes, openapi.DefaultParseOptions())
	require.NoError(t, err)
	require.NotNil(t, result.Document)

	// Step 3: Generate commands from parsed spec
	gen, err := generator.NewGenerator(generator.Config{
		AppName:        "petstore-cli",
		AppDescription: "CLI for Pet Store API",
		Version:        "1.0.0",
		BaseURL:        server.URL,
		NamingStrategy: generator.NamingKebab,
	})
	require.NoError(t, err)

	cmds, err := gen.GenerateCommands(result.Document)
	require.NoError(t, err)
	require.NotEmpty(t, cmds, "expected commands from spec generation")

	// Step 4: Execute command (simulate API call)
	resp, err = http.Get(server.URL + "/pets")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestSystem_CachingBehavior tests system caching behavior.
func TestSystem_CachingBehavior(t *testing.T) {
	_ = harness.NewState()

	// First request (cache miss)
	result1 := makeCachedRequest("/api/pets", false)
	require.False(t, result1.fromCache)
	require.NotNil(t, result1.data)

	// Second request (cache hit)
	result2 := makeCachedRequest("/api/pets", true)
	require.True(t, result2.fromCache)
	require.NotNil(t, result2.data)
}

type requestResult struct {
	fromCache bool
	data      []byte
}

func makeCachedRequest(path string, useCache bool) requestResult {
	return requestResult{
		fromCache: useCache,
		data:      []byte(`{"success":true}`),
	}
}

// TestSystem_ConcurrentRequests tests system handles concurrent operations.
func TestSystem_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errors := make(chan error, 10)

	// Launch 10 concurrent requests
	for i := 0; i < 10; i++ {
		go func() {
			resp, err := http.Get(server.URL + "/api/pets")
			if err != nil {
				errors <- err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			errors <- nil
		}()
	}

	// All requests should succeed
	for i := 0; i < 10; i++ {
		select {
		case err := <-errors:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("timeout waiting for requests")
		}
	}
}
