package builtin

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/config"
	"github.com/wojons/muster/pkg/errors"
)

// These tests are the integration half of the HTTP/2 work (specs/PROTOCOLS.md
// §6 order 9): they drive the CLI's request handler — config value, client
// options, wire exchange, diagnostics — and assert against the server's own
// observation rather than the client's claim alone.

// versionProbe records what the server saw for every request it served.
type versionProbe struct {
	proto atomic.Value // string
	hits  atomic.Int64
}

func (p *versionProbe) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		p.proto.Store(r.Proto)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"server_proto":%q}`, r.Proto)
	})
}

func (p *versionProbe) lastProto() string {
	v, _ := p.proto.Load().(string)
	return v
}

// startH2Fixture starts an in-process loopback TLS server offering h2.
func startH2Fixture(t *testing.T, probe *versionProbe) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(probe.handler())
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}

// trustDefaultTransportFor points the process-global default transport at one
// that trusts s's certificate. pkg/client's default transport clones the global
// when a client is constructed, so this is how the CLI's internally built
// client is given a trust root. Restored on cleanup.
func trustDefaultTransportFor(t *testing.T, s *httptest.Server) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = s.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// captureStdio runs fn with os.Stdout and os.Stderr redirected to pipes,
// returning what each stream received. It keeps the verbose diagnostics out of
// the test output and makes the stdout/stderr split assertable (stdout payload
// fidelity is frozen by PROTOCOLS.md §4.4/§5.3).
func captureStdio(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	errR, errW, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout, os.Stderr = outW, errW

	runErr := fn()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	os.Stdout, os.Stderr = origOut, origErr

	outBytes, _ := io.ReadAll(outR)
	errBytes, _ := io.ReadAll(errR)
	return string(outBytes), string(errBytes), runErr
}

// TestHandleRequest_HTTPVersionReportsHTTP2 is the CLI-level h2 cell: the
// default policy negotiates HTTP/2 against an h2 peer and the verbose surface
// reports the version that carried the request, from the record, on stderr.
func TestHandleRequest_HTTPVersionReportsHTTP2(t *testing.T) {
	probe := &versionProbe{}
	s := startH2Fixture(t, probe)
	trustDefaultTransportFor(t, s)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "auto"}}
	stdout, stderr, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.NoError(t, err)

	assert.Equal(t, "HTTP/2.0", probe.lastProto(), "the server must have seen HTTP/2.0")
	assert.Contains(t, stderr, "HTTP: HTTP/2.0 (wire, requested auto)",
		"the verbose diagnostic must report the version the wire used")
	assert.NotContains(t, stdout, "HTTP:", "diagnostics belong on stderr, never in the payload")
	assert.Contains(t, stdout, `"server_proto":"HTTP/2.0"`, "stdout carries the response body")
}

// TestHandleRequest_HTTPVersionReportsHTTP11Only is the reporting control: a
// peer offering only HTTP/1.1 still serves the request under the default
// policy, and the version reported comes from the wire.
func TestHandleRequest_HTTPVersionReportsHTTP11Only(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	stdout, stderr, err := captureStdio(t, func() error {
		return handleRequest(nil, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.NoError(t, err)

	assert.Equal(t, "HTTP/1.1", probe.lastProto())
	assert.Contains(t, stderr, "HTTP: HTTP/1.1 (wire, requested auto)")
	assert.Contains(t, stdout, `"server_proto":"HTTP/1.1"`)
}

// TestHandleRequest_HTTPVersionRequiredPolicyFails covers the policy cell: a
// required HTTP/2 that the endpoint cannot provide is a coded failure naming
// requested, observed and reason — not a downgrade.
func TestHandleRequest_HTTPVersionRequiredPolicyFails(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "2"}}
	stdout, _, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})
	require.Error(t, err)

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeProtocolMismatch, appErr.Code())
	assert.Equal(t, "2", appErr.Details()["requested"])
	assert.Equal(t, "HTTP/1.1", appErr.Details()["observed"])
	assert.Equal(t, "plaintext-endpoint", appErr.Details()["reason"])
	assert.Empty(t, stdout, "a failed request must not print a payload")
}

// TestHandleRequest_HTTPVersionUnknownValueRejected: the policy value is
// validated before anything is sent.
func TestHandleRequest_HTTPVersionUnknownValueRejected(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "h2"}}
	_, _, err := captureStdio(t, func() error {
		return handleRequest(cfg, "GET", s.URL+"/x", nil, nil, "", true, 5*time.Second, "", "", "")
	})

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.CodeValidation, appErr.Code())
	assert.Equal(t, int64(0), probe.hits.Load(), "no request may be sent for an invalid policy")
}

// TestRequestCommand_HTTPVersionFlagOverridesConfig drives the real cobra
// command: the flag must win over the configured policy in both directions.
func TestRequestCommand_HTTPVersionFlagOverridesConfig(t *testing.T) {
	probe := &versionProbe{}
	s := httptest.NewServer(probe.handler())
	t.Cleanup(s.Close)

	// Config requires HTTP/2, the flag asks for auto: the flag wins and the
	// request succeeds over the plaintext endpoint.
	cfg := &config.Config{Request: config.RequestConfig{HTTPVersion: "2"}}
	cmd := newRequestCommand(cfg)
	cmd.SetArgs([]string{"GET", s.URL + "/x", "--http-version", "auto", "-v"})
	_, stderr, err := captureStdio(t, func() error { return cmd.ExecuteContext(t.Context()) })
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.Request.HTTPVersion, "the flag must override the configured policy")
	assert.Contains(t, stderr, "HTTP: HTTP/1.1 (wire, requested auto)")

	// The other direction: auto configured, HTTP/2 required by the flag, and
	// the cleartext endpoint cannot satisfy it.
	cfg2 := &config.Config{Request: config.RequestConfig{HTTPVersion: "auto"}}
	cmd2 := newRequestCommand(cfg2)
	cmd2.SetArgs([]string{"GET", s.URL + "/x", "--http-version", "2", "-v"})
	_, _, err = captureStdio(t, func() error { return cmd2.ExecuteContext(t.Context()) })
	require.Error(t, err)
	assert.Equal(t, "2", cfg2.Request.HTTPVersion)
	assert.True(t, errors.IsError(err, errors.CodeProtocolMismatch))
	assert.Contains(t, err.Error(), "requested 2, observed HTTP/1.1 (plaintext-endpoint)")
}

// TestRequestCommandHasHTTPVersionFlag pins the flag's existence and default:
// an empty value means "use the configured policy".
func TestRequestCommandHasHTTPVersionFlag(t *testing.T) {
	cmd := newRequestCommand(&config.Config{})
	flag := cmd.Flags().Lookup("http-version")
	require.NotNil(t, flag, "the request verb must expose --http-version")
	assert.Equal(t, "", flag.DefValue)
}
