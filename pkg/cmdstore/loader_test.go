package cmdstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewLoader ---

func TestNewLoader(t *testing.T) {
	store := NewCommandStore()
	err := store.Save(&StoredData{
		Commands: []CommandMetadata{
			{Name: "test", Use: "test", Short: "test"},
		},
	})
	require.NoError(t, err)

	loader := NewLoader(store, "http://example.com")
	require.NotNil(t, loader)

	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	assert.Equal(t, "http://example.com", baseURL)
}

// --- LoadCommands ---

func TestLoadCommands_EmptyStore(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	cmds, err := loader.LoadCommands()
	require.NoError(t, err)
	assert.Empty(t, cmds)
}

func TestLoadCommands_WithCommands(t *testing.T) {
	store := NewCommandStore()
	store.commands = []CommandMetadata{
		{Name: "cmd1", Use: "cmd1", Short: "first command"},
		{Name: "cmd2", Use: "cmd2", Short: "second command"},
	}
	store.loaded = true

	loader := NewLoader(store, "http://example.com")
	cmds, err := loader.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 2)
	assert.Equal(t, "cmd1", cmds[0].Name())
	assert.Equal(t, "first command", cmds[0].Short)
	assert.Equal(t, "cmd2", cmds[1].Name())
	assert.Equal(t, "second command", cmds[1].Short)
}

func TestLoadCommands_StoreError(t *testing.T) {
	dir := t.TempDir()
	path := fmt.Sprintf("%s/commands.json", dir)
	store := NewCommandStore(path)
	// Write corrupt JSON so loadFromDisk fails
	os.WriteFile(path, []byte("{bad json}"), 0644)

	loader := NewLoader(store, "http://example.com")
	_, err := loader.LoadCommands()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load metadata")
}

// --- GetBaseURL ---

func TestGetBaseURL_MetaBaseURLSet(t *testing.T) {
	store := NewCommandStore()
	store.meta = &CommandMetadata{BaseURL: "https://from-meta.com"}
	store.loaded = true

	loader := NewLoader(store, "https://default.com")
	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	assert.Equal(t, "https://from-meta.com", baseURL)
}

func TestGetBaseURL_FallbackToLoaderBaseURL(t *testing.T) {
	store := NewCommandStore()
	store.meta = &CommandMetadata{Name: "test"}
	store.loaded = true

	loader := NewLoader(store, "https://fallback.com")
	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	assert.Equal(t, "https://fallback.com", baseURL)
}

func TestGetBaseURL_EmptyStoreError(t *testing.T) {
	store := NewCommandStore()

	loader := NewLoader(store, "http://example.com")
	_, err := loader.GetBaseURL()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no stored data")
}

func TestGetBaseURL_NilMeta(t *testing.T) {
	store := NewCommandStore()
	store.commands = []CommandMetadata{{Name: "test-no-baseurl"}}
	store.loaded = true
	// store.meta is nil, Load() will read from commands[0] but its BaseURL is empty

	loader := NewLoader(store, "https://default-base.com")
	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	// Since meta.BaseURL is empty, falls back to l.baseURL
	assert.Equal(t, "https://default-base.com", baseURL)
}

// --- SetHeaders ---

func TestSetHeaders(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	headers := []string{"X-Api-Key: abc123", "Accept: application/json"}
	loader.SetHeaders(headers)
	assert.Equal(t, headers, loader.headers)
}

// --- reconstructCommand ---

func TestReconstructCommand_AllFlagTypes(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	meta := CommandMetadata{
		Name:    "test-cmd",
		Use:     "test-cmd [flags]",
		Short:   "A test command",
		Long:    "A longer description",
		Example: "test-cmd --name foo --count 5",
		BaseURL: "http://override.example.com",
		Flags: []FlagMetadata{
			{
				Name:        "name",
				Shorthand:   "n",
				Type:        "string",
				Description: "Name parameter",
				Default:     "default-name",
			},
			{
				Name:        "count",
				Shorthand:   "c",
				Type:        "int",
				Description: "Count parameter",
				Default:     "42",
			},
			{
				Name:        "enabled",
				Shorthand:   "e",
				Type:        "bool",
				Description: "Enable feature",
				Default:     "true",
			},
			{
				Name:        "disabled",
				Shorthand:   "d",
				Type:        "bool",
				Description: "Disable feature",
				Default:     "false",
			},
			{
				Name:        "tags",
				Shorthand:   "t",
				Type:        "array",
				Description: "Tags list",
				Default:     "default-tag",
			},
			{
				Name:        "required-flag",
				Shorthand:   "r",
				Type:        "string",
				Description: "A required flag",
				Required:    true,
			},
			{
				Name:        "int-no-default",
				Shorthand:   "i",
				Type:        "int",
				Description: "Int with no default",
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)
	require.NotNil(t, cmd)

	// Verify command metadata
	assert.Equal(t, "test-cmd [flags]", cmd.Use)
	assert.Equal(t, "A test command", cmd.Short)
	assert.Equal(t, "A longer description", cmd.Long)
	assert.Equal(t, "test-cmd --name foo --count 5", cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify string flag
	nameFlag := cmd.Flag("name")
	require.NotNil(t, nameFlag)
	assert.Equal(t, "n", nameFlag.Shorthand)
	assert.Equal(t, "default-name", nameFlag.DefValue)

	// Verify int flag with default
	countFlag := cmd.Flag("count")
	require.NotNil(t, countFlag)
	assert.Equal(t, "c", countFlag.Shorthand)

	// Verify bool flag with default true
	enabledFlag := cmd.Flag("enabled")
	require.NotNil(t, enabledFlag)
	assert.Equal(t, "e", enabledFlag.Shorthand)

	// Verify bool flag with default false
	disabledFlag := cmd.Flag("disabled")
	require.NotNil(t, disabledFlag)
	assert.Equal(t, "d", disabledFlag.Shorthand)

	// Verify array flag
	tagsFlag := cmd.Flag("tags")
	require.NotNil(t, tagsFlag)
	assert.Equal(t, "t", tagsFlag.Shorthand)

	// Verify required flag is marked
	reqFlag := cmd.Flag("required-flag")
	require.NotNil(t, reqFlag)
	anns := reqFlag.Annotations
	require.NotNil(t, anns)
	assert.Contains(t, anns[cobra.BashCompOneRequiredFlag], "true")

	// Verify int flag without default
	intFlag := cmd.Flag("int-no-default")
	require.NotNil(t, intFlag)
	assert.Equal(t, "i", intFlag.Shorthand)

	// Verify common flags added by reconstructCommand
	headerFlag := cmd.Flag("header")
	require.NotNil(t, headerFlag)
	assert.Equal(t, "H", headerFlag.Shorthand)

	outputFlag := cmd.Flag("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, "o", outputFlag.Shorthand)
	assert.Equal(t, "table", outputFlag.DefValue)
}

func TestReconstructCommand_BaseURLOverride(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://default.com")

	// No BaseURL in meta -> should use loader's baseURL
	meta := CommandMetadata{
		Name: "no-baseurl",
		Use:  "no-baseurl",
	}
	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)
	require.NotNil(t, cmd)
	assert.Equal(t, "no-baseurl", cmd.Name())

	// With BaseURL in meta
	meta2 := CommandMetadata{
		Name:    "with-baseurl",
		Use:     "with-baseurl",
		BaseURL: "http://override.com",
	}
	cmd2, err := loader.reconstructCommand(meta2)
	require.NoError(t, err)
	assert.Equal(t, "with-baseurl", cmd2.Name())
}

func TestReconstructCommand_NoFlags(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	meta := CommandMetadata{
		Name: "simple",
		Use:  "simple",
	}
	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)
	require.NotNil(t, cmd)
	assert.Equal(t, "simple", cmd.Use)
	// Even with no flags, reconstructCommand always adds --header and --output
	assert.NotNil(t, cmd.Flag("header"))
	assert.NotNil(t, cmd.Flag("output"))
}

// --- getFlagValue ---

func TestGetFlagValue_String(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	cmd := &cobra.Command{}
	cmd.Flags().String("my-flag", "hello", "test")
	cmd.Flags().Set("my-flag", "world")

	val, err := loader.getFlagValue(cmd, FlagMetadata{Name: "my-flag", Type: "string"})
	require.NoError(t, err)
	assert.Equal(t, "world", val)
}

func TestGetFlagValue_Int(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	cmd := &cobra.Command{}
	cmd.Flags().Int("my-flag", 0, "test")
	cmd.Flags().Set("my-flag", "42")

	val, err := loader.getFlagValue(cmd, FlagMetadata{Name: "my-flag", Type: "int"})
	require.NoError(t, err)
	assert.Equal(t, 42, val)
}

func TestGetFlagValue_Bool(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("my-flag", false, "test")
	cmd.Flags().Set("my-flag", "true")

	val, err := loader.getFlagValue(cmd, FlagMetadata{Name: "my-flag", Type: "bool"})
	require.NoError(t, err)
	assert.Equal(t, true, val)
}

func TestGetFlagValue_Array(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("my-flag", []string{}, "test")
	cmd.Flags().Set("my-flag", "a,b,c")

	val, err := loader.getFlagValue(cmd, FlagMetadata{Name: "my-flag", Type: "array"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, val)
}

func TestGetFlagValue_UnknownType(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	cmd := &cobra.Command{}

	val, err := loader.getFlagValue(cmd, FlagMetadata{Name: "unknown", Type: "float"})
	require.NoError(t, err)
	assert.Nil(t, val)
}

// --- createRunHandler / RunE execution ---

func TestRunE_ArgCountMismatchError(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	meta := CommandMetadata{
		Name: "test",
		Use:  "test [id]",
		Args: []string{"id"},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	// Call RunE with 0 args but we expect 1
	err = cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 1 arguments, got 0")
}

func TestRunE_SuccessfulGETRequest(t *testing.T) {
	// Set up test server
	var capturedMethod, capturedPath string
	var capturedHeaders map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedHeaders = make(map[string]string)
		for k, v := range r.Header {
			capturedHeaders[k] = strings.Join(v, ", ")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "get-test",
		Use:    "get-test",
		Short:  "Test GET",
		Method: "GET",
		Path:   "/api/test",
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	// Set up a buffer to capture stdout
	var buf strings.Builder
	cmd.SetOut(&buf)

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	assert.Equal(t, "GET", capturedMethod)
	assert.Equal(t, "/api/test", capturedPath)

	// Verify response was written to stdout
	assert.Contains(t, buf.String(), `{"status":"ok"}`)
}

func TestRunE_WithArgsAndQueryParams(t *testing.T) {
	var capturedPath, capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "get-resource",
		Use:    "get-resource [id]",
		Method: "GET",
		Path:   "/api/resource/{id}",
		Args:   []string{"id"},
		Flags: []FlagMetadata{
			{
				Name: "filter",
				Type: "string",
				Kind: FlagKindQuery,
			},
			{
				Name: "limit",
				Type: "int",
				Kind: FlagKindQuery,
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	// Set flag values so they appear as "Changed"
	err = cmd.Flags().Set("filter", "active")
	require.NoError(t, err)
	err = cmd.Flags().Set("limit", "10")
	require.NoError(t, err)

	err = cmd.RunE(cmd, []string{"123"})
	require.NoError(t, err)

	assert.Equal(t, "/api/resource/123", capturedPath)

	// Query params may be in any order
	assert.Contains(t, capturedQuery, "filter=active")
	assert.Contains(t, capturedQuery, "limit=10")
	assert.Contains(t, capturedQuery, "&")
}

func TestRunE_WithHeaders(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	// Set loader-level headers
	loader.SetHeaders([]string{"X-Custom: loader-value"})

	meta := CommandMetadata{
		Name:   "auth-test",
		Use:    "auth-test",
		Method: "GET",
		Path:   "/api/secure",
		Flags: []FlagMetadata{
			{
				Name: "authorization",
				Type: "string",
				Kind: FlagKindHeader,
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	// Set MUSTER_AUTH_TOKEN env var
	os.Setenv("MUSTER_AUTH_TOKEN", "test-token-123")
	defer os.Unsetenv("MUSTER_AUTH_TOKEN")

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token-123", capturedAuth)
}

func TestRunE_BodyFlags(t *testing.T) {
	var capturedBody string
	var capturedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"new"}`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "create-resource",
		Use:    "create-resource",
		Method: "POST",
		Path:   "/api/resources",
		Flags: []FlagMetadata{
			{
				Name: "name",
				Type: "string",
				Kind: FlagKindBody,
			},
			{
				Name:     "description",
				Type:     "string",
				Kind:     FlagKindBody,
				JSONName: "desc",
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	err = cmd.Flags().Set("name", "test-resource")
	require.NoError(t, err)
	err = cmd.Flags().Set("description", "A test resource")
	require.NoError(t, err)

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	assert.Equal(t, "POST", capturedMethod)

	var parsedBody map[string]interface{}
	err = json.Unmarshal([]byte(capturedBody), &parsedBody)
	require.NoError(t, err)
	assert.Equal(t, "test-resource", parsedBody["name"])
	assert.Equal(t, "A test resource", parsedBody["desc"])
}

func TestRunE_HTTPErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "error-test",
		Use:    "error-test",
		Method: "GET",
		Path:   "/api/error",
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	err = cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP error: 500 Internal Server Error")

	// Even on error, response body should be printed
	assert.Contains(t, buf.String(), `{"error":"server error"}`)
}

func TestRunE_UnchangedQueryFlagsExcluded(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "query-test",
		Use:    "query-test",
		Method: "GET",
		Path:   "/api/items",
		Flags: []FlagMetadata{
			{
				Name: "filter",
				Type: "string",
				Kind: FlagKindQuery,
			},
			{
				Name: "sort",
				Type: "string",
				Kind: FlagKindQuery,
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	// Only set filter, not sort
	err = cmd.Flags().Set("filter", "active")
	require.NoError(t, err)

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	// Only filter should appear in query params
	assert.Equal(t, "filter=active", capturedQuery)
}

func TestRunE_PUTWithBody(t *testing.T) {
	var capturedMethod, capturedBody, capturedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedContentType = r.Header.Get("Content-Type")
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`updated`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "update-resource",
		Use:    "update-resource",
		Method: "PUT",
		Path:   "/api/resource/1",
		Flags: []FlagMetadata{
			{
				Name: "value",
				Type: "string",
				Kind: FlagKindBody,
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	err = cmd.Flags().Set("value", "new-value")
	require.NoError(t, err)

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	assert.Equal(t, "PUT", capturedMethod)
	assert.Contains(t, capturedContentType, "application/json")

	var parsedBody map[string]interface{}
	json.Unmarshal([]byte(capturedBody), &parsedBody)
	assert.Equal(t, "new-value", parsedBody["value"])
}

func TestRunE_GETNoBodyFlags(t *testing.T) {
	var capturedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "get-only",
		Use:    "get-only",
		Method: "GET",
		Path:   "/api/data",
		Flags: []FlagMetadata{
			{
				Name: "payload",
				Type: "string",
				Kind: FlagKindBody,
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	// GET should not send body even with body-kind flags
	err = cmd.Flags().Set("payload", "test")
	require.NoError(t, err)

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	assert.Equal(t, "GET", capturedMethod)
}

func TestRunE_FileUpload(t *testing.T) {
	// Create a temp file to upload
	tmpFile, err := os.CreateTemp(t.TempDir(), "upload-*.txt")
	require.NoError(t, err)
	tmpFile.WriteString("file content")
	tmpFile.Close()

	var capturedMethod string
	var capturedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`uploaded`))
	}))
	defer server.Close()

	store := NewCommandStore()
	loader := NewLoader(store, server.URL)

	meta := CommandMetadata{
		Name:   "upload",
		Use:    "upload",
		Method: "POST",
		Path:   "/api/upload",
		Flags: []FlagMetadata{
			{
				Name: "file",
				Type: "string",
				Kind: FlagKindFile,
			},
		},
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)

	// Set the file flag value
	err = cmd.Flags().Set("file", tmpFile.Name())
	require.NoError(t, err)

	var buf strings.Builder
	cmd.SetOut(&buf)

	err = cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	assert.Equal(t, "POST", capturedMethod)
	assert.Contains(t, capturedContentType, "multipart/form-data")
}
