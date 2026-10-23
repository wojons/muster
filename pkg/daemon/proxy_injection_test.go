package daemon

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// MUSTER-001: a daemon fronting an authenticated API can hold the upstream
// credential itself, so socket clients need neither the key nor the base URL.
// The injection must be authoritative: a caller-supplied value is replaced,
// not appended, so a client cannot downgrade or shadow the daemon credential.
func TestProxyHandlerWithAuth_InjectsAuthoritativeCredential(t *testing.T) {
	var gotKey, gotMarker string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		gotMarker = r.Header.Get("X-Client-Marker")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	handler := NewProxyHandlerWithAuth(upstream.URL, "X-API-Key", "daemon-held-secret")
	req := httptest.NewRequest(http.MethodPost, "/api/memories", strings.NewReader(`{"key":"k"}`))
	req.Header.Set("X-API-Key", "caller-supplied")
	req.Header.Set("X-Client-Marker", "kept")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "daemon-held-secret", gotKey, "daemon credential must replace the caller's value")
	require.Equal(t, "kept", gotMarker, "unrelated client headers still pass through")
}

// Without injection the proxy behaves exactly as before: the caller's own
// credential reaches the upstream untouched.
func TestProxyHandler_NoInjectionPreservesClientCredential(t *testing.T) {
	var gotKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	handler := NewProxyHandler(upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/memories", nil)
	req.Header.Set("X-API-Key", "caller-supplied")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "caller-supplied", gotKey)
}

// A daemon that injects an upstream credential must gate its own socket:
// without a client token the request is refused BEFORE it reaches the upstream,
// so an unauthenticated caller can never write through as the daemon's identity.
func TestController_SocketRequiresClientTokenWhenInjecting(t *testing.T) {
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	dir := socketTestDir(t)
	sockPath := filepath.Join(dir, "d.sock")
	handler := NewProxyHandlerWithAuth(upstream.URL, "X-API-Key", "daemon-held-secret")
	controller, err := NewController(ControllerConfig{
		PidFilePath: filepath.Join(dir, "d.pid"),
		SocketPath:  sockPath,
		Version:     "test",
		Handler:     handler,
		AuthTokens:  []string{"client-token"},
	})
	require.NoError(t, err)
	require.NoError(t, controller.StartWithSocket(handler))
	t.Cleanup(func() { _ = controller.Shutdown(context.Background()) })

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		},
	}}

	// No token → 401 from the daemon, upstream untouched.
	resp, err := client.Post("http://unix/api/memories", "application/json", strings.NewReader(`{"key":"k"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, int32(0), atomic.LoadInt32(&hits), "unauthenticated request must not reach the upstream")

	// With the client token → proxied.
	req, err := http.NewRequest(http.MethodPost, "http://unix/api/memories", strings.NewReader(`{"key":"k"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer client-token")
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, int32(1), atomic.LoadInt32(&hits))
}

// Concurrent clients share one socket and one injected credential: the daemon
// is a multiplexer, not a per-client tunnel.
func TestController_SocketServesConcurrentClients(t *testing.T) {
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "daemon-held-secret" {
			t.Errorf("upstream X-API-Key = %q", got)
		}
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	dir := socketTestDir(t)
	sockPath := filepath.Join(dir, "d.sock")
	handler := NewProxyHandlerWithAuth(upstream.URL, "X-API-Key", "daemon-held-secret")
	controller, err := NewController(ControllerConfig{
		PidFilePath: filepath.Join(dir, "d.pid"),
		SocketPath:  sockPath,
		Version:     "test",
		Handler:     handler,
		AuthTokens:  []string{"client-token"},
	})
	require.NoError(t, err)
	require.NoError(t, controller.StartWithSocket(handler))
	t.Cleanup(func() { _ = controller.Shutdown(context.Background()) })

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		},
		MaxIdleConnsPerHost: 8,
	}}

	const clients = 6
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		go func() {
			req, reqErr := http.NewRequest(http.MethodGet, "http://unix/api/memories/key/k", nil)
			if reqErr != nil {
				errs <- reqErr
				return
			}
			req.Header.Set("Authorization", "Bearer client-token")
			resp, doErr := client.Do(req)
			if doErr != nil {
				errs <- doErr
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				errs <- &statusError{code: resp.StatusCode}
				return
			}
			errs <- nil
		}()
	}
	for i := 0; i < clients; i++ {
		require.NoError(t, <-errs)
	}
	require.Equal(t, int32(clients), atomic.LoadInt32(&hits))
}

type statusError struct{ code int }

func (e *statusError) Error() string { return http.StatusText(e.code) }
