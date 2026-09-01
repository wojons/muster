package cmdstore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// --- NewLoader ---

func TestNewLoader_ValidStore(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "http://example.com")
	require.NotNil(t, l)
	require.Equal(t, "http://example.com", l.baseURL)
	require.NotNil(t, l.store)
}

func TestNewLoader_NilStore(t *testing.T) {
	l := NewLoader(nil, "")
	require.NotNil(t, l)
	require.Nil(t, l.store)
	// Note: calling methods on nil store will panic — Go convention
	// The production code does not guard against nil store
}

// --- LoadCommands ---

func TestLoader_LoadCommands_Empty(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Empty(t, cmds)
}

func TestLoader_LoadCommands_WithCommands(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "list-pets",
			Use:    "list-pets",
			Short:  "List all pets",
			Method: "GET",
			Path:   "/pets",
		},
		{
			Name:   "get-pet",
			Use:    "get-pet [id]",
			Short:  "Get a pet by ID",
			Method: "GET",
			Path:   "/pets/{id}",
			Args:   []string{"id"},
		},
	}
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 2)

	names := make(map[string]*cobra.Command)
	for _, c := range cmds {
		names[c.Use] = c
	}
	require.Contains(t, names, "list-pets")
	require.Contains(t, names, "get-pet [id]")
}

// --- GetBaseURL ---

func TestLoader_GetBaseURL_FromMeta(t *testing.T) {
	cs := NewCommandStore()
	cs.meta = &CommandMetadata{BaseURL: "http://meta-url.com"}
	l := NewLoader(cs, "http://default-url.com")
	url, err := l.GetBaseURL()
	require.NoError(t, err)
	require.Equal(t, "http://meta-url.com", url)
}

func TestLoader_GetBaseURL_FromLoader(t *testing.T) {
	cs := NewCommandStore()
	cs.meta = &CommandMetadata{BaseURL: ""}
	l := NewLoader(cs, "http://default-url.com")
	url, err := l.GetBaseURL()
	require.NoError(t, err)
	require.Equal(t, "http://default-url.com", url)
}

func TestLoader_GetBaseURL_NoStoredData(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "http://default-url.com")
	url, err := l.GetBaseURL()
	require.Error(t, err)
	require.Equal(t, "", url)
	require.Contains(t, err.Error(), "no stored data")
}

// --- SetHeaders ---

func TestLoader_SetHeaders(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "http://example.com")
	require.Nil(t, l.headers)
	l.SetHeaders([]string{"X-Test: value1", "Authorization: Bearer tok"})
	require.Len(t, l.headers, 2)
	require.Equal(t, "X-Test: value1", l.headers[0])

	l.SetHeaders([]string{"X-New: value"})
	require.Len(t, l.headers, 1)
}

// --- reconstructCommand via LoadCommands ---

func TestLoader_ReconstructCommand_Flags(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:    "test-cmd",
			Use:     "test-cmd",
			Short:   "Test command",
			Long:    "Long description",
			Method:  "GET",
			Path:    "/test",
			Example: "muster test-cmd --name foo",
			Flags: []FlagMetadata{
				{Name: "name", Type: "string", Default: "default", Description: "Name flag"},
				{Name: "count", Type: "int", Default: "10", Description: "Count flag"},
				{Name: "verbose", Type: "bool", Default: "true", Description: "Verbose flag"},
				{Name: "tags", Type: "array", Default: "tag1", Description: "Tags flag"},
			},
		},
	}
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	cmd := cmds[0]
	require.Equal(t, "test-cmd", cmd.Use)
	require.Equal(t, "Long description", cmd.Long)
	require.Equal(t, "muster test-cmd --name foo", cmd.Example)

	require.NotNil(t, cmd.Flag("name"))
	require.NotNil(t, cmd.Flag("count"))
	require.NotNil(t, cmd.Flag("verbose"))
	require.NotNil(t, cmd.Flag("tags"))

	nameVal, _ := cmd.Flags().GetString("name")
	require.Equal(t, "default", nameVal)

	countVal, _ := cmd.Flags().GetInt("count")
	require.Equal(t, 10, countVal)

	verboseVal, _ := cmd.Flags().GetBool("verbose")
	require.True(t, verboseVal)

	tagsVal, _ := cmd.Flags().GetStringSlice("tags")
	require.Equal(t, []string{"tag1"}, tagsVal)
}

func TestLoader_ReconstructCommand_RequiredFlags(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "required-cmd",
			Use:    "required-cmd",
			Short:  "Has required flags",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{Name: "required-flag", Type: "string", Required: true},
			},
		},
	}
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmd := cmds[0]
	require.NotNil(t, cmd.Flag("required-flag"))
}

func TestLoader_ReconstructCommand_BuiltinFlags(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "simple-cmd",
			Use:    "simple-cmd",
			Short:  "Simple command",
			Method: "GET",
			Path:   "/test",
		},
	}
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	cmd := cmds[0]

	require.NotNil(t, cmd.Flag("header"))
	require.NotNil(t, cmd.Flag("output"))
}

func TestLoader_ReconstructCommand_Args(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "get-item",
			Use:    "get-item [id]",
			Short:  "Get an item",
			Method: "GET",
			Path:   "/items/{id}",
			Args:   []string{"id"},
		},
	}
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	require.Equal(t, "get-item [id]", cmds[0].Use)
}

// --- getFlagValue ---

func TestLoader_GetFlagValue_String(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "")
	cmd := &cobra.Command{}
	cmd.Flags().String("name", "test-val", "")
	_ = cmd.ParseFlags([]string{"--name", "hello"})

	val, err := l.getFlagValue(cmd, FlagMetadata{Name: "name", Type: "string"})
	require.NoError(t, err)
	require.Equal(t, "hello", val)
}

func TestLoader_GetFlagValue_Int(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "")
	cmd := &cobra.Command{}
	cmd.Flags().Int("count", 0, "")
	_ = cmd.ParseFlags([]string{"--count", "42"})

	val, err := l.getFlagValue(cmd, FlagMetadata{Name: "count", Type: "int"})
	require.NoError(t, err)
	require.Equal(t, 42, val)
}

func TestLoader_GetFlagValue_Bool(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "")
	cmd := &cobra.Command{}
	cmd.Flags().Bool("verbose", false, "")
	_ = cmd.ParseFlags([]string{"--verbose"})

	val, err := l.getFlagValue(cmd, FlagMetadata{Name: "verbose", Type: "bool"})
	require.NoError(t, err)
	require.Equal(t, true, val)
}

func TestLoader_GetFlagValue_Array(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "")
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("tags", nil, "")
	_ = cmd.ParseFlags([]string{"--tags", "a", "--tags", "b"})

	val, err := l.getFlagValue(cmd, FlagMetadata{Name: "tags", Type: "array"})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, val)
}

func TestLoader_GetFlagValue_UnknownType(t *testing.T) {
	cs := NewCommandStore()
	l := NewLoader(cs, "")
	cmd := &cobra.Command{}
	val, err := l.getFlagValue(cmd, FlagMetadata{Name: "unknown", Type: "unknown"})
	require.NoError(t, err)
	require.Nil(t, val)
}

// --- createRunHandler (via executeAndReadResponse) ---

func TestLoader_CreateRunHandler_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/pets", r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":1,"name":"fido"}]`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "list-pets",
			Use:    "list-pets",
			Method: "GET",
			Path:   "/pets",
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
	require.Contains(t, buf.String(), `[{"id":1,"name":"fido"}]`)
}

func TestLoader_CreateRunHandler_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "list-pets",
			Use:    "list-pets",
			Method: "GET",
			Path:   "/pets",
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	err = cmds[0].RunE(cmds[0], []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP error")
	require.Contains(t, buf.String(), `{"error":"internal"}`)
}

func TestLoader_CreateRunHandler_ConnectionRefused(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "list-pets",
			Use:    "list-pets",
			Method: "GET",
			Path:   "/pets",
		},
	}
	l := NewLoader(cs, "http://127.0.0.1:1")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	err = cmds[0].RunE(cmds[0], []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execute request")
}

func TestLoader_CreateRunHandler_ArgsMismatch(t *testing.T) {
	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "get-pet",
			Use:    "get-pet [id]",
			Method: "GET",
			Path:   "/pets/{id}",
			Args:   []string{"id"},
		},
	}
	l := NewLoader(cs, "http://example.com")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	err = cmds[0].RunE(cmds[0], []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 arguments")
}

func TestLoader_CreateRunHandler_PostBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		require.Equal(t, "fido", body["name"])
		require.Equal(t, float64(3), body["age"])

		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":1,"name":"fido"}`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "create-pet",
			Use:    "create-pet",
			Method: "POST",
			Path:   "/pets",
			Flags: []FlagMetadata{
				{Name: "name", Type: "string", Kind: FlagKindBody, JSONName: "name"},
				{Name: "age", Type: "int", Kind: FlagKindBody, JSONName: "age"},
			},
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	_ = cmds[0].Flags().Set("name", "fido")
	_ = cmds[0].Flags().Set("age", "3")

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
	require.Contains(t, buf.String(), `{"id":1,"name":"fido"}`)
}

func TestLoader_CreateRunHandler_HeaderFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-value", r.Header.Get("X-Custom"))
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "header-test",
			Use:    "header-test",
			Method: "GET",
			Path:   "/test",
			Flags: []FlagMetadata{
				{Name: "X-Custom", Type: "string", Kind: FlagKindHeader},
			},
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)

	_ = cmds[0].Flags().Set("X-Custom", "test-value")

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
}

func TestLoader_CreateRunHandler_QueryFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "10", r.URL.Query().Get("limit"))
		require.Equal(t, "2", r.URL.Query().Get("page"))
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "list-with-query",
			Use:    "list-with-query",
			Method: "GET",
			Path:   "/items",
			Flags: []FlagMetadata{
				{Name: "limit", Type: "int", Kind: ""},
				{Name: "page", Type: "int", Kind: ""},
			},
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)

	_ = cmds[0].Flags().Set("limit", "10")
	_ = cmds[0].Flags().Set("page", "2")

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
}

func TestLoader_CreateRunHandler_FileUpload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)

		file, _, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()

		buf := make([]byte, 100)
		n, _ := file.Read(buf)
		require.Equal(t, "file-content", string(buf[:n]))

		w.WriteHeader(200)
		_, _ = w.Write([]byte(`uploaded`))
	}))
	defer srv.Close()

	tmpFile := t.TempDir() + "/test-upload.txt"
	require.NoError(t, os.WriteFile(tmpFile, []byte("file-content"), 0644))

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "upload",
			Use:    "upload",
			Method: "POST",
			Path:   "/upload",
			Flags: []FlagMetadata{
				{Name: "file", Type: "string", Kind: FlagKindFile},
			},
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)

	_ = cmds[0].Flags().Set("file", tmpFile)

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
}

func TestLoader_CreateRunHandler_AuthEnvVar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token-123", r.Header.Get("Authorization"))
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "protected",
			Use:    "protected",
			Method: "GET",
			Path:   "/protected",
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)

	t.Setenv("MUSTER_AUTH_TOKEN", "test-token-123")

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
}

func TestLoader_CreateRunHandler_GlobalHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "global-value", r.Header.Get("X-Global"))
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "test",
			Use:    "test",
			Method: "GET",
			Path:   "/test",
		},
	}
	l := NewLoader(cs, srv.URL)
	l.SetHeaders([]string{"X-Global: global-value"})

	cmds, err := l.LoadCommands()
	require.NoError(t, err)

	buf := new(testWriter)
	cmds[0].SetOut(buf)

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
}

// testWriter captures stdout for testing
type testWriter struct {
	data []byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *testWriter) String() string {
	return string(w.data)
}

func TestLoader_CreateRunHandler_BaseURLFlagOverride(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		require.Equal(t, "/pets", r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{Name: "list-pets", Use: "list-pets", Method: "GET", Path: "/pets"},
	}
	// Stored/generate-time URL points at an unreachable address; the
	// explicit --base-url flag must win without regenerating.
	l := NewLoader(cs, "http://127.0.0.1:1")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	require.NoError(t, cmds[0].Flags().Set("base-url", srv.URL))

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	require.NoError(t, cmds[0].RunE(cmds[0], []string{}))
	require.Equal(t, srv.Listener.Addr().String(), gotHost)
	require.Contains(t, buf.String(), `[{"id":1}]`)
}

func TestLoader_CreateRunHandler_BaseURLEnvOverride(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()
	t.Setenv("MUSTER_BASE_URL", srv.URL)

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{Name: "list-pets", Use: "list-pets", Method: "GET", Path: "/pets"},
	}
	l := NewLoader(cs, "http://127.0.0.1:1")
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	require.NoError(t, cmds[0].RunE(cmds[0], []string{}))
	require.Equal(t, srv.Listener.Addr().String(), gotHost)
}

func TestLoader_CreateRunHandler_RawBodyFlag(t *testing.T) {
	var gotBody string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		require.Empty(t, r.URL.RawQuery, "--body must not leak into the query string")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "create-task",
			Use:    "create-task",
			Method: "POST",
			Path:   "/tasks",
			Flags: []FlagMetadata{
				{Name: "body", Shorthand: "b", Type: "string", Kind: FlagKindRawBody},
				{Name: "title", Type: "string", Kind: FlagKindBody, JSONName: "title"},
			},
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	// Reconstructed command exposes --body with -b shorthand
	bodyFlag := cmds[0].Flag("body")
	require.NotNil(t, bodyFlag, "reconstructed command should expose --body")
	require.Equal(t, "b", bodyFlag.Shorthand)

	// --body wins over per-property body flags: raw JSON sent verbatim
	raw := `{"title":"raw","nested":{"k":[1,2]}}`
	require.NoError(t, cmds[0].Flags().Set("body", raw))
	require.NoError(t, cmds[0].Flags().Set("title", "ignored"))

	err = cmds[0].RunE(cmds[0], []string{})
	require.NoError(t, err)
	require.Equal(t, raw, gotBody)
	require.Equal(t, "application/json", gotContentType)
	require.Contains(t, buf.String(), `{"id":1}`)
}

func TestLoader_CreateRunHandler_RawBodyFlag_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not be sent for invalid --body JSON")
	}))
	defer srv.Close()

	cs := NewCommandStore()
	cs.commands = []CommandMetadata{
		{
			Name:   "create-task",
			Use:    "create-task",
			Method: "POST",
			Path:   "/tasks",
			Flags: []FlagMetadata{
				{Name: "body", Shorthand: "b", Type: "string", Kind: FlagKindRawBody},
			},
		},
	}
	l := NewLoader(cs, srv.URL)
	cmds, err := l.LoadCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	buf := new(testWriter)
	cmds[0].SetOut(buf)
	cmds[0].SetErr(buf)

	require.NoError(t, cmds[0].Flags().Set("body", "{not-json"))
	err = cmds[0].RunE(cmds[0], []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON for --body")
}
