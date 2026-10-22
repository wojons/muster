package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
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

// WriteEvent appends one event to the shared audit sink (SinkPath), applying the
// mandatory sink transformations first: ID/timestamp stamping, sensitive-field
// redaction, and hash-chain linking.
//
// This is the write entry point for callers that persist a single event without
// holding a long-lived FileStorage (the management-plane emitter does exactly
// that). Callers that own a FileStorage may call its Write method directly; both
// paths go through the same prepareEvent + chain logic.
func WriteEvent(event *AuditEvent) error {
	return WriteEventTo(SinkPath(), event)
}

// WriteEventTo appends one event to the sink at path (creating the parent
// directory with 0700 when needed) using the configured hash algorithm.
//
// The redaction step inside the write path is mandatory: RedactSensitive runs on
// every record regardless of AuditConfig, so no caller can persist a credential
// by disabling a flag.
func WriteEventTo(path string, event *AuditEvent) error {
	if path == "" {
		return fmt.Errorf("audit: empty sink path")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("audit: create log dir for %s: %w", path, err)
		}
	}

	storage, err := NewFileStorageWithHashAlgorithm(path, DefaultFileStorageOptions(), DefaultConfig().HashAlgorithm)
	if err != nil {
		return fmt.Errorf("audit: open log %s: %w", path, err)
	}
	if err := storage.Write(event); err != nil {
		_ = storage.Close()
		return fmt.Errorf("audit: write event to %s: %w", path, err)
	}
	return storage.Close()
}

// prepareEvent applies the transformations every persisted audit record receives,
// in this order:
//
//  1. ID — a random identifier when the emitter did not supply one.
//  2. Timestamp — now (UTC) when the emitter did not supply one. It is stamped
//     before hashing because the hash covers it.
//  3. Redaction — RedactSensitive over Details and Context.Metadata. Mandatory
//     and not gated on configuration: specs/EVENTS-AUDIT.md requires redaction
//     that "cannot be disabled".
//
// Hashing happens inside FileStorage.Write, after this function, so the digest
// covers the redacted bytes that actually reach the disk.
func prepareEvent(event *AuditEvent) {
	if event.ID == "" {
		event.ID = newEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	applyRedaction(event)
}

// applyRedaction is the single place sensitive-field redaction is applied to an
// event on its way to storage. Both the sink write path (prepareEvent) and the
// spec-shaped Redactor handle call it, so there is exactly one definition of
// "what gets redacted".
//
// Typed, non-free-form fields (actor, resource, result) are left alone — their
// values are identifiers, not credentials.
func applyRedaction(event *AuditEvent) {
	if event == nil {
		return
	}
	if len(event.Details) > 0 {
		// Details is arbitrary JSON from the emitter: credentials land here.
		RedactSensitive(event.Details)
	}
	if len(event.Context.Metadata) > 0 {
		event.Context.Metadata = RedactMetadata(event.Context.Metadata)
	}
}
