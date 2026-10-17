package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// axiom:trace work_item=prod-readiness-01 spec=specs/EVENTS-AUDIT.md plan=phase-4/task-4.1/subtask-4.1.1

func TestFileStorage_Write(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	defer fs.Close()

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test.write",
		Actor:    AuditActor{ID: "tester", Type: "test", Name: "tester"},
		Resource: AuditResource{ID: "test-resource", Type: "test", Name: "test-resource"},
		Result:   AuditResult{Success: true, Code: "OK"},
	}

	if err := fs.Write(event); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var decoded AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Action != "test.write" {
		t.Errorf("Action = %q, want %q", decoded.Action, "test.write")
	}
	if decoded.Actor.ID != "tester" {
		t.Errorf("Actor.ID = %q, want %q", decoded.Actor.ID, "tester")
	}
	if !decoded.Result.Success {
		t.Error("Result.Success should be true")
	}
	if decoded.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestFileStorage_Write_Multiple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	defer fs.Close()

	for i := 0; i < 5; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "test.multi",
			Actor:    AuditActor{ID: "tester", Type: "test"},
			Result:   AuditResult{Success: true, Code: "OK"},
		}
		if err := fs.Write(event); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
}

func TestFileStorage_Write_Concurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	defer fs.Close()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			event := &AuditEvent{
				Level:    AuditLevelInfo,
				Category: AuditCategorySystem,
				Action:   "test.concurrent",
				Actor:    AuditActor{ID: "tester", Type: "test"},
				Result:   AuditResult{Success: true, Code: "OK"},
			}
			_ = fs.Write(event)
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines))
	}
}

func TestAuditLogger_Log(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	cfg := AuditConfig{Enabled: true, BufferSize: 10}
	logger := NewAuditLogger(cfg, fs)

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test.log",
		Actor:    AuditActor{ID: "tester", Type: "test"},
		Result:   AuditResult{Success: true, Code: "OK"},
	}

	if err := logger.Log(context.Background(), event); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Give consumer goroutine time to process
	time.Sleep(50 * time.Millisecond)

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestAuditLogger_Close_DrainsEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	cfg := AuditConfig{Enabled: true, BufferSize: 100}
	logger := NewAuditLogger(cfg, fs)

	for i := 0; i < 20; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "test.drain",
			Actor:    AuditActor{ID: "tester", Type: "test"},
			Result:   AuditResult{Success: true, Code: "OK"},
		}
		_ = logger.Log(context.Background(), event)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 20 {
		t.Fatalf("expected 20 lines, got %d", len(lines))
	}
}

func TestHook_LogCommand(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	// The hook must resolve the shared sink (AUD-001), not a private default.
	if got := SinkPath(); got != sink {
		t.Fatalf("SinkPath() = %q, want %q", got, sink)
	}

	hook := NewHook(DefaultConfig())
	if !hook.config.Enabled {
		t.Fatal("hook should be enabled with MUSTER_AUDIT_ENABLED=1")
	}

	ctx := context.Background()
	hook.LogCommand(ctx, "inventory-fetch", "cli-user", true, map[string]string{
		"api": "petstore",
	})

	// Close to flush
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify the event landed on the resolved sink.
	auditPath := SinkPath()
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("ReadFile audit log %s: %v", auditPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 {
		t.Fatal("expected at least 1 audit event")
	}

	var decoded AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Action != "command.inventory-fetch" {
		t.Errorf("Action = %q, want %q", decoded.Action, "command.inventory-fetch")
	}
	if decoded.Actor.ID != "cli-user" {
		t.Errorf("Actor.ID = %q, want %q", decoded.Actor.ID, "cli-user")
	}
	if !decoded.Result.Success {
		t.Error("Result.Success should be true")
	}
	if decoded.Context.Metadata["api"] != "petstore" {
		t.Errorf("Metadata[api] = %q, want %q", decoded.Context.Metadata["api"], "petstore")
	}
}

// TestHook_LogToolCall pins the non-override resolution end to end: with
// $MUSTER_AUDIT_LOG unset the command-level hook writes to
// <config dir>/muster/audit.jsonl. Before AUD-001 this hook wrote
// $TMPDIR/muster-audit/audit.log, a file no reader ever opened.
func TestHook_LogToolCall(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	sink := defaultSinkUnderHome(t, home)

	hook := NewHook(DefaultConfig())
	ctx := context.Background()
	hook.LogToolCall(ctx, "openapi-parse", "mcp-client-1", true, map[string]string{
		"spec": "petstore.yaml",
	})

	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := SinkPath(); got != sink {
		t.Fatalf("SinkPath() = %q, want %q", got, sink)
	}
	data, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", sink, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 {
		t.Fatal("expected at least 1 audit event")
	}

	var decoded AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Action != "tool.openapi-parse" {
		t.Errorf("Action = %q, want %q", decoded.Action, "tool.openapi-parse")
	}
	if decoded.Actor.Type != "mcp-client" {
		t.Errorf("Actor.Type = %q, want %q", decoded.Actor.Type, "mcp-client")
	}
}

func TestHook_LogAuth(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	hook := NewHook(DefaultConfig())
	ctx := context.Background()

	// Successful auth
	hook.LogAuth(ctx, "user-1", true, map[string]string{"method": "oauth2"})

	// Failed auth
	hook.LogAuth(ctx, "user-2", false, map[string]string{"method": "basic", "reason": "invalid credentials"})

	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", sink, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 audit events, got %d", len(lines))
	}

	// First event: success
	var decoded1 AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &decoded1); err != nil {
		t.Fatalf("json.Unmarshal line 0: %v", err)
	}
	if decoded1.Level != AuditLevelInfo {
		t.Errorf("successful auth should be info level, got %s", decoded1.Level)
	}
	if !decoded1.Result.Success {
		t.Error("first auth should be successful")
	}

	// Second event: failure
	var decoded2 AuditEvent
	if err := json.Unmarshal([]byte(lines[1]), &decoded2); err != nil {
		t.Fatalf("json.Unmarshal line 1: %v", err)
	}
	if decoded2.Level != AuditLevelWarning {
		t.Errorf("failed auth should be warning level, got %s", decoded2.Level)
	}
	if decoded2.Result.Success {
		t.Error("second auth should be failed")
	}
}

func TestHook_Disabled(t *testing.T) {
	os.Setenv("MUSTER_AUDIT_ENABLED", "0")
	defer os.Unsetenv("MUSTER_AUDIT_ENABLED")

	hook := NewHook(DefaultConfig())
	if hook.config.Enabled {
		t.Fatal("hook should be disabled")
	}

	ctx := context.Background()
	hook.LogCommand(ctx, "test", "user", true, nil)

	// Should not panic or create files
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	os.Unsetenv("MUSTER_AUDIT_ENABLED")

	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("DefaultConfig should be disabled without env var")
	}
	if cfg.StorageType != "file" {
		t.Errorf("StorageType = %q, want %q", cfg.StorageType, "file")
	}
	if cfg.RetentionDays != 90 {
		t.Errorf("RetentionDays = %d, want 90", cfg.RetentionDays)
	}
	if cfg.BufferSize != 100 {
		t.Errorf("BufferSize = %d, want 100", cfg.BufferSize)
	}
	if cfg.HashAlgorithm != "sha256" {
		t.Errorf("HashAlgorithm = %q, want %q", cfg.HashAlgorithm, "sha256")
	}

	os.Setenv("MUSTER_AUDIT_ENABLED", "1")
	cfg2 := DefaultConfig()
	if !cfg2.Enabled {
		t.Error("DefaultConfig should be enabled with MUSTER_AUDIT_ENABLED=1")
	}
}

func TestDefaultConfigEnabled(t *testing.T) {
	os.Unsetenv("MUSTER_AUDIT_ENABLED")

	cfg := DefaultConfigEnabled()
	if !cfg.Enabled {
		t.Error("DefaultConfigEnabled should always be enabled")
	}
}

func TestResultCode(t *testing.T) {
	if resultCode(true) != "OK" {
		t.Errorf("resultCode(true) = %q, want %q", resultCode(true), "OK")
	}
	if resultCode(false) != "ERROR" {
		t.Errorf("resultCode(false) = %q, want %q", resultCode(false), "ERROR")
	}
}

func TestFileStorage_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Second close should not panic (file already closed)
	// This is expected to return an error on some platforms
	_ = fs.Close()
}

func TestFileStorage_Interface(t *testing.T) {
	// Compile-time check that FileStorage implements AuditStorage
	var _ AuditStorage = (*FileStorage)(nil)
	// This test just confirms the interface assertion compiles
}

func TestAuditLogger_Log_Disabled(t *testing.T) {
	cfg := AuditConfig{Enabled: false, BufferSize: 10}
	logger := NewAuditLogger(cfg, nil)

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test.disabled",
	}

	// Should not panic even with nil storage
	err := logger.Log(context.Background(), event)
	if err != nil {
		t.Errorf("Log with disabled config should not error: %v", err)
	}

	// Close should not panic
	_ = logger.Close()
}

func TestAuditEvent_JSON_RoundTrip(t *testing.T) {
	event := &AuditEvent{
		Timestamp: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
		EventType: "command",
		User:      "test-user",
		Level:     AuditLevelInfo,
		Category:  AuditCategorySystem,
		Action:    "test.action",
		Actor:     AuditActor{ID: "actor-1", Type: "user", Name: "Test Actor"},
		Resource:  AuditResource{ID: "res-1", Type: "api", Name: "Test Resource"},
		Result:    AuditResult{Success: true, Code: "OK"},
		Context:   AuditContext{Metadata: map[string]string{"key": "value"}},
		Details:   map[string]interface{}{"count": float64(42)},
		Error:     "",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Action != event.Action {
		t.Errorf("Action = %q, want %q", decoded.Action, event.Action)
	}
	if decoded.Actor.ID != event.Actor.ID {
		t.Errorf("Actor.ID = %q, want %q", decoded.Actor.ID, event.Actor.ID)
	}
	if decoded.Result.Code != event.Result.Code {
		t.Errorf("Result.Code = %q, want %q", decoded.Result.Code, event.Result.Code)
	}
	if decoded.Context.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %q, want %q", decoded.Context.Metadata["key"], "value")
	}
}

// TestHook_LogCommand_Disabled verifies LogCommand does not panic or create
// files when audit is disabled (config.Enabled=false).
func TestHook_LogCommand_Disabled(t *testing.T) {
	os.Unsetenv("MUSTER_AUDIT_ENABLED")

	cfg := AuditConfig{Enabled: false, BufferSize: 10}
	hook := NewHook(cfg)

	if hook.config.Enabled {
		t.Fatal("hook should be disabled")
	}

	ctx := context.Background()
	// Should not panic and should not create any files
	hook.LogCommand(ctx, "inventory-fetch", "cli-user", true, map[string]string{
		"api": "petstore",
	})

	// Close should not panic even with no logger
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestHook_LogToolCall_Disabled verifies LogToolCall does not panic or create
// files when audit is disabled (config.Enabled=false).
func TestHook_LogToolCall_Disabled(t *testing.T) {
	os.Unsetenv("MUSTER_AUDIT_ENABLED")

	cfg := AuditConfig{Enabled: false, BufferSize: 10}
	hook := NewHook(cfg)

	if hook.config.Enabled {
		t.Fatal("hook should be disabled")
	}

	ctx := context.Background()
	// Should not panic and should not create any files
	hook.LogToolCall(ctx, "list-tasks", "mcp-client", true, nil)

	// Close should not panic even with no logger
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestAuditLogger_Log_BufferFull verifies that Log returns nil (silent drop)
// when the event channel buffer is full. Uses BufferSize=1 and sends 2 events
// rapidly — the second hits the default branch.
func TestAuditLogger_Log_BufferFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	// BufferSize=1 means only one event fits before consumer picks it up
	cfg := AuditConfig{Enabled: true, BufferSize: 1}
	logger := NewAuditLogger(cfg, fs)

	event1 := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test.buffer-full-1",
		Actor:    AuditActor{ID: "tester", Type: "test"},
		Result:   AuditResult{Success: true, Code: "OK"},
	}
	event2 := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test.buffer-full-2",
		Actor:    AuditActor{ID: "tester", Type: "test"},
		Result:   AuditResult{Success: true, Code: "OK"},
	}

	// Send first event — fills the buffer
	if err := logger.Log(context.Background(), event1); err != nil {
		t.Fatalf("Log event1: %v", err)
	}

	// Send second event immediately — should hit default (buffer full)
	// Both paths return nil, so we just verify no panic and no error
	if err := logger.Log(context.Background(), event2); err != nil {
		t.Fatalf("Log event2: %v", err)
	}

	// Close logger to flush remaining events
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// At least one event should be in the log (event1 was queued before consumer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		t.Error("expected at least one event in audit log")
	}
}

// ── AUD-001: one audit sink for both writers ─────────────────────────────

// defaultSinkUnderHome returns the fallback sink path expected when HOME is
// redirected to home and $MUSTER_AUDIT_LOG is unset. It skips when the
// platform's config-dir resolution ignores HOME, so the assertion can never
// become vacuous.
func defaultSinkUnderHome(t *testing.T, home string) string {
	t.Helper()
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir: %v", err)
	}
	if !strings.HasPrefix(cfgDir, home) {
		t.Skipf("os.UserConfigDir() ignores HOME on this platform (cfgDir=%q)", cfgDir)
	}
	return filepath.Join(cfgDir, "muster", "audit.jsonl")
}

// TestSinkPath_MUSTER_AUDIT_LOGWins pins the operator override: a non-empty
// $MUSTER_AUDIT_LOG is the sink for every writer, whatever the config dir says.
func TestSinkPath_MUSTER_AUDIT_LOGWins(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom", "events.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", override)

	if got := SinkPath(); got != override {
		t.Fatalf("SinkPath() = %q, want the override %q", got, override)
	}
	if got := DefaultSinkPath(); got == override {
		t.Fatal("DefaultSinkPath() must stay the config-dir path, got the override")
	}
}

// TestSinkPath_ConfigDirFallback pins the fallback rule: with no override the
// sink is <config dir>/muster/audit.jsonl — the JSON-lines store `audit log`
// reads — and never the legacy $TMPDIR/muster-audit/audit.log.
func TestSinkPath_ConfigDirFallback(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_LOG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	want := defaultSinkUnderHome(t, home)
	if got := SinkPath(); got != want {
		t.Fatalf("SinkPath() = %q, want %q", got, want)
	}
	if got := DefaultSinkPath(); got != want {
		t.Fatalf("DefaultSinkPath() = %q, want %q", got, want)
	}
	if base := filepath.Base(want); base != "audit.jsonl" {
		t.Fatalf("fallback file name = %q, want audit.jsonl", base)
	}
	if parent := filepath.Base(filepath.Dir(want)); parent != "muster" {
		t.Fatalf("fallback parent dir = %q, want muster", parent)
	}
	if legacy := filepath.Join(os.TempDir(), "muster-audit", "audit.log"); SinkPath() == legacy {
		t.Fatalf("SinkPath() still resolves the legacy temporary path %q", legacy)
	}
}

// TestSinkPath_BothWritersUseSinkPath proves the command hook resolves the sink
// through SinkPath: the file the hook actually wrote is the path the shared
// helper returns for the same environment. The management-plane writer is
// covered by TestAuditSink_BothWritersShareOnePath in internal/builtin.
func TestSinkPath_BothWritersUseSinkPath(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	sink := filepath.Join(t.TempDir(), "shared.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	hook := NewHook(DefaultConfigEnabled())
	hook.Log(&AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "command.audit-sink-probe",
		Actor:    AuditActor{ID: "probe", Type: "cli-user"},
		Result:   AuditResult{Success: true, Code: "OK"},
	})
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(SinkPath())
	if err != nil {
		t.Fatalf("ReadFile SinkPath() = %s: %v", SinkPath(), err)
	}
	if !strings.Contains(string(data), "command.audit-sink-probe") {
		t.Fatalf("hook event missing from the SinkPath() file: %q", string(data))
	}
}

// TestHook_JSONL_WireFormat_SnakeCase is the formatting contract for DF-051:
// the audit sink is a machine-readable wire format, so a hook-written row must
// carry a NON-EMPTY event_type and snake_case nested keys. Without this, a
// reflection/reader regression makes every `audit show`/`audit log` row render a
// blank event_type column while the JSONL silently carries Go field names
// (actor.ID, result.Success) that no consumer documented.
func TestHook_JSONL_WireFormat_SnakeCase(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	hook := NewHook(DefaultConfig())
	hook.LogCommand(context.Background(), "inventory fetch", "tester", true, map[string]string{"k": "v"})
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", sink, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 event line, got %d: %q", len(lines), string(raw))
	}
	line := lines[0]

	// Byte-level: no Go field name may appear as a JSON key.
	for _, bad := range []string{`"ID":`, `"Type":`, `"Name":`, `"Success":`, `"Code":`, `"Metadata":`} {
		if strings.Contains(line, bad) {
			t.Errorf("wire shape leaks Go field name %s: %s", bad, line)
		}
	}

	// Structural: the documented snake_case keys must be present.
	var row map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("audit line is not valid JSON: %v", err)
	}
	for _, key := range []string{"event_type", "timestamp", "level", "category", "action", "actor", "resource", "result"} {
		if _, ok := row[key]; !ok {
			t.Errorf("wire shape missing top-level key %q: %s", key, line)
		}
	}
	var eventType string
	if err := json.Unmarshal(row["event_type"], &eventType); err != nil {
		t.Fatalf("event_type is not a string: %v", err)
	}
	if eventType != EventTypeCommand {
		t.Errorf("event_type = %q, want %q (blank means the emitter never populated it)", eventType, EventTypeCommand)
	}

	nested := []struct {
		field string
		keys  []string
	}{
		{"actor", []string{"id", "type", "name"}},
		{"resource", []string{"id", "type", "name"}},
		{"result", []string{"success", "code"}},
	}
	for _, want := range nested {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(row[want.field], &obj); err != nil {
			t.Fatalf("%s is not an object: %v", want.field, err)
		}
		for _, key := range want.keys {
			if _, ok := obj[key]; !ok {
				t.Errorf("%s is missing snake_case key %q: %s", want.field, key, line)
			}
		}
	}
	// Round-trip: the reader must still decode the documented shape.
	var decoded AuditEvent
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("readAuditEvents-style decode failed: %v", err)
	}
	if decoded.Actor.ID != "tester" || !decoded.Result.Success || decoded.Result.Code != "OK" {
		t.Errorf("decoded event lost data: actor.ID=%q result=%+v", decoded.Actor.ID, decoded.Result)
	}
}
