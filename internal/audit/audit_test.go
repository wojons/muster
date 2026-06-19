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
	os.Setenv("MUSTER_AUDIT_ENABLED", "1")
	defer os.Unsetenv("MUSTER_AUDIT_ENABLED")

	// Clean up any leftover audit files from previous tests
	auditDir := filepath.Join(os.TempDir(), "muster-audit")
	os.RemoveAll(auditDir)

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

	// Verify audit log file was created
	auditPath := filepath.Join(os.TempDir(), "muster-audit", "audit.log")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("ReadFile audit log: %v", err)
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

func TestHook_LogToolCall(t *testing.T) {
	os.Setenv("MUSTER_AUDIT_ENABLED", "1")
	defer os.Unsetenv("MUSTER_AUDIT_ENABLED")

	// Clean up any leftover audit files
	auditDir := filepath.Join(os.TempDir(), "muster-audit")
	os.RemoveAll(auditDir)

	hook := NewHook(DefaultConfig())
	ctx := context.Background()
	hook.LogToolCall(ctx, "openapi-parse", "mcp-client-1", true, map[string]string{
		"spec": "petstore.yaml",
	})

	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	auditPath := filepath.Join(os.TempDir(), "muster-audit", "audit.log")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
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
	os.Setenv("MUSTER_AUDIT_ENABLED", "1")
	defer os.Unsetenv("MUSTER_AUDIT_ENABLED")

	// Clean up any leftover audit files
	auditDir := filepath.Join(os.TempDir(), "muster-audit")
	os.RemoveAll(auditDir)

	hook := NewHook(DefaultConfig())
	ctx := context.Background()

	// Successful auth
	hook.LogAuth(ctx, "user-1", true, map[string]string{"method": "oauth2"})

	// Failed auth
	hook.LogAuth(ctx, "user-2", false, map[string]string{"method": "basic", "reason": "invalid credentials"})

	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	auditPath := filepath.Join(os.TempDir(), "muster-audit", "audit.log")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
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
		Success:   true,
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
