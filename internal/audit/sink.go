package audit

import (
	"os"
	"path/filepath"
)

// SinkPath returns the single audit sink path shared by every audit writer in
// Muster:
//
//   - the command-level hook (Hook.ensureInit), which records ordinary CLI
//     command and MCP tool events, and
//   - the management-plane emitter (internal/builtin.emitAuditEvent), which
//     records tenant and webhook mutations,
//
// and read by the `audit log` / `audit show` / `audit info` commands.
//
// Resolution order:
//
//  1. $MUSTER_AUDIT_LOG, when set to a non-empty value. This operator override
//     wins for writers and readers alike.
//  2. DefaultSinkPath(): <config dir>/muster/audit.jsonl, the JSON-lines store
//     the CLI has always read.
//
// The parent directory is not created here; callers that write must create it
// first (0700).
func SinkPath() string {
	if p := os.Getenv("MUSTER_AUDIT_LOG"); p != "" {
		return p
	}
	return DefaultSinkPath()
}

// DefaultSinkPath returns the audit sink used when $MUSTER_AUDIT_LOG is unset:
// <config dir>/muster/audit.jsonl, where <config dir> is the OS configuration
// directory ($XDG_CONFIG_HOME or $HOME/.config on Unix, %AppData% on Windows).
func DefaultSinkPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "muster", "audit.jsonl")
}
