package builtin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/openapi"
)

func TestNewDiscoverCommand(t *testing.T) {
	t.Run("creates discover command", func(t *testing.T) {
		cmd := newDiscoverCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "discover", cmd.Name())
		assert.Equal(t, "Discover API operations from a remote OpenAPI spec", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newDiscoverCommand()

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s), received 0")
	})

	t.Run("requires valid http url", func(t *testing.T) {
		cmd := newDiscoverCommand()

		// bare string without http:// prefix should fail
		err := cmd.RunE(cmd, []string{"not-a-url"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "must start with http:// or https://")
	})

	t.Run("handles unreachable URL", func(t *testing.T) {
		cmd := newDiscoverCommand()

		err := cmd.RunE(cmd, []string{"http://localhost:19999/nonexistent"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetch spec")
	})
}

func TestDiscoverCommand_WithServer(t *testing.T) {
	t.Run("discovers operations from valid spec", func(t *testing.T) {
		// Create a test HTTP server serving a valid OpenAPI spec
		specContent := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Test API",
    "version": "1.0.0"
  },
  "paths": {
    "/health": {
      "get": {
        "operationId": "getHealth",
        "summary": "Health check",
        "tags": ["system"],
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    },
    "/pets": {
      "get": {
        "operationId": "listPets",
        "summary": "List all pets",
        "tags": ["pets"],
        "parameters": [
          {
            "name": "limit",
            "in": "query",
            "required": false,
            "schema": {
              "type": "integer"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "A list of pets"
          }
        }
      },
      "post": {
        "operationId": "createPet",
        "summary": "Create a pet",
        "tags": ["pets"],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["name"],
                "properties": {
                  "name": {
                    "type": "string",
                    "description": "Name of the pet"
                  },
                  "tag": {
                    "type": "string",
                    "description": "Tag for the pet"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Created"
          }
        }
      }
    },
    "/pets/{petId}": {
      "get": {
        "operationId": "getPetById",
        "summary": "Get a pet by ID",
        "tags": ["pets"],
        "parameters": [
          {
            "name": "petId",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "A pet"
          }
        }
      },
      "delete": {
        "operationId": "deletePet",
        "summary": "Delete a pet",
        "tags": ["pets"],
        "parameters": [
          {
            "name": "petId",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "204": {
            "description": "Deleted"
          }
        }
      }
    }
  }
}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, specContent)
		}))
		defer server.Close()

		cmd := newDiscoverCommand()
		err := cmd.RunE(cmd, []string{server.URL})

		require.NoError(t, err)
	})

	t.Run("output format matches specification", func(t *testing.T) {
		specContent := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Task Forge",
    "version": "1.0.0"
  },
  "paths": {
    "/projects": {
      "get": {
        "operationId": "listProjects",
        "summary": "List projects",
        "tags": ["Projects"],
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "operationId": "createProject",
        "summary": "Create project",
        "tags": ["Projects"],
        "responses": {"201": {"description": "Created"}}
      }
    },
    "/tasks": {
      "get": {
        "operationId": "listTasks",
        "summary": "List tasks",
        "tags": ["Tasks"],
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "operationId": "createTask",
        "summary": "Create task",
        "tags": ["Tasks"],
        "responses": {"201": {"description": "Created"}}
      }
    }
  }
}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, specContent)
		}))
		defer server.Close()

		cmd := newDiscoverCommand()
		err := cmd.RunE(cmd, []string{server.URL})

		require.NoError(t, err)
	})

	t.Run("handles non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "not found")
		}))
		defer server.Close()

		cmd := newDiscoverCommand()
		err := cmd.RunE(cmd, []string{server.URL})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status")
	})

	t.Run("handles invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "this is not valid json")
		}))
		defer server.Close()

		cmd := newDiscoverCommand()
		err := cmd.RunE(cmd, []string{server.URL})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse spec")
	})

	t.Run("handles empty paths spec", func(t *testing.T) {
		specContent := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Empty API",
    "version": "1.0.0"
  },
  "paths": {}
}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, specContent)
		}))
		defer server.Close()

		cmd := newDiscoverCommand()
		err := cmd.RunE(cmd, []string{server.URL})

		require.NoError(t, err)
	})
}

func TestSortOperations(t *testing.T) {
	ops := []*openapi.Operation{
		{Method: "POST", Path: "/pets", Tags: []string{"pets"}, Summary: "Create pet", ID: "createPet"},
		{Method: "GET", Path: "/pets", Tags: []string{"pets"}, Summary: "List pets", ID: "listPets"},
		{Method: "GET", Path: "/health", Tags: []string{"system"}, Summary: "Health", ID: "getHealth"},
		{Method: "DELETE", Path: "/pets/{id}", Tags: []string{"pets"}, Summary: "Delete pet", ID: "deletePet"},
		{Method: "GET", Path: "/pets/{id}", Tags: []string{"pets"}, Summary: "Get pet", ID: "getPetById"},
	}

	sortOperations(ops)

	// After sort: by tag (alphabetical), then path, then method order
	// pets < system alphabetically, so pets group first
	// Within pets: /pets then /pets/{id} (alphabetical path)
	// Within /pets: GET(0) then POST(1) (method order)
	// Within /pets/{id}: GET(0) then DELETE(4) (method order)

	// Index 0: pets, /pets, GET
	assert.Equal(t, "pets", firstTag(ops[0].Tags))
	assert.Equal(t, "GET", ops[0].Method)
	assert.Equal(t, "/pets", ops[0].Path)

	// Index 1: pets, /pets, POST
	assert.Equal(t, "pets", firstTag(ops[1].Tags))
	assert.Equal(t, "POST", ops[1].Method)
	assert.Equal(t, "/pets", ops[1].Path)

	// Index 2: pets, /pets/{id}, GET
	assert.Equal(t, "pets", firstTag(ops[2].Tags))
	assert.Equal(t, "GET", ops[2].Method)
	assert.Equal(t, "/pets/{id}", ops[2].Path)

	// Index 3: pets, /pets/{id}, DELETE
	assert.Equal(t, "pets", firstTag(ops[3].Tags))
	assert.Equal(t, "DELETE", ops[3].Method)
	assert.Equal(t, "/pets/{id}", ops[3].Path)

	// Index 4: system, /health, GET
	assert.Equal(t, "system", firstTag(ops[4].Tags))
	assert.Equal(t, "GET", ops[4].Method)
	assert.Equal(t, "/health", ops[4].Path)
}

func TestFirstTag(t *testing.T) {
	assert.Equal(t, "pets", firstTag([]string{"pets", "system"}))
	assert.Equal(t, "", firstTag([]string{}))
	assert.Equal(t, "", firstTag(nil))
}

func TestMethodOrder(t *testing.T) {
	// Standard methods should sort in expected order
	assert.True(t, methodOrder("GET") < methodOrder("POST"))
	assert.True(t, methodOrder("POST") < methodOrder("PUT"))
	assert.True(t, methodOrder("PUT") < methodOrder("PATCH"))
	assert.True(t, methodOrder("PATCH") < methodOrder("DELETE"))
	assert.True(t, methodOrder("DELETE") < methodOrder("HEAD"))
	assert.True(t, methodOrder("HEAD") < methodOrder("OPTIONS"))
}

func TestGetBaseURL(t *testing.T) {
	assert.Equal(t, "http://localhost:9800", getBaseURL("http://localhost:9800/openapi.json"))
	assert.Equal(t, "https://api.example.com", getBaseURL("https://api.example.com/openapi.json"))
	assert.Equal(t, "https://petstore3.swagger.io", getBaseURL("https://petstore3.swagger.io/api/v3/openapi.json"))
	assert.Equal(t, "http://localhost:9800", getBaseURL("http://localhost:9800"))
	assert.Equal(t, "http://host:80", getBaseURL("http://host:80/path/to/spec"))
}

func TestDiscoverCommand_HelpFlag(t *testing.T) {
	t.Run("has discover subcommand in root", func(t *testing.T) {
		cmd := newDiscoverCommand()
		assert.Contains(t, cmd.Use, "discover")
	})

	t.Run("supports --help", func(t *testing.T) {
		cmd := newDiscoverCommand()
		assert.NotNil(t, cmd)

		assert.NotEmpty(t, cmd.Long)
		assert.NotEmpty(t, cmd.Short)
	})
}

// TestDiscoverCommand_WithLargeSpec verifies that a spec with many operations
// is handled correctly.
func TestDiscoverCommand_WithManyOperations(t *testing.T) {
	specContent := `{"openapi":"3.0.0","info":{"title":"Large API","version":"1.0.0"},"paths":{`
	pathEntries := []string{}
	for i := 0; i < 50; i++ {
		pathEntries = append(pathEntries, fmt.Sprintf(`"/resource%d":{"get":{"operationId":"getResource%d","summary":"Get resource %d","tags":["resources"],"responses":{"200":{"description":"OK"}}}}`, i, i, i))
	}
	specContent += strings.Join(pathEntries, ",")
	specContent += `}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, specContent)
	}))
	defer server.Close()

	cmd := newDiscoverCommand()
	err := cmd.RunE(cmd, []string{server.URL})

	require.NoError(t, err)
}
