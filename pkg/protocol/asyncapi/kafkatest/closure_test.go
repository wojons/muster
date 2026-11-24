package kafkatest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStubNeverInProductionClosure enforces the PROTO-021 acceptance rule
// that the in-process broker stub is TEST-ONLY: none of the production
// binaries' import closures may reach this package.
func TestStubNeverInProductionClosure(t *testing.T) {
	// The repo root relative to this package's test cwd.
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	cmd := exec.Command("go", "list", "-deps",
		"./cmd/openapi-cli", "./cmd/mcp-server", "./cmd/daemon")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// Surface stderr for diagnosis.
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps failed: %v: %s", err, string(ee.Stderr))
		}
		t.Fatalf("go list -deps failed: %v", err)
	}
	for _, dep := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(dep, "/pkg/protocol/asyncapi/kafkatest") {
			t.Fatalf("test-only broker stub leaked into a production build closure: %s", dep)
		}
	}
}
