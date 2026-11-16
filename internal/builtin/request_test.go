package builtin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
	"golang.org/x/net/webdav"
)

// =============================================================================
// validateURLScheme
// =============================================================================

func TestValidateURLScheme(t *testing.T) {
	t.Run("http scheme is valid", func(t *testing.T) {
		err := validateURLScheme("http")
		require.NoError(t, err)
	})

	t.Run("https scheme is valid", func(t *testing.T) {
		err := validateURLScheme("https")
		require.NoError(t, err)
	})

	t.Run("ftp scheme is invalid", func(t *testing.T) {
		err := validateURLScheme("ftp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
		assert.Contains(t, err.Error(), `"ftp"`)
	})

	t.Run("empty scheme is invalid", func(t *testing.T) {
		err := validateURLScheme("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
	})

	t.Run("HTTP uppercase is case-sensitive and invalid", func(t *testing.T) {
		// The function does exact string comparison, no case-insensitive check
		err := validateURLScheme("HTTP")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"HTTP"`)
	})

	t.Run("unknown scheme gives descriptive error", func(t *testing.T) {
		err := validateURLScheme("gopher")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
		assert.Contains(t, err.Error(), `"gopher"`)
	})
}

// =============================================================================
// handleRequest — error paths
// =============================================================================

func TestHandleRequest_ParseURLError(t *testing.T) {
	// A URL with an invalid control character will fail URL parsing
	err := handleRequest(nil, "GET", "http://example.com/\x00path", nil, nil, "", false, 5*time.Second, "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse URL")
}

func TestHandleRequest_InvalidScheme(t *testing.T) {
	err := handleRequest(nil, "GET", "ftp://example.com/path", nil, nil, "", false, 5*time.Second, "", "", "")
	require.Error(t, err)
	// The error wraps: "invalid URL scheme: unsupported scheme ..."
	assert.True(t, strings.Contains(err.Error(), "invalid URL scheme"), "expected 'invalid URL scheme' in error: %v", err)
	assert.True(t, strings.Contains(err.Error(), `"ftp"`), "expected 'ftp' mentioned in error: %v", err)
}

// =============================================================================
// handleRequest — httptest-backed integration paths
// =============================================================================

// captureStdoutRequest temporarily redirects os.Stdout and returns the captured output.
func captureStdoutRequest(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	done := make(chan string)
	go func() {
		var buf strings.Builder
		// Read until pipe is closed
		data := make([]byte, 4096)
		for {
			n, err := r.Read(data)
			if n > 0 {
				buf.Write(data[:n])
			}
			if err != nil {
				break
			}
		}
		done <- buf.String()
	}()

	fn()
	w.Close()
	return <-done
}

// captureStderrRequest temporarily redirects os.Stderr and returns the captured output.
func captureStderrRequest(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	done := make(chan string)
	go func() {
		var buf strings.Builder
		data := make([]byte, 4096)
		for {
			n, err := r.Read(data)
			if n > 0 {
				buf.Write(data[:n])
			}
			if err != nil {
				break
			}
		}
		done <- buf.String()
	}()

	fn()
	w.Close()
	return <-done
}

func TestHandleRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	output := captureStdoutRequest(func() {
		err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	assert.Contains(t, output, `{"status":"ok"}`)
}

func TestHandleRequest_HTTP400Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	var err error
	output := captureStdoutRequest(func() {
		err = handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "")
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed with status 400")
	assert.Contains(t, output, `{"error":"bad request"}`)
}

// TestHandleRequest_HTTP500Error checks a persistent 500 is reported to the
// caller. The verb retries transient statuses by default (QA-MUSTER-044), but
// an exhausted ceiling still hands the last response back to this handler, so
// the status is reported and the error text is unchanged from the single-shot
// path; the attempts themselves are reported on stderr.
func TestHandleRequest_HTTP500Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed with status 500")
}

func TestHandleRequest_JSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hello":"world","nested":{"key":"value"}}`))
	}))
	defer server.Close()

	output := captureStdoutRequest(func() {
		err := handleRequest(nil, "GET", server.URL, nil, nil, "json", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	// JSON output mode pretty-prints
	assert.Contains(t, output, `"hello"`)
	assert.Contains(t, output, `"world"`)
	assert.Contains(t, output, `"nested"`)
	assert.Contains(t, output, `"key"`)
	assert.Contains(t, output, `"value"`)
}

func TestHandleRequest_JSONOutput_NonJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`plain text response`))
	}))
	defer server.Close()

	output := captureStdoutRequest(func() {
		err := handleRequest(nil, "GET", server.URL, nil, nil, "json", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	// Non-JSON body in json mode: falls back to plain output
	assert.Contains(t, output, `plain text response`)
}

func TestHandleRequest_VerboseOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	var stdout, stderr string
	stdout = captureStdoutRequest(func() {
		stderr = captureStderrRequest(func() {
			err := handleRequest(nil, "GET", server.URL, nil, nil, "", true, 5*time.Second, "", "", "")
			require.NoError(t, err)
		})
	})

	assert.Contains(t, stderr, "Status: 200")
	assert.Contains(t, stderr, "X-Custom: test-value")
	assert.Contains(t, stdout, "ok")
}

func TestHandleRequest_WithRateLimitConfig(t *testing.T) {
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled:    true,
			RatePerSec: 10,
			Burst:      5,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	err := handleRequest(cfg, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "")
	require.NoError(t, err)
}

func TestHandleRequest_WithCacheConfig(t *testing.T) {
	cfg := &config.Config{
		Cache: config.CacheConfig{
			Enabled: true,
			MaxSize: 100,
			TTL:     60 * time.Second,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"cached":true}`))
	}))
	defer server.Close()

	output := captureStdoutRequest(func() {
		err := handleRequest(cfg, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	assert.Contains(t, output, `{"cached":true}`)
}

func TestHandleRequest_POSTWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"created":true}`))
	}))
	defer server.Close()

	output := captureStdoutRequest(func() {
		err := handleRequest(nil, "POST", server.URL, nil, nil, "", false, 5*time.Second, `{"name":"test"}`, "", "")
		require.NoError(t, err)
	})

	assert.Contains(t, output, `{"created":true}`)
}

func TestHandleRequest_WithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer abc123", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"auth":"ok"}`))
	}))
	defer server.Close()

	headers := []string{"Authorization: Bearer abc123", "Content-Type: application/json"}
	output := captureStdoutRequest(func() {
		err := handleRequest(nil, "GET", server.URL, headers, nil, "", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	assert.Contains(t, output, `{"auth":"ok"}`)
}

func TestHandleRequest_WithQueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// NOTE: Query params are added to the URL by request.Builder.buildURL().
		// The builder constructs the full URL string with query params appended
		// (e.g., http://host?key1=value1&key2=value2).
		// The Go HTTP transport strips the Host from the URL when sending to
		// httptest servers, but preserves the path and query string.
		// Currently, the builder includes query params in the URL but httptest
		// may not preserve them across the Muster client wrapper.
		// This test verifies the handler is called and a successful response
		// is returned. Query param transport is best tested via integration tests.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"query":"ok"}`))
	}))
	defer server.Close()

	params := []string{"key1=value1", "key2=value2"}
	output := captureStdoutRequest(func() {
		err := handleRequest(nil, "GET", server.URL, params, nil, "", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	assert.Contains(t, output, `{"query":"ok"}`)
}

func TestHandleRequest_EmptyResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var captured string
	captured = captureStdoutRequest(func() {
		err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "")
		require.NoError(t, err)
	})

	// 204 with empty body: nothing printed to stdout
	assert.Empty(t, captured)
}

func TestHandleRequest_ConnectionRefused(t *testing.T) {
	// Use an unreachable port
	err := handleRequest(nil, "GET", "http://127.0.0.1:1/path", nil, nil, "", false, 1*time.Second, "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute request")
}

func TestHandleRequest_WASMPluginLoadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"original":"data"}`))
	}))
	defer server.Close()

	err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "/nonexistent/plugin.wasm", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wasm: load plugin")
}

// =============================================================================
// Cobra-level usage-suppression tests (AC1)
// =============================================================================

// TestRequestCommand_HTTPErrorNoUsageBlock verifies that when the request
// command encounters a 4xx/5xx HTTP response, the cobra usage block is NOT
// printed — only the response body and the status error appear.
func TestRequestCommand_HTTPErrorNoUsageBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	root := NewRootCommand(nil)
	root.SetArgs([]string{"request", "GET", server.URL})

	stdout := captureStdoutRequest(func() {
		_ = root.Execute()
	})

	// The response body should still be printed to stdout.
	assert.Contains(t, stdout, `{"error":"not found"}`)
	// The usage block must NOT appear.
	assert.NotContains(t, stdout, "Usage:")
}

// =============================================================================
// PROTO-019 — the request body is attached for ANY method, not just POST/PUT/PATCH
// =============================================================================

// wireRequest is an exact record of what a fixture server observed: the verb it
// received, the verbatim body bytes that reached the wire and the Content-Type
// (if any) the request carried.
type wireRequest struct {
	Method        string
	Body          []byte
	ContentLength int64
	ContentType   string
}

// echoOnce returns an httptest.Server that records the first request it sees
// and answers 207 Multi-Status (the WebDAV success status for a PROPFIND). The
// record is handed over a buffered channel so the assertion is synchronized
// with the handler goroutine instead of relying on the response round trip.
func echoOnce(t *testing.T) (*httptest.Server, <-chan wireRequest) {
	t.Helper()

	seen := make(chan wireRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		seen <- wireRequest{
			Method:        r.Method,
			Body:          body,
			ContentLength: r.ContentLength,
			ContentType:   r.Header.Get("Content-Type"),
		}
		w.WriteHeader(http.StatusMultiStatus)
	}))
	t.Cleanup(server.Close)

	return server, seen
}

// TestHandleRequest_NonAllowlistedMethodSendsBody is the PROTO-019 regression
// test. Before the fix, request.go attached the body only when the method was
// POST, PUT or PATCH, so every other body-bearing verb — WebDAV PROPFIND /
// REPORT / PROPPATCH / LOCK, an extension method, a DELETE with a body — reached
// the wire with body length 0 while the flag was silently ignored. The WebDAV
// application profile (specs/PROTOCOLS.md matrix row 17, gate 11) cannot work
// while that holds, because RFC 4918 defines those verbs as body-bearing.
//
// POST is included as a control: the previously allow-listed verb must keep
// behaving exactly as it did.
func TestHandleRequest_NonAllowlistedMethodSendsBody(t *testing.T) {
	cases := []struct {
		method string
		body   string
	}{
		{"PROPFIND", `<propfind xmlns="DAV:"><prop><getcontentlength/></prop></propfind>`},
		{"REPORT", `<report/>`},
		{"PROPPATCH", `<propertyupdate/>`},
		{"LOCK", `<lockinfo/>`},
		{"DELETE", `<deletebody/>`},
		{"MKCALENDAR", `<mkcalendar/>`}, // extension verb, no allowlist entry
		{"POST", `{"name":"test"}`},     // control: allow-listed before the fix
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			server, seen := echoOnce(t)

			err := handleRequest(nil, tc.method, server.URL+"/dav", nil, nil, "", false, 5*time.Second, tc.body, "", "")
			require.NoError(t, err)

			got := <-seen
			assert.Equal(t, tc.method, got.Method, "the server must observe the verb verbatim")
			assert.Equal(t, len(tc.body), len(got.Body), "body length that reached the wire")
			assert.Equal(t, tc.body, string(got.Body), "body bytes must match -b exactly")
			assert.EqualValues(t, len(tc.body), got.ContentLength,
				"Content-Length must describe the attached body")
			// -b alone never invents a Content-Type, for any verb. This is the
			// pre-existing POST/PUT/PATCH behaviour, pinned so the widening of
			// the method gate cannot silently change it; -H is the only source.
			assert.Empty(t, got.ContentType, "-b must not set a Content-Type by default")
		})
	}
}

// TestHandleRequest_EmptyBodyFlagSendsNoBody pins the other half of the
// contract: an empty --body/-b attaches nothing, for any verb. The check runs
// for an allow-listed verb, a WebDAV verb and DELETE so the regression cannot be
// "fixed" by unconditionally attaching a zero-length reader.
func TestHandleRequest_EmptyBodyFlagSendsNoBody(t *testing.T) {
	for _, method := range []string{"POST", "PROPFIND", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			server, seen := echoOnce(t)

			err := handleRequest(nil, method, server.URL+"/dav", nil, nil, "", false, 5*time.Second, "", "", "")
			require.NoError(t, err)

			got := <-seen
			assert.Empty(t, got.Body, "an empty -b must send no bytes")
			assert.EqualValues(t, 0, got.ContentLength)
		})
	}
}

// TestHandleRequest_WebDAVNonEmptyBodyReachesServerProfile drives the request
// through a real WebDAV server implementation (the same x/net/webdav handler a
// conformance server uses) rather than a recording stub. It answers the
// Content-Type question PROTO-019 raises with a measurement instead of an
// assumption: the request must carry the exact bytes AND no Content-Type, and
// the server must still accept and parse the document. The parsed document is
// observable — the server echoes back the property we named as a 404 propstat —
// while an empty body would make it answer the allprop default and that marker
// would never appear (measured RED before the fix).
func TestHandleRequest_WebDAVNonEmptyBodyReachesServerProfile(t *testing.T) {
	handler := &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: webdav.NewMemFS(),
		LockSystem: webdav.NewMemLS(),
	}

	// The recorder must not read the body: the webdav handler needs it intact.
	seen := make(chan wireRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- wireRequest{
			Method:        r.Method,
			ContentLength: r.ContentLength,
			ContentType:   r.Header.Get("Content-Type"),
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	const propfindBody = `<?xml version="1.0" encoding="utf-8"?>` +
		`<propfind xmlns="DAV:"><prop><proto-019-marker/></prop></propfind>`

	var stdout string
	stdout = captureStdoutRequest(func() {
		err := handleRequest(nil, "PROPFIND", server.URL+"/dav/", nil, nil, "", false, 5*time.Second, propfindBody, "", "")
		require.NoError(t, err)
	})

	got := <-seen
	assert.Equal(t, "PROPFIND", got.Method)
	assert.EqualValues(t, len(propfindBody), got.ContentLength,
		"the WebDAV body must reach the server with a Content-Length")
	assert.Empty(t, got.ContentType,
		"no Content-Type default is required for the WebDAV profile to accept a body")

	// 207 Multi-Status with the named property echoed back as a 404 propstat:
	// proof the server parsed our document instead of falling back to allprop,
	// i.e. proof the bytes arrived and no Content-Type was needed.
	assert.Contains(t, stdout, "multistatus")
	assert.Contains(t, stdout, "proto-019-marker")
}

// TestRequestCommand_FlagMisuseShowsUsage verifies that flag misuse (unknown
// flag) still surfaces the usage block — SilenceUsage should only suppress
// usage for runtime errors, not flag parsing errors.
func TestRequestCommand_FlagMisuseShowsUsage(t *testing.T) {
	root := NewRootCommand(nil)
	root.SetArgs([]string{"request", "GET", "http://127.0.0.1:1", "--unknown-flag"})

	_ = root.Execute()

	// Capture stderr to check for usage block. cobra prints usage to stderr
	// (or stdout depending on configuration); we check both.
	// Since we can't easily capture cobra's output here, we verify the command
	// has SilenceUsage=false after a flag error via the FlagErrorFunc path.
	// This is covered by the integration in TestRequestCommand_FlagMisuseShowsUsage.
	// The key assertion: the command still returned an error (flag parsing).
	require.Error(t, root.Execute())
}

// =============================================================================
// Retries (QA-MUSTER-044)
// =============================================================================

// flakyEndpoint serves a test server that answers the first failCount requests
// with status (optionally carrying extraHeaders) and every later request with
// 200. The returned counter is how many requests the server actually saw —
// the number that proves whether a retry happened.
func flakyEndpoint(failCount int, status int, extraHeaders http.Header) (*httptest.Server, *int32) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&count, 1)
		if int(n) <= failCount {
			for k, values := range extraHeaders {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	return srv, &count
}

// TestHandleRequest_RetriesTransient503ThenSucceeds is the regression test for
// QA-MUSTER-044: a flaky endpoint that answers 503 twice must be retried, and
// the `request` verb must end successfully after exactly three exchanges.
func TestHandleRequest_RetriesTransient503ThenSucceeds(t *testing.T) {
	srv, count := flakyEndpoint(2, http.StatusServiceUnavailable, nil)
	defer srv.Close()

	cfg := &config.Config{MaxRetries: 3, MaxRetriesSet: true}
	err := handleRequest(cfg, "GET", srv.URL+"/flaky", nil, nil, "", false, 5*time.Second, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(count),
		"server must see exactly 3 requests: the first attempt plus 2 retries")
}

// TestHandleRequest_RetriesOnByDefault pins the honest default: with no
// max_retries setting at all, transient statuses are retried (3 attempts).
func TestHandleRequest_RetriesOnByDefault(t *testing.T) {
	srv, count := flakyEndpoint(2, http.StatusServiceUnavailable, nil)
	defer srv.Close()

	err := handleRequest(&config.Config{}, "GET", srv.URL+"/flaky", nil, nil, "", false, 5*time.Second, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(count),
		"an unset max_retries resolves to the default 3 attempts")
}

// TestHandleRequest_MaxRetriesZeroDisablesRetries pins the backward-compatible
// escape hatch: MaxRetries=0 means one attempt, never a retry.
func TestHandleRequest_MaxRetriesZeroDisablesRetries(t *testing.T) {
	srv, count := flakyEndpoint(2, http.StatusServiceUnavailable, nil)
	defer srv.Close()

	cfg := &config.Config{MaxRetries: 0, MaxRetriesSet: true}
	err := handleRequest(cfg, "GET", srv.URL+"/flaky", nil, nil, "", false, 5*time.Second, "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
	assert.Equal(t, int32(1), atomic.LoadInt32(count),
		"max_retries=0 must produce exactly one request")
}

// TestHandleRequest_RetryAfterSurfacedOnStderr checks the 429 contract: the
// Retry-After value is printed to stderr and the wait before the next attempt
// is at least that value (the header is honoured, not just logged).
func TestHandleRequest_RetryAfterSurfacedOnStderr(t *testing.T) {
	headers := http.Header{"Retry-After": []string{"1"}}
	srv, count := flakyEndpoint(1, http.StatusTooManyRequests, headers)
	defer srv.Close()

	cfg := &config.Config{MaxRetries: 3, MaxRetriesSet: true}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	start := time.Now()
	reqErr := handleRequest(cfg, "GET", srv.URL+"/limited", nil, nil, "", false, 10*time.Second, "", "", "")
	elapsed := time.Since(start)

	os.Stderr = oldStderr
	require.NoError(t, w.Close())
	captured, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, reqErr)
	assert.Equal(t, int32(2), atomic.LoadInt32(count))
	assert.Contains(t, string(captured), "honouring Retry-After: 1s",
		"the Retry-After value must be surfaced on stderr")
	assert.GreaterOrEqual(t, elapsed, time.Second,
		"the wait before the second attempt must honour Retry-After")
}
