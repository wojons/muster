package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureHelp runs the daemon with --help and returns the usage text the flag
// package printed to stderr.
func captureHelp(t *testing.T) string {
	t.Helper()
	oldArgs, oldStderr := os.Args, os.Stderr
	t.Cleanup(func() {
		os.Args, os.Stderr = oldArgs, oldStderr
	})

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	os.Args = []string{"openapi-daemon", "--help"}

	runErr := run()

	require.NoError(t, w.Close())
	os.Stderr = oldStderr

	buf := make([]byte, 16384)
	n, _ := r.Read(buf)
	_ = r.Close()
	require.NoError(t, runErr, "--help must exit 0")
	return string(buf[:n])
}

// TestRun_HelpDocumentsMetricsAddr checks the new scrape target is discoverable
// from --help: an operator who cannot find the flag cannot configure Prometheus.
func TestRun_HelpDocumentsMetricsAddr(t *testing.T) {
	output := captureHelp(t)

	require.Contains(t, output, "-metrics-addr", "the help output must document the metrics flag")
	require.Contains(t, output, "127.0.0.1:9110", "the help output must show the loopback default")
}

// TestRun_RejectsNonLoopbackMetricsAddr checks spec §4.2 is enforced up front:
// the one network-reachable surface the daemon gains must not be bound to a
// routable address by a typo.
func TestRun_RejectsNonLoopbackMetricsAddr(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	for _, addr := range []string{"0.0.0.0:9110", "8.8.8.8:9110"} {
		os.Args = []string{"openapi-daemon", "status", "--metrics-addr", addr}
		err := run()
		require.Error(t, err, "metrics addr %q must be refused", addr)
		require.Contains(t, err.Error(), "loopback")
	}
}

// TestRun_AcceptsLoopbackMetricsAddr checks a valid override is not rejected
// before any listener is bound (the status action never binds one).
func TestRun_AcceptsLoopbackMetricsAddr(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	os.Args = []string{"openapi-daemon", "status", "--metrics-addr", "127.0.0.1:19110"}
	require.NoError(t, run())
}

// TestMetricsTenantID checks the tenant_id label value: the configured value
// when set, the documented default otherwise.
func TestMetricsTenantID(t *testing.T) {
	t.Setenv("MUSTER_TENANT_ID", "")
	require.Equal(t, "default", metricsTenantID())

	t.Setenv("MUSTER_TENANT_ID", "  acme  ")
	require.Equal(t, "acme", metricsTenantID(), "surrounding whitespace is not part of a tenant id")
}
