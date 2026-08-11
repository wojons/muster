package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRun_BareHelpExitsZero verifies that bare --help prints usage and
// returns nil (exit 0) instead of surfacing flag.ErrHelp as a fatal error.
func TestRun_BareHelpExitsZero(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"muster-daemon", "--help"}
	require.NoError(t, run(), "--help should return nil (exit 0)")
}

// TestRun_ShortHelpExitsZero verifies the -h form of the help flag.
func TestRun_ShortHelpExitsZero(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"muster-daemon", "-h"}
	require.NoError(t, run(), "-h should return nil (exit 0)")
}

// TestRun_HelpAfterSubcommandExitsZero verifies that "start --help" also
// exits 0 — the subcommand scanner strips "start" before flag parsing, so
// --help must be handled by the same ErrHelp path.
func TestRun_HelpAfterSubcommandExitsZero(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"muster-daemon", "start", "--help"}
	require.NoError(t, run(), "start --help should return nil (exit 0)")
}

// TestRun_UnknownFlagStillFails verifies real flag errors are NOT masked by
// the ErrHelp special-case: an undefined flag must still return an error.
func TestRun_UnknownFlagStillFails(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"muster-daemon", "--definitely-not-a-flag"}
	require.Error(t, run(), "unknown flag should still return an error")
}
