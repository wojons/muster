//go:build linux && !seccomp
// +build linux,!seccomp

package platform

import "testing"

// seccompFilterLoaded tracks whether a seccomp filter has been loaded.
// In the stub build, filters can never be loaded.
var seccompFilterLoaded bool

// seccompAvailable skips the test because seccomp is not compiled in.
func seccompAvailable(t *testing.T) {
	t.Helper()
	t.Skip("seccomp not compiled in (build with -tags seccomp to enable)")
}
