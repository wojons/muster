//go:build !windows

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDaemonProcAttr_Unix_NewSession pins the Unix contract of the
// daemon-spawn process attributes: the re-exec'd child must start a new
// session so it outlives the invoking shell. The Windows twin is pinned by
// daemonize_windows_test.go, which cannot run on this host.
func TestDaemonProcAttr_Unix_NewSession(t *testing.T) {
	attr := daemonProcAttr()
	require.NotNil(t, attr, "daemon spawn needs process attributes")
	require.True(t, attr.Setsid, "daemon child must start a new session (setsid)")
}
