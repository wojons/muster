package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/daemon"
)

// unixSocketClient returns an http.Client that dials the given Unix socket
// directly, matching the pattern used in pkg/daemon/proxy_test.go.
func unixSocketClient(t *testing.T, socketPath string) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}
}

// getViaSocket performs a GET over a unix-socket client and returns the
// status code and body.
func getViaSocket(t *testing.T, client *http.Client, path string) (int, string) {
	t.Helper()
	resp, err := client.Get("http://unix" + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// startTestController starts a controller on a fresh temp socket with the
// given handler and returns it together with the socket path. Cleanup
// (shutdown + socket removal) is registered automatically.
func startTestController(t *testing.T, handler http.Handler) (*daemon.Controller, string) {
	t.Helper()
	socketPath := filepath.Join(socketTestDir(t), "start.sock")
	pidFile := filepath.Join(socketTestDir(t), "start.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)
	require.NoError(t, controller.StartWithSocket(handler))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = controller.Shutdown(ctx)
	})
	return controller, socketPath
}

// TestStartModeHandler_WithBackendURL_ServesProxiedData is the DF-009
// regression test: `start` with --backend-url must serve REAL backend data
// through its Unix socket, not the canned "Muster daemon is running"
// message. handleStart used to ignore the proxy handler and always serve
// createDaemonHandler().
func TestStartModeHandler_WithBackendURL_ServesProxiedData(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":1,"name":"probe"}]}`))
		default:
			// Distinguish backend responses from daemon-local ones.
			_, _ = w.Write([]byte(`{"from":"backend"}`))
		}
	}))
	defer backend.Close()

	// Same handler run() builds from --backend-url and now passes to
	// handleStart, which serves it via startModeHandler.
	handler := startModeHandler(daemon.NewProxyHandler(backend.URL))

	_, socketPath := startTestController(t, handler)
	client := unixSocketClient(t, socketPath)

	// /projects must return REAL backend data, not the canned message.
	code, body := getViaSocket(t, client, "/projects")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, `{"projects":[{"id":1,"name":"probe"}]}`, body)
	require.NotContains(t, body, "Muster daemon is running")

	// /health must still be served by the daemon itself (startModeHandler
	// composes the local /health endpoints around the proxy).
	code, body = getViaSocket(t, client, "/health")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, `{"status":"healthy"}`, body)

	// Unknown paths proxy through to the backend (never canned).
	code, body = getViaSocket(t, client, "/other")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, `{"from":"backend"}`, body)
}

// TestStartModeHandler_NoBackendURL_ServesCannedHandler verifies that
// `start` without --backend-url keeps serving the canned daemon handler
// (root info + /health endpoints) — the previous behavior must not regress.
func TestStartModeHandler_NoBackendURL_ServesCannedHandler(t *testing.T) {
	_, socketPath := startTestController(t, startModeHandler(nil))
	client := unixSocketClient(t, socketPath)

	// Root catch-all serves the canned message.
	code, body := getViaSocket(t, client, "/projects")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "Muster daemon is running")

	// Health endpoints still respond.
	code, body = getViaSocket(t, client, "/health")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, `{"status":"healthy"}`, body)

	code, body = getViaSocket(t, client, "/health/live")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `"status":"alive"`)
}
