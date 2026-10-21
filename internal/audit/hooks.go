package audit

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Hook provides a lightweight integration point for audit logging in
// application entry points (CLI, MCP server, daemon). It lazily
// initializes an AuditLogger on first use and exposes convenience
// methods for common audit events.
//
//	hook := audit.NewHook(audit.DefaultConfig())
//	defer hook.Close()
//	hook.LogCommand(ctx, "inventory fetch", "cli-user", true, nil)
type Hook struct {
	config AuditConfig
	mu     sync.Mutex
	logger *AuditLogger
}

// NewHook creates an audit hook with the given configuration.
// The logger is not initialized until the first call to Log or EnsureInit.
func NewHook(config AuditConfig) *Hook {
	return &Hook{config: config}
}

// ensureInit lazily creates the underlying AuditLogger on first use.
func (h *Hook) ensureInit() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.logger != nil {
		return nil
	}

	if !h.config.Enabled {
		return nil
	}

	// Resolve the one shared sink (SinkPath) so command-level events land in
	// the same JSON-lines file the management-plane emitter writes and the
	// `audit log` reader reads.
	path := SinkPath()
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("audit: create log dir: %w", err)
		}
	}

	storage, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		return fmt.Errorf("audit: open storage: %w", err)
	}

	h.logger = NewAuditLogger(h.config, storage)
	return nil
}

// Log records a generic audit event through the hook.
func (h *Hook) Log(event *AuditEvent) error {
	if err := h.ensureInit(); err != nil {
		return err
	}
	if h.logger == nil {
		return nil // disabled
	}
	return h.logger.Log(context.Background(), event)
}

// LogCommand records a CLI command execution.
func (h *Hook) LogCommand(ctx context.Context, command string, actorID string, success bool, details map[string]string) {
	actorID = normalizeActorID(actorID)
	event := &AuditEvent{
		EventType: EventTypeCommand,
		User:      actorID,
		Level:     AuditLevelInfo,
		Category:  AuditCategorySystem,
		Action:    fmt.Sprintf("command.%s", command),
		Actor: AuditActor{
			ID:   actorID,
			Type: "cli-user",
			Name: actorID,
		},
		Resource: AuditResource{
			ID:   command,
			Type: "command",
			Name: command,
		},
		Result: AuditResult{
			Success: success,
			Code:    resultCode(success),
		},
		Context: AuditContext{
			Metadata: details,
		},
	}

	if err := h.Log(event); err != nil {
		log.Printf("audit: failed to log command event: %v", err)
	}
}

// LogToolCall records an MCP tool invocation.
func (h *Hook) LogToolCall(ctx context.Context, toolName string, actorID string, success bool, details map[string]string) {
	actorID = normalizeActorID(actorID)
	event := &AuditEvent{
		EventType: EventTypeTool,
		User:      actorID,
		Level:     AuditLevelInfo,
		Category:  AuditCategoryAccess,
		Action:    fmt.Sprintf("tool.%s", toolName),
		Actor: AuditActor{
			ID:   actorID,
			Type: "mcp-client",
			Name: actorID,
		},
		Resource: AuditResource{
			ID:   toolName,
			Type: "tool",
			Name: toolName,
		},
		Result: AuditResult{
			Success: success,
			Code:    resultCode(success),
		},
		Context: AuditContext{
			Metadata: details,
		},
	}

	if err := h.Log(event); err != nil {
		log.Printf("audit: failed to log tool call event: %v", err)
	}
}

// LogAuth records an authentication event.
func (h *Hook) LogAuth(ctx context.Context, actorID string, success bool, details map[string]string) {
	actorID = normalizeActorID(actorID)
	level := AuditLevelInfo
	if !success {
		level = AuditLevelWarning
	}

	event := &AuditEvent{
		EventType: EventTypeAuth,
		User:      actorID,
		Level:     level,
		Category:  AuditCategoryAuth,
		Action:    "authenticate",
		Actor: AuditActor{
			ID:   actorID,
			Type: "user",
			Name: actorID,
		},
		Resource: AuditResource{
			ID:   "auth",
			Type: "authentication",
		},
		Result: AuditResult{
			Success: success,
			Code:    resultCode(success),
		},
		Context: AuditContext{
			Metadata: details,
		},
	}

	if err := h.Log(event); err != nil {
		log.Printf("audit: failed to log auth event: %v", err)
	}
}

// Close flushes and shuts down the underlying audit logger.
func (h *Hook) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.logger != nil {
		return h.logger.Close()
	}
	return nil
}

// DefaultConfig returns an audit config suitable for the CLI/MCP entry points.
// Enabled is false by default (opt-in via --audit flag or env var).
func DefaultConfig() AuditConfig {
	enabled := os.Getenv("MUSTER_AUDIT_ENABLED") == "1" || os.Getenv("MUSTER_AUDIT_ENABLED") == "true"
	return AuditConfig{
		Enabled:       enabled,
		StorageType:   "file",
		RetentionDays: 90,
		BufferSize:    100,
		FlushInterval: 5 * time.Second,
		HashAlgorithm: "sha256",
	}
}

// DefaultConfigEnabled returns an audit config with audit enabled by default.
func DefaultConfigEnabled() AuditConfig {
	cfg := DefaultConfig()
	cfg.Enabled = true
	return cfg
}

// normalizeActorID returns actorID, or "unknown" when the caller has no actor
// identity (empty string). AuditEvent.User and AuditActor.ID must never go to
// the wire empty (DF-055): an audit row with no identity is unreadable for
// `audit log` consumers, so a missing caller identity is recorded explicitly.
func normalizeActorID(actorID string) string {
	if actorID == "" {
		return "unknown"
	}
	return actorID
}

func resultCode(success bool) string {
	if success {
		return "OK"
	}
	return "ERROR"
}
