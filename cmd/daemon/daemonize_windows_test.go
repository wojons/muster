//go:build windows

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDaemonProcAttr_Windows_Detached pins the Windows contract of the
// daemon-spawn process attributes: the re-exec'd child must be created
// detached from the parent's console and own a fresh process group, which
// is the Windows equivalent of the Unix setsid(2) session detach pinned by
// daemonize_unix_test.go.
func TestDaemonProcAttr_Windows_Detached(t *testing.T) {
	attr := daemonProcAttr()
	require.NotNil(t, attr, "daemon spawn needs process attributes")
	require.NotZero(t, attr.CreationFlags&detachedProcess,
		"daemon child must not inherit the parent console")
	require.NotZero(t, attr.CreationFlags&createNewProcessGroup,
		"daemon child must own its process group")
}
