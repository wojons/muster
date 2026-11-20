package builtin

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestHttpGetBuiltin_WithHeaders verifies that httpGetBuiltin forwards an
// optional headers dict to the outgoing request (auth use case).
func TestHttpGetBuiltin_WithHeaders(t *testing.T) {
	var gotAuth, gotCustom string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	thread := makeStarlarkThread("test")
	headers := starlark.NewDict(2)
	if err := headers.SetKey(starlark.String("Authorization"), starlark.String("Bearer tok123")); err != nil {
		t.Fatalf("failed to build headers dict: %v", err)
	}
	if err := headers.SetKey(starlark.String("X-Test"), starlark.String("yes")); err != nil {
		t.Fatalf("failed to build headers dict: %v", err)
	}
	args := starlark.Tuple{starlark.String(srv.URL + "/secure"), headers}

	result, err := httpGetBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("expected Authorization header 'Bearer tok123', got %q", gotAuth)
	}
	if gotCustom != "yes" {
		t.Errorf("expected X-Test header 'yes', got %q", gotCustom)
	}
}

// TestHttpGetBuiltin_HeadersNone verifies that an explicit headers=None is
// accepted and sends no extra headers (backward compatibility).
func TestHttpGetBuiltin_HeadersNone(t *testing.T) {
	var gotAuth string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String(srv.URL), starlark.None}

	if _, err := httpGetBuiltin(thread, nil, args, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header, got %q", gotAuth)
	}
}

// TestHttpGetBuiltin_HeadersInvalidType verifies that a non-dict headers value
// is rejected with an error.
func TestHttpGetBuiltin_HeadersInvalidType(t *testing.T) {
	thread := makeStarlarkThread("test")
	args := starlark.Tuple{starlark.String("http://invalid.example.invalid"), starlark.String("not-a-dict")}

	if _, err := httpGetBuiltin(thread, nil, args, nil); err == nil {
		t.Fatal("expected error for non-dict headers, got nil")
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

// TestHttpPostBuiltin_WithHeaders verifies that httpPostBuiltin forwards an
// optional headers dict, enabling bearer-auth POSTs that previously failed
// with the upstream 401 echoed as body text (DF-037 repro shape).
func TestHttpPostBuiltin_WithHeaders(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"UNAUTHENTICATED"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":7}`))
	})

	thread := makeStarlarkThread("test")
	headers := starlark.NewDict(1)
	if err := headers.SetKey(starlark.String("Authorization"), starlark.String("Bearer tok123")); err != nil {
		t.Fatalf("failed to build headers dict: %v", err)
	}
	args := starlark.Tuple{starlark.String(srv.URL + "/sessions"), starlark.String(`{"prompt":"hi"}`), headers}

	result, err := httpPostBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 201 {
		t.Errorf("expected 201 with auth header, got %d (body %q)", code, string(tuple[1].(starlark.String)))
	}
}

// TestHttpPostBuiltin_HeadersContentTypeOverride verifies a headers-supplied
// Content-Type wins over the builtin default.
func TestHttpPostBuiltin_HeadersContentTypeOverride(t *testing.T) {
	var gotCT string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	thread := makeStarlarkThread("test")
	headers := starlark.NewDict(1)
	if err := headers.SetKey(starlark.String("Content-Type"), starlark.String("text/plain")); err != nil {
		t.Fatalf("failed to build headers dict: %v", err)
	}
	args := starlark.Tuple{starlark.String(srv.URL), starlark.String("plain"), headers}

	if _, err := httpPostBuiltin(thread, nil, args, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCT != "text/plain" {
		t.Errorf("expected Content-Type override 'text/plain', got %q", gotCT)
	}
}

// TestHttpPutBuiltin_WithHeaders verifies that httpPutBuiltin forwards an
// optional headers dict to the outgoing request.
func TestHttpPutBuiltin_WithHeaders(t *testing.T) {
	var gotAuth string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("updated"))
	})

	thread := makeStarlarkThread("test")
	headers := starlark.NewDict(1)
	if err := headers.SetKey(starlark.String("Authorization"), starlark.String("Bearer tok123")); err != nil {
		t.Fatalf("failed to build headers dict: %v", err)
	}
	args := starlark.Tuple{starlark.String(srv.URL + "/update"), starlark.String(`{"a":1}`), headers}

	result, err := httpPutBuiltin(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tuple := result.(starlark.Tuple)
	code, _ := tuple[0].(starlark.Int).Int64()
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("expected Authorization header 'Bearer tok123', got %q", gotAuth)
	}
}

// TestHttpDeleteBuiltin_WithHeaders verifies that httpDeleteBuiltin forwards
// an optional headers dict to the outgoing request.
func TestHttpDeleteBuiltin_WithHeaders(t *testing.T) {
	var gotAuth string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	})

	thread := makeStarlarkThread("test")
	headers := starlark.NewDict(1)
	if err := headers.SetKey(starlark.String("Authorization"), starlark.String("Bearer tok123")); err != nil {
		t.Fatalf("failed to build headers dict: %v", err)
	}
	args := starlark.Tuple{starlark.String(srv.URL + "/resource/1"), headers}

	if _, err := httpDeleteBuiltin(thread, nil, args, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("expected Authorization header 'Bearer tok123', got %q", gotAuth)
	}
}

// TestRunDsl_HttpAuthedWorkflow verifies an authed 3-step workflow
// (create -> message -> poll) end to end through runDsl, the exact scenario
// DF-037 found impossible before headers support.
func TestRunDsl_HttpAuthedWorkflow(t *testing.T) {
	type captured struct {
		auth   string
		hasKey bool
	}
	var steps [3]captured
	mu := &sync.Mutex{}
	n := 0
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := n
		n++
		mu.Unlock()
		if i < len(steps) {
			steps[i] = captured{auth: r.Header.Get("Authorization"), hasKey: r.Header.Get("X-Api-Key") != ""}
		}
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"UNAUTHENTICATED"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "authed_workflow.star")
	scriptContent := "auth = {'Authorization': 'Bearer tok123', 'X-Api-Key': 'k-123'}\n" +
		"code1, body1 = http_post('" + srv.URL + "/sessions', '{}', auth)\n" +
		"code2, body2 = http_post('" + srv.URL + "/messages', '{\"text\":\"hi\"}', auth)\n" +
		"code3, body3 = http_get('" + srv.URL + "/poll', auth)\n" +
		"result = str(code1) + ',' + str(code2) + ',' + str(code3)\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	if err := runDsl(nil, []string{"dsl", scriptPath}, false); err != nil {
		t.Fatalf("runDsl authed workflow failed: %v", err)
	}
	for i, s := range steps {
		if s.auth != "Bearer tok123" || !s.hasKey {
			t.Errorf("step %d: expected both headers, got auth=%q hasKey=%v", i+1, s.auth, s.hasKey)
		}
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

// =============================================================================
// Body types + json() encoder (DF-MUSTER-7)
// =============================================================================

// starPair builds a (key, value) tuple for newStarDict.
func starPair(k, v starlark.Value) starlark.Tuple { return starlark.Tuple{k, v} }

// newStarDict builds a Starlark dict from key/value pairs.
func newStarDict(t *testing.T, pairs ...starlark.Tuple) *starlark.Dict {
	t.Helper()
	d := starlark.NewDict(len(pairs))
	for _, p := range pairs {
		if len(p) != 2 {
			t.Fatalf("newStarDict: pair has %d elements, want 2", len(p))
		}
		if err := d.SetKey(p[0], p[1]); err != nil {
			t.Fatalf("newStarDict: SetKey(%s): %v", p[0].String(), err)
		}
	}
	return d
}

// TestHttpPostBuiltin_BodyTypes verifies that http_post accepts a dict (and
// the other JSON-encodable Starlark values) as its body, marshals them to JSON
// and sends Content-Type: application/json, while a plain string body is still
// sent verbatim.
func TestHttpPostBuiltin_BodyTypes(t *testing.T) {
	tests := []struct {
		name     string
		body     starlark.Value // nil means "body argument omitted"
		wantBody string
	}{
		{
			name:     "string sent verbatim",
			body:     starlark.String(`{"pre":"encoded"}`),
			wantBody: `{"pre":"encoded"}`,
		},
		{
			name:     "dict marshalled to JSON object",
			body:     newStarDict(t, starPair(starlark.String("title"), starlark.String("x"))),
			wantBody: `{"title":"x"}`,
		},
		{
			name: "dict with mixed value types (keys sorted)",
			body: newStarDict(t,
				starPair(starlark.String("title"), starlark.String("x")),
				starPair(starlark.String("count"), starlark.MakeInt(2)),
			),
			wantBody: `{"count":2,"title":"x"}`,
		},
		{
			name: "nested dict and list",
			body: newStarDict(t,
				starPair(starlark.String("meta"), newStarDict(t, starPair(starlark.String("n"), starlark.MakeInt(1)))),
				starPair(starlark.String("tags"), starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")})),
			),
			wantBody: `{"meta":{"n":1},"tags":["a","b"]}`,
		},
		{
			name:     "list marshalled to JSON array",
			body:     starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)}),
			wantBody: `[1,2,3]`,
		},
		{
			name:     "tuple marshalled to JSON array",
			body:     starlark.Tuple{starlark.String("a"), starlark.MakeInt(1)},
			wantBody: `["a",1]`,
		},
		{
			name:     "int marshalled to JSON number",
			body:     starlark.MakeInt(42),
			wantBody: `42`,
		},
		{
			name:     "bool marshalled to JSON literal",
			body:     starlark.True,
			wantBody: `true`,
		},
		{
			name:     "None sends empty body",
			body:     starlark.None,
			wantBody: ``,
		},
		{
			name:     "omitted body sends empty body",
			body:     nil,
			wantBody: ``,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody, gotCT string
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("server: reading body: %v", err)
				}
				gotBody = string(b)
				gotCT = r.Header.Get("Content-Type")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"ok":true}`))
			})

			thread := makeStarlarkThread("test")
			args := starlark.Tuple{starlark.String(srv.URL + "/items")}
			if tc.body != nil {
				args = append(args, tc.body)
			}

			result, err := httpPostBuiltin(thread, nil, args, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tuple, ok := result.(starlark.Tuple)
			if !ok {
				t.Fatalf("expected Tuple, got %T", result)
			}
			code, _ := tuple[0].(starlark.Int).Int64()
			if code != 200 {
				t.Errorf("expected 200, got %d (body %q)", code, string(tuple[1].(starlark.String)))
			}
			if gotBody != tc.wantBody {
				t.Errorf("request body = %q, want %q", gotBody, tc.wantBody)
			}
			if gotCT != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", gotCT)
			}
		})
	}
}

// TestHttpPostBuiltin_BodyTypeErrors verifies that an unsupported body value is
// rejected with an error that tells the user how to encode a body.
func TestHttpPostBuiltin_BodyTypeErrors(t *testing.T) {
	tests := []struct {
		name    string
		body    starlark.Value
		wantSub string
	}{
		{
			name:    "non-string dict key",
			body:    newStarDict(t, starPair(starlark.MakeInt(1), starlark.String("a"))),
			wantSub: "want string",
		},
		{
			name:    "unsupported set body",
			body:    starlark.NewSet(1),
			wantSub: "unsupported body type set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			thread := makeStarlarkThread("test")
			args := starlark.Tuple{starlark.String("http://invalid.example.invalid:12345/items"), tc.body}

			_, err := httpPostBuiltin(thread, nil, args, nil)
			if err == nil {
				t.Fatal("expected error for unsupported body, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("expected error to contain %q, got: %v", tc.wantSub, err)
			}
			// The message must also teach the user how to encode a body.
			if !strings.Contains(err.Error(), "json(") {
				t.Errorf("expected error to mention the json() encoder, got: %v", err)
			}
			if !strings.Contains(err.Error(), "dict") {
				t.Errorf("expected error to name the accepted dict body type, got: %v", err)
			}
		})
	}
}

// TestHttpPutBuiltin_DictBody verifies http_put gets the same body handling.
func TestHttpPutBuiltin_DictBody(t *testing.T) {
	var gotBody string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})

	thread := makeStarlarkThread("test")
	body := newStarDict(t, starPair(starlark.String("name"), starlark.String("updated")))
	args := starlark.Tuple{starlark.String(srv.URL + "/update"), body}

	if _, err := httpPutBuiltin(thread, nil, args, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `{"name":"updated"}`; gotBody != want {
		t.Errorf("request body = %q, want %q", gotBody, want)
	}
}

// TestJsonBuiltin verifies the json() encoder helper.
func TestJsonBuiltin(t *testing.T) {
	tests := []struct {
		name    string
		value   starlark.Value
		want    string
		wantErr string
	}{
		{
			name:  "dict",
			value: newStarDict(t, starPair(starlark.String("title"), starlark.String("x"))),
			want:  `{"title":"x"}`,
		},
		{
			name:  "string is quoted",
			value: starlark.String("x"),
			want:  `"x"`,
		},
		{
			name:  "int",
			value: starlark.MakeInt(7),
			want:  `7`,
		},
		{
			name:  "bool",
			value: starlark.True,
			want:  `true`,
		},
		{
			name:  "none",
			value: starlark.None,
			want:  `null`,
		},
		{
			name:  "list",
			value: starlark.NewList([]starlark.Value{starlark.String("a"), starlark.MakeInt(1)}),
			want:  `["a",1]`,
		},
		{
			name:    "unsupported set",
			value:   starlark.NewSet(1),
			wantErr: "unsupported body type set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			thread := makeStarlarkThread("test")
			args := starlark.Tuple{tc.value}
			got, err := jsonBuiltin(thread, nil, args, nil)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error to contain %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			s, ok := got.(starlark.String)
			if !ok {
				t.Fatalf("expected String, got %T", got)
			}
			if string(s) != tc.want {
				t.Errorf("json() = %q, want %q", string(s), tc.want)
			}
		})
	}
}

// TestRunDsl_DictBodyEndToEnd verifies the original DF-MUSTER-7 repro: a script
// calling http_post(url, {"title": "x"}) sends a JSON body, and the json()
// helper is available to scripts.
func TestRunDsl_DictBodyEndToEnd(t *testing.T) {
	var gotBodies []string
	var gotCTs []string
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBodies = append(gotBodies, string(b))
		gotCTs = append(gotCTs, r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1}`))
	})

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "dict_body.star")
	scriptContent := "code1, body1 = http_post('" + srv.URL + "/a', {'title': 'x'})\n" +
		"encoded = json({'title': 'x'})\n" +
		"code2, body2 = http_post('" + srv.URL + "/b', encoded)\n" +
		"print(encoded)\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	output := captureStdoutRequest(func() {
		if err := runDsl(nil, []string{"dsl", scriptPath}, false); err != nil {
			t.Fatalf("runDsl with dict body failed: %v", err)
		}
	})

	if len(gotBodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(gotBodies))
	}
	for i, want := range []string{`{"title":"x"}`, `{"title":"x"}`} {
		if gotBodies[i] != want {
			t.Errorf("request %d body = %q, want %q", i+1, gotBodies[i], want)
		}
		if gotCTs[i] != "application/json" {
			t.Errorf("request %d Content-Type = %q, want application/json", i+1, gotCTs[i])
		}
	}
	assertContains(t, output, `{"title":"x"}`)
}

// TestDslCommand_HelpDocumentsBodyTypes verifies the dsl --help surface
// documents the accepted body types and the json() encoder (DF-MUSTER-7 AC2).
func TestDslCommand_HelpDocumentsBodyTypes(t *testing.T) {
	cmd := newDslCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() returned error: %v", err)
	}
	help := buf.String()

	for _, want := range []string{
		"Body types accepted by http_post and http_put",
		"str",
		"dict",
		"list",
		"tuple",
		"int",
		"float",
		"bool",
		"None",
		"json(value) -> string",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("dsl --help missing %q; got:\n%s", want, help)
		}
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got: %s", substr, s)
	}
}
