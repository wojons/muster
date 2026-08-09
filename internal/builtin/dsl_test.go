package builtin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// =============================================================================
// HTTP builtin tests
// =============================================================================

func startTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// makeStarlarkThread creates a minimal Starlark thread for testing HTTP builtins.
func makeStarlarkThread(name string) *starlark.Thread {
	return &starlark.Thread{Name: name}
}

// =============================================================================
// httpGetBuiltin
// =============================================================================

// TestHttpGetBuiltin_ValidURL verifies that httpGetBuiltin returns the status
// code and response body from a valid URL.
func TestHttpGetBuiltin_ValidURL(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/test")}

	result, err := httpGetBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple, ok := result.(starlark.Tuple)
	if !ok {
		t.Fatalf("expected Tuple, got %T", result)
	}
	if len(tuple) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(tuple))
	}

	statusCode, ok := tuple[0].(starlark.Int)
	if !ok {
		t.Fatalf("expected Int for status code, got %T", tuple[0])
	}
	code, _ := statusCode.Int64()
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}

	body, ok := tuple[1].(starlark.String)
	if !ok {
		t.Fatalf("expected String for body, got %T", tuple[1])
	}
	if string(body) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(body))
	}
}

// TestHttpGetBuiltin_ErrorStatus verifies that httpGetBuiltin returns non-200
// status codes correctly.
func TestHttpGetBuiltin_ErrorStatus(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/missing")}

	result, err := httpGetBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 404 {
		t.Errorf("expected 404, got %d", code)
	}
	body := string(tuple[1].(starlark.String))
	if body != "not found" {
		t.Errorf("expected 'not found', got %q", body)
	}
}

// TestHttpGetBuiltin_InvalidURL verifies that httpGetBuiltin returns an error
// for an invalid URL.
func TestHttpGetBuiltin_InvalidURL(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String("http://invalid.example.invalid:12345/nonexistent")}

	_, err := httpGetBuiltin(thread, nil, args, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// TestHttpGetBuiltin_MissingArg verifies that httpGetBuiltin returns an error
// when the url argument is missing.
func TestHttpGetBuiltin_MissingArg(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{}

	_, err := httpGetBuiltin(thread, nil, args, nil)
	if err == nil {
		t.Fatal("expected error for missing url argument")
	}
	if !strings.Contains(err.Error(), "http_get") {
		t.Errorf("expected error to contain 'http_get', got: %v", err)
	}
}

// =============================================================================
// httpPostBuiltin
// =============================================================================

// TestHttpPostBuiltin_Valid verifies that httpPostBuiltin sends a POST request
// and returns the response.
func TestHttpPostBuiltin_Valid(t *testing.T) {
	var receivedBody string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1}`))
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/create"), starlark.String(`{"name":"test"}`)}

	result, err := httpPostBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 201 {
		t.Errorf("expected 201, got %d", code)
	}
	if receivedBody != `{"name":"test"}` {
		t.Errorf("expected body '{\"name\":\"test\"}', got %q", receivedBody)
	}
}

// TestHttpPostBuiltin_NoBody verifies that httpPostBuiltin works when body is
// omitted (optional parameter).
func TestHttpPostBuiltin_NoBody(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/ping")}

	result, err := httpPostBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}
}

// TestHttpPostBuiltin_InvalidURL verifies that httpPostBuiltin returns an error
// for an unreachable URL.
func TestHttpPostBuiltin_InvalidURL(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String("http://invalid.example.invalid:12345/api")}

	_, err := httpPostBuiltin(thread, nil, args, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// =============================================================================
// httpPutBuiltin
// =============================================================================

// TestHttpPutBuiltin_Valid verifies that httpPutBuiltin sends a PUT request
// with the correct method and returns the response.
func TestHttpPutBuiltin_Valid(t *testing.T) {
	var receivedMethod string
	var receivedBody string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"updated":true}`))
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/update"), starlark.String(`{"name":"updated"}`)}

	result, err := httpPutBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}
	if receivedMethod != "PUT" {
		t.Errorf("expected PUT method, got %s", receivedMethod)
	}
	if receivedBody != `{"name":"updated"}` {
		t.Errorf("expected body '{\"name\":\"updated\"}', got %q", receivedBody)
	}
}

// TestHttpPutBuiltin_EmptyBody verifies that httpPutBuiltin works with an empty
// body (optional parameter).
func TestHttpPutBuiltin_EmptyBody(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/reset")}

	result, err := httpPutBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 204 {
		t.Errorf("expected 204, got %d", code)
	}
}

// TestHttpPutBuiltin_InvalidURL verifies that httpPutBuiltin returns an error
// for an invalid/new-request URL.
func TestHttpPutBuiltin_InvalidURL(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String("://bad-url")}

	_, err := httpPutBuiltin(thread, nil, args, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// =============================================================================
// httpDeleteBuiltin
// =============================================================================

// TestHttpDeleteBuiltin_Valid verifies that httpDeleteBuiltin sends a DELETE
// request and returns the response.
func TestHttpDeleteBuiltin_Valid(t *testing.T) {
	var receivedMethod string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL + "/resource/1")}

	result, err := httpDeleteBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 204 {
		t.Errorf("expected 204, got %d", code)
	}
	if receivedMethod != "DELETE" {
		t.Errorf("expected DELETE method, got %s", receivedMethod)
	}
}

// TestHttpDeleteBuiltin_MissingArg verifies that httpDeleteBuiltin returns an
// error when the url argument is missing.
func TestHttpDeleteBuiltin_MissingArg(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{}

	_, err := httpDeleteBuiltin(thread, nil, args, nil)
	if err == nil {
		t.Fatal("expected error for missing url argument")
	}
}

// TestHttpDeleteBuiltin_InvalidURL verifies that httpDeleteBuiltin returns an
// error for an unreachable URL.
func TestHttpDeleteBuiltin_InvalidURL(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String("http://invalid.example.invalid:12345/resource")}

	_, err := httpDeleteBuiltin(thread, nil, args, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// =============================================================================
// runDsl tests
// =============================================================================

// TestRunDsl_SimpleScript verifies that runDsl can execute a simple Starlark
// script that calls http_get and prints globals.
func TestRunDsl_SimpleScript(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from server"))
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "test.star")
	scriptContent := "url = '" + srv.URL + "'\n" +
		"code, body = http_get(url)\n" +
		"result = body\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	// runDsl expects args[1] to be the script path (args[0] is the command name)
	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err != nil {
		t.Fatalf("runDsl failed: %v", err)
	}
}

// TestRunDsl_FileNotFound verifies that runDsl returns an error for a
// non-existent file.
func TestRunDsl_FileNotFound(t *testing.T) {
	err := runDsl(nil, []string{"dsl", "/nonexistent/path/script.star"}, false)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "reading script") {
		t.Errorf("expected 'reading script' error, got: %v", err)
	}
}

// TestRunDsl_ParseError verifies that runDsl returns a parse error for invalid
// Starlark syntax.
func TestRunDsl_ParseError(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "broken.star")
	if err := os.WriteFile(scriptPath, []byte("this is not valid starlark {{{"), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err == nil {
		t.Fatal("expected parse error for invalid syntax")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("expected 'parse error', got: %v", err)
	}
}

// TestRunDsl_ExecError verifies that runDsl returns an error when the
// script references an undefined variable (caught at compile time).
func TestRunDsl_ExecError(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "exec_err.star")
	if err := os.WriteFile(scriptPath, []byte("undefined_var + 1"), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	// Undefined vars are caught at compile time, not execution time
	if !strings.Contains(err.Error(), "error") {
		t.Errorf("expected error to contain 'error', got: %v", err)
	}
}

// TestRunDsl_HttpBuiltinIntegration verifies full integration: Starlark script
// calls http_get builtin via runDsl and receives real HTTP response.
func TestRunDsl_HttpBuiltinIntegration(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","count":42}`))
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "http_test.star")
	scriptContent := "code, body = http_get('" + srv.URL + "')\n" +
		"parsed = body\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err != nil {
		t.Fatalf("runDsl with http_get failed: %v", err)
	}
}

// TestRunDsl_HttpPostIntegration verifies runDsl with http_post.
func TestRunDsl_HttpPostIntegration(t *testing.T) {
	var receivedBody string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "post_test.star")
	scriptContent := "code, body = http_post('" + srv.URL + "', '{\"key\":\"val\"}')\n" +
		"result = body\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err != nil {
		t.Fatalf("runDsl with http_post failed: %v", err)
	}
	if receivedBody != `{"key":"val"}` {
		t.Errorf("expected body '{\"key\":\"val\"}', got %q", receivedBody)
	}
}

// TestRunDsl_HttpPutIntegration verifies runDsl with http_put.
func TestRunDsl_HttpPutIntegration(t *testing.T) {
	var receivedMethod string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.Write([]byte("updated"))
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "put_test.star")
	scriptContent := "code, body = http_put('" + srv.URL + "', 'data')\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err != nil {
		t.Fatalf("runDsl with http_put failed: %v", err)
	}
	if receivedMethod != "PUT" {
		t.Errorf("expected PUT method, got %s", receivedMethod)
	}
}

// TestRunDsl_HttpDeleteIntegration verifies runDsl with http_delete.
func TestRunDsl_HttpDeleteIntegration(t *testing.T) {
	var receivedMethod string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "delete_test.star")
	scriptContent := "code, body = http_delete('" + srv.URL + "')\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	err := runDsl(nil, []string{"dsl", scriptPath}, false)
	if err != nil {
		t.Fatalf("runDsl with http_delete failed: %v", err)
	}
	if receivedMethod != "DELETE" {
		t.Errorf("expected DELETE method, got %s", receivedMethod)
	}
}

// =============================================================================
// Interface check — ensure builtins are callable functions
// =============================================================================
func TestDslBuiltins_AreCallable(t *testing.T) {
	// Verify all builtins are declared functions (compile-time check)
	// Call each with empty args to verify they handle missing args gracefully
	thread := makeStarlarkThread("test")
	emptyArgs := starlark.Tuple{}

	_, err := httpGetBuiltin(thread, nil, emptyArgs, nil)
	if err == nil {
		t.Error("httpGetBuiltin should error on empty args")
	}

	_, err = httpPostBuiltin(thread, nil, emptyArgs, nil)
	if err == nil {
		t.Error("httpPostBuiltin should error on empty args")
	}

	_, err = httpPutBuiltin(thread, nil, emptyArgs, nil)
	if err == nil {
		t.Error("httpPutBuiltin should error on empty args")
	}

	_, err = httpDeleteBuiltin(thread, nil, emptyArgs, nil)
	if err == nil {
		t.Error("httpDeleteBuiltin should error on empty args")
	}
}

// TestNewDslCommand verifies that newDslCommand creates a valid cobra command.
func TestNewDslCommand(t *testing.T) {
	cmd := newDslCommand()
	if cmd == nil {
		t.Fatal("newDslCommand returned nil")
	}
	if cmd.Use != "dsl run <file.star>" {
		t.Errorf("expected Use 'dsl run <file.star>', got %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("RunE should not be nil")
	}

	// No file → should return error
	err := cmd.RunE(cmd, []string{"dsl", "/nonexistent"})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// TestDslCommand_NoArgs verifies that runDsl panics with 0 args (no cobra
// validation — ExactArgs is enforced by cobra before RunE).
func TestDslCommand_NoArgs(t *testing.T) {
	// runDsl accesses args[1] directly — without cobra validation, this panics.
	// This test verifies the expected behavior: cobra's ExactArgs prevents this path.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with 0 args")
		}
	}()
	runDsl(nil, []string{}, false)
}

// TestDslCommand_ExtraArgs verifies that runDsl ignores extra args beyond
// args[1] (the script path). Extra args are a cobra validation concern.
func TestDslCommand_ExtraArgs(t *testing.T) {
	// runDsl only reads args[1]; extra args are ignored
	err := runDsl(nil, []string{"dsl", "/nonexistent", "extra"}, false)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// =============================================================================
// Quiet/script mode tests (AC2)
// =============================================================================

// TestRunDsl_QuietMode_PrintOnly verifies that dsl run in default (quiet) mode
// shows only the print() output — no global binding dump, no structured
// log (INF/INFO) lines.
func TestRunDsl_QuietMode_PrintOnly(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "print_test.star")
	scriptContent := `x = 42
y = "hello"
print("Hello, World!")
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	output := captureStdoutRequest(func() {
		err := runDsl(nil, []string{"dsl", scriptPath}, false)
		if err != nil {
			t.Fatalf("runDsl failed: %v", err)
		}
	})

	// The print() output should appear.
	assertContains(t, output, "Hello, World!")
	// No global binding dump (x = 42, y = hello).
	if strings.Contains(output, "x = 42") {
		t.Errorf("expected no binding dump for 'x', got: %s", output)
	}
	if strings.Contains(output, "y = hello") {
		t.Errorf("expected no binding dump for 'y', got: %s", output)
	}
	// No structured log lines.
	if strings.Contains(output, "INF") || strings.Contains(output, "INFO") {
		t.Errorf("expected no structured log lines, got: %s", output)
	}
}

// TestRunDsl_VerboseMode_PrintsBindings verifies that --verbose mode dumps
// global variable bindings after script execution.
func TestRunDsl_VerboseMode_PrintsBindings(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "verbose_test.star")
	scriptContent := `x = 42
print("done")
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	output := captureStdoutRequest(func() {
		err := runDsl(nil, []string{"dsl", scriptPath}, true)
		if err != nil {
			t.Fatalf("runDsl failed: %v", err)
		}
	})

	// The print() output should appear.
	assertContains(t, output, "done")
	// The binding dump should appear in verbose mode.
	assertContains(t, output, "x = 42")
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got: %s", substr, s)
	}
}
