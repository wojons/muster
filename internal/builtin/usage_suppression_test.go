package builtin

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// REVIEW-MUSTER-32: on a RUNTIME failure the hand-written command surfaces
// print ONLY the error line — the same behavior generated commands have had
// since DF-019 (request.go). The five surfaces here previously buried the
// error under cobra's twelve-line Usage/Flags block.
//
// The capture mirrors production stderr wiring: cobra sends the usage block to
// OutOrStderr (falls back to the error writer when no out writer is set) and
// the root command sets SilenceErrors, so cmd/openapi-cli/main.go prints the
// "Error: ..." line itself — the test helper reproduces exactly that print so
// the buffer is byte-equivalent to what an operator sees on stderr.

// runtimeErrorNoUsage runs one failing invocation against the real root
// command and asserts the captured output contains the Error line and no
// Usage/Flags block.
//
// Both root writers point at one buffer: cobra v1.10 prints the usage block
// through OutOrStderr (which falls back to os.Stderr, ignoring the error
// writer), so capturing only the error writer would let a usage block leak
// past the assertion — exactly the production defect. The buffer is therefore
// byte-equivalent to what an operator sees on stderr.
func runtimeErrorNoUsage(t *testing.T, args []string, wantInError string) {
	t.Helper()

	root := NewRootCommand(nil)
	root.SetArgs(args)
	stderr := &bytes.Buffer{}
	root.SetOut(stderr)
	root.SetErr(stderr)

	err := root.Execute()
	require.Error(t, err)
	// Mirror cmd/openapi-cli/main.go's single error print (root sets
	// SilenceErrors, so cobra itself prints the error nowhere).
	fmt.Fprintf(stderr, "Error: %v\n", err)

	assert.Contains(t, stderr.String(), "Error: ")
	assert.Contains(t, stderr.String(), wantInError)
	assert.NotContains(t, stderr.String(), "Usage:")
	assert.NotContains(t, stderr.String(), "Flags:")
}

// TestRuntimeFailureSuppressesUsageForAllFiveSurfaces covers the REVIEW-
// MUSTER-32 acceptance: grpc, protocol graphql, protocol jsonrpc, protocol
// soap and inventory all report a failing invocation as the error line only.
func TestRuntimeFailureSuppressesUsageForAllFiveSurfaces(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			// No --target and a non-reflection source: the call fails in
			// RunE before any dial happens.
			name: "grpc call",
			args: []string{"grpc", "call", "kv.KeyValue-Get", "--source", "kv.proto"},
			want: "no invocation target",
		},
		{
			name: "protocol graphql",
			args: []string{"protocol", "graphql"},
			want: "--schema is required",
		},
		{
			name: "protocol jsonrpc",
			args: []string{"protocol", "jsonrpc", "--endpoint", "http://127.0.0.1:1"},
			want: "--method is required when --doc is omitted",
		},
		{
			name: "protocol soap",
			args: []string{"protocol", "soap"},
			want: "--wsdl is required",
		},
		{
			name: "inventory add",
			args: []string{"inventory", "add"},
			want: "--host is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtimeErrorNoUsage(t, tc.args, tc.want)
		})
	}
}

// TestFlagMisuseStillShowsUsage pins the other half of the DF-019 pattern:
// flag parsing errors and positional-argument misuse legitimately keep the
// usage block — only RUNTIME errors are silenced.
func TestFlagMisuseStillShowsUsage(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "unknown flag on protocol graphql",
			args: []string{"protocol", "graphql", "--no-such-flag"},
		},
		{
			name: "unknown flag on grpc call",
			args: []string{"grpc", "call", "kv.KeyValue-Get", "--no-such-flag"},
		},
		{
			// grpc discover takes exactly one positional argument; wrong arg
			// count un-silences usage through the wrapped Args validator.
			name: "wrong arg count on grpc discover",
			args: []string{"grpc", "discover"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand(nil)
			root.SetArgs(tc.args)
			stderr := &bytes.Buffer{}
			root.SetOut(stderr)
			root.SetErr(stderr)

			err := root.Execute()
			require.Error(t, err)
			fmt.Fprintf(stderr, "Error: %v\n", err)

			assert.Contains(t, stderr.String(), "Usage:")
		})
	}
}
