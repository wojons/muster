package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/monitoring"
	"github.com/wojons/muster/pkg/daemon"
)

func TestDaemonRequest_EmitsAuditEventAndMetric(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	socketDir := shortSocketTempDir(t)
	socketPath := filepath.Join(socketDir, "daemon.sock")
	recorder := monitoring.NewMetricRecorder("daemon-test", nil)
	hook := audit.NewHookWithMetrics(audit.DefaultConfigEnabled(), recorder)
	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
		AuditHook:  hook,
	})
	require.NoError(t, err)
	require.NoError(t, controller.StartWithSocket(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = controller.Shutdown(ctx)
		_ = hook.Close()
	})

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://unix/request", nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	require.NoError(t, hook.Close())
	require.Equal(t, float64(1), testutil.ToFloat64(recorder.Muster().AuditEventsWritten.WithLabelValues(audit.EventTypeCommand, string(audit.AuditCategorySystem))))

	raw, err := os.ReadFile(sink)
	require.NoError(t, err)
	var event audit.AuditEvent
	require.NoError(t, json.Unmarshal(raw[:len(raw)-1], &event))
	require.Equal(t, "command.daemon.request", event.Action)
}
