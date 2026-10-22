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

// readSinkLines returns the non-empty JSON lines of an audit sink.
func readSinkLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// writeSinkLines rewrites an audit sink with the given lines (tamper helper).
func writeSinkLines(t *testing.T, path string, lines []string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// TestRedactSensitive_ReplacesMandatoryFields is the DOC-13 unit arm for the
// redaction table: every vocabulary the spec names (password, token, secret,
// api_key, authorization, card, ssn) is replaced, including decorated spellings
// and nested containers.
func TestRedactSensitive_ReplacesMandatoryFields(t *testing.T) {
	data := map[string]interface{}{
		"password":      "hunter2",
		"passwd":        "hunter2",
		"pwd":           "hunter2",
		"token":         "tok_live_123",
		"access_token":  "tok_live_123",
		"refresh_token": "tok_live_123",
		"secret":        "s3cr3t",
		"client_secret": "s3cr3t",
		"api_key":       "sk-live-abc",
		"apikey":        "sk-live-abc",
		"X-API-Key":     "sk-live-abc",
		"Authorization": "Bearer abc.def.ghi",
		"auth_header":   "Bearer abc.def.ghi",
		"credit_card":   "4111111111111111",
		"card_number":   "4111111111111111",
		"cc_number":     "4111111111111111",
		"ssn":           "000-00-0000",
		"db_password":   "hunter2",
		"nested": map[string]interface{}{
			"api_key": "sk-nested",
			"deep": map[string]interface{}{
				"refresh_token": "tok-nested",
			},
		},
		"list": []interface{}{
			map[string]interface{}{"secret": "list-secret"},
		},
	}

	RedactSensitive(data)

	for _, field := range []string{
		"password", "passwd", "pwd", "token", "access_token", "refresh_token",
		"secret", "client_secret", "api_key", "apikey", "X-API-Key",
		"Authorization", "auth_header", "credit_card", "card_number", "cc_number",
		"ssn", "db_password",
	} {
		if got, ok := data[field].(string); !ok || got != RedactedValue {
			t.Errorf("%s = %v, want %q", field, data[field], RedactedValue)
		}
	}

	nested := data["nested"].(map[string]interface{})
	if nested["api_key"] != RedactedValue {
		t.Errorf("nested api_key = %v, want %q", nested["api_key"], RedactedValue)
	}
	deep := nested["deep"].(map[string]interface{})
	if deep["refresh_token"] != RedactedValue {
		t.Errorf("nested deep refresh_token = %v, want %q", deep["refresh_token"], RedactedValue)
	}
	list := data["list"].([]interface{})
	if entry := list[0].(map[string]interface{}); entry["secret"] != RedactedValue {
		t.Errorf("list[0].secret = %v, want %q", entry["secret"], RedactedValue)
	}
}

// TestRedactSensitive_KeepsNonSensitiveFields pins the negative arm: redaction
// must not eat ordinary audit detail.
func TestRedactSensitive_KeepsNonSensitiveFields(t *testing.T) {
	data := map[string]interface{}{
		"command":    "inventory fetch",
		"status":     "OK",
		"tenant":     "acme",
		"duration_s": 1.5,
		"nested":     map[string]interface{}{"region": "eu-west"},
	}

	RedactSensitive(data)

	if data["command"] != "inventory fetch" || data["status"] != "OK" || data["tenant"] != "acme" {
		t.Errorf("redaction ate non-sensitive fields: %+v", data)
	}
	if nested, ok := data["nested"].(map[string]interface{}); !ok || nested["region"] != "eu-west" {
		t.Errorf("redaction ate nested non-sensitive field: %+v", data["nested"])
	}
}

// TestRedactSensitive_NilMapIsSafe covers the degenerate input.
func TestRedactSensitive_NilMapIsSafe(t *testing.T) {
	var data map[string]interface{}
	RedactSensitive(data) // must not panic
	RedactSensitive(map[string]interface{}{})
}

// TestFileStorage_Write_RedactsAndChains proves the sink write path (the only
// place audit records are persisted) applies redaction and assigns the chain
// fields, and that a caller cannot escape redaction by leaving the config flag
// false.
func TestFileStorage_Write_RedactsAndChains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	// Config explicitly says "do not redact": the persisted record must still be
	// redacted, because the control is mandatory and lives in the write path.
	cfg := AuditConfig{Enabled: true, RedactSensitive: false, HashAlgorithm: "sha256"}
	if cfg.RedactSensitive {
		t.Fatal("test setup: RedactSensitive must be false to prove redaction is not gated on it")
	}

	fs, err := NewFileStorageWithHashAlgorithm(path, DefaultFileStorageOptions(), cfg.HashAlgorithm)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	event := &AuditEvent{
		EventType: EventTypeCommand,
		User:      "alice",
		Action:    "command.login",
		Details: map[string]interface{}{
			"password":    "hunter2",
			"api_key":     "sk-live-abc",
			"target_host": "example.test",
		},
		Context: AuditContext{Metadata: map[string]string{
			"authorization": "Bearer abc.def.ghi",
			"request_id":    "req-1",
		}},
	}
	if err := fs.Write(event); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readSinkLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	var stored AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if stored.Details["password"] != RedactedValue {
		t.Errorf("details.password = %v, want %q — sink persisted a credential", stored.Details["password"], RedactedValue)
	}
	if stored.Details["api_key"] != RedactedValue {
		t.Errorf("details.api_key = %v, want %q", stored.Details["api_key"], RedactedValue)
	}
	if stored.Details["target_host"] != "example.test" {
		t.Errorf("details.target_host = %v, want example.test", stored.Details["target_host"])
	}
	if stored.Context.Metadata["authorization"] != RedactedValue {
		t.Errorf("context.authorization = %q, want %q", stored.Context.Metadata["authorization"], RedactedValue)
	}
	if stored.Context.Metadata["request_id"] != "req-1" {
		t.Errorf("context.request_id = %q, want req-1", stored.Context.Metadata["request_id"])
	}

	if stored.ID == "" {
		t.Error("stored record has no id")
	}
	if len(stored.Hash) != 64 {
		t.Errorf("stored hash = %q (len %d), want 64 hex chars (sha256)", stored.Hash, len(stored.Hash))
	}
	if stored.PreviousHash != "" {
		t.Errorf("first record previous_hash = %q, want empty (chain start)", stored.PreviousHash)
	}
	if err := VerifyChain(path); err != nil {
		t.Fatalf("VerifyChain on freshly written sink: %v", err)
	}
}

// TestFileStorage_Write_HashChainLinksRecords proves the chain links: each
// record's previous_hash is the prior record's hash, IDs are unique, and the
// chain verifies.
func TestFileStorage_Write_HashChainLinksRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	fs, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	defer fs.Close()

	for i := 0; i < 3; i++ {
		event := &AuditEvent{
			EventType: EventTypeCommand,
			User:      "alice",
			Action:    "command.run",
			Details:   map[string]interface{}{"n": i},
		}
		if err := fs.Write(event); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	lines := readSinkLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}

	var prev string
	seenIDs := map[string]struct{}{}
	for i, line := range lines {
		var stored AuditEvent
		if err := json.Unmarshal([]byte(line), &stored); err != nil {
			t.Fatalf("line %d decode: %v", i+1, err)
		}
		if stored.Hash == "" {
			t.Fatalf("line %d has no hash", i+1)
		}
		if stored.PreviousHash != prev {
			t.Errorf("line %d previous_hash = %q, want %q (prior record's hash)", i+1, stored.PreviousHash, prev)
		}
		if want := computeEventHash(HashAlgorithmSHA256, &stored); stored.Hash != want {
			t.Errorf("line %d hash = %s, recomputed %s", i+1, stored.Hash, want)
		}
		if _, dup := seenIDs[stored.ID]; dup {
			t.Errorf("line %d id %q duplicates an earlier record", i+1, stored.ID)
		}
		seenIDs[stored.ID] = struct{}{}
		prev = stored.Hash
	}
	if err := VerifyChain(path); err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
}

// TestFileStorage_ChainContinuesAcrossReopen is the regression for the
// short-lived-writer case: the management-plane emitter opens a FileStorage per
// event, so a chain seeded only in memory would restart with an empty
// previous_hash on every event.
func TestFileStorage_ChainContinuesAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	first, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage #1: %v", err)
	}
	if err := first.Write(&AuditEvent{EventType: EventTypeAuth, Action: "authenticate"}); err != nil {
		t.Fatalf("Write #1: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	second, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatalf("NewFileStorage #2: %v", err)
	}
	if err := second.Write(&AuditEvent{EventType: EventTypeTool, Action: "tool.call"}); err != nil {
		t.Fatalf("Write #2: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close #2: %v", err)
	}

	lines := readSinkLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	var a, b AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &a); err != nil {
		t.Fatalf("decode a: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &b); err != nil {
		t.Fatalf("decode b: %v", err)
	}
	if b.PreviousHash != a.Hash {
		t.Errorf("second record previous_hash = %q, want %q (first record's hash)", b.PreviousHash, a.Hash)
	}
	if err := VerifyChain(path); err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
}

// TestVerifyChain_DetectsTampering covers the tamper-evidence promise: editing a
// record body, reordering records, and dropping a record are each detected.
func TestVerifyChain_DetectsTampering(t *testing.T) {
	build := func(t *testing.T) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		fs, err := NewFileStorage(path, DefaultFileStorageOptions())
		if err != nil {
			t.Fatalf("NewFileStorage: %v", err)
		}
		defer fs.Close()
		for i := 0; i < 3; i++ {
			if err := fs.Write(&AuditEvent{
				EventType: EventTypeCommand,
				User:      "alice",
				Action:    "command.run",
				Details:   map[string]interface{}{"n": i},
			}); err != nil {
				t.Fatalf("Write %d: %v", i, err)
			}
		}
		return path
	}

	t.Run("intact sink verifies", func(t *testing.T) {
		if err := VerifyChain(build(t)); err != nil {
			t.Fatalf("VerifyChain on intact sink: %v", err)
		}
	})

	t.Run("edited body is caught", func(t *testing.T) {
		path := build(t)
		lines := readSinkLines(t, path)
		lines[1] = strings.Replace(lines[1], `"command.run"`, `"command.exfiltrate"`, 1)
		writeSinkLines(t, path, lines)
		err := VerifyChain(path)
		if err == nil {
			t.Fatal("VerifyChain accepted a rewritten record body")
		}
		if !strings.Contains(err.Error(), "line 2") {
			t.Errorf("error should name line 2, got: %v", err)
		}
	})

	t.Run("dropped record is caught", func(t *testing.T) {
		path := build(t)
		lines := readSinkLines(t, path)
		writeSinkLines(t, path, []string{lines[0], lines[2]})
		if err := VerifyChain(path); err == nil {
			t.Fatal("VerifyChain accepted a sink with a removed record")
		}
	})

	t.Run("reordered records are caught", func(t *testing.T) {
		path := build(t)
		lines := readSinkLines(t, path)
		writeSinkLines(t, path, []string{lines[1], lines[0], lines[2]})
		if err := VerifyChain(path); err == nil {
			t.Fatal("VerifyChain accepted reordered records")
		}
	})

	t.Run("blanked hash is caught", func(t *testing.T) {
		path := build(t)
		lines := readSinkLines(t, path)
		var stored AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &stored); err != nil {
			t.Fatalf("decode: %v", err)
		}
		stored.Hash = ""
		data, err := json.Marshal(stored)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		writeSinkLines(t, path, []string{string(data), lines[1], lines[2]})
		if err := VerifyChain(path); err == nil {
			t.Fatal("VerifyChain accepted a record whose hash was blanked")
		}
	})
}

// TestVerifyChain_MissingSinkIsClean covers the no-audit-yet case.
func TestVerifyChain_MissingSinkIsClean(t *testing.T) {
	if err := VerifyChain(filepath.Join(t.TempDir(), "absent.jsonl")); err != nil {
		t.Fatalf("VerifyChain on missing sink: %v", err)
	}
}

// TestWriteEvent_ProductionEntryPoint exercises the sink write helper on the
// real sink resolution path ($MUSTER_AUDIT_LOG), including directory creation.
func TestWriteEvent_ProductionEntryPoint(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "nested", "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	if err := WriteEvent(&AuditEvent{
		EventType: EventTypeTool,
		User:      "mcp-alice",
		Action:    "tool.call",
		Details:   map[string]interface{}{"token": "tok_live_123"},
	}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	lines := readSinkLines(t, sink)
	if len(lines) != 1 {
		t.Fatalf("want 1 record, got %d", len(lines))
	}
	var stored AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stored.Hash == "" || stored.ID == "" {
		t.Errorf("WriteEvent did not stamp chain fields: id=%q hash=%q", stored.ID, stored.Hash)
	}
	if stored.Details["token"] != RedactedValue {
		t.Errorf("WriteEvent persisted token = %v, want %q", stored.Details["token"], RedactedValue)
	}
	if err := VerifyChain(sink); err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
}

// TestHook_WritesRedactedChainedRecords is the end-to-end arm: a real CLI/MCP
// entry-point hook must produce redacted, chained records on the shared sink.
func TestHook_WritesRedactedChainedRecords(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	hook := NewHook(DefaultConfig())
	hook.LogCommand(context.Background(), "login", "alice", true, map[string]string{
		"password": "hunter2",
		"api_key":  "sk-live-abc",
	})
	hook.LogAuth(context.Background(), "alice", false, map[string]string{
		"authorization": "Bearer abc.def.ghi",
	})
	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readSinkLines(t, sink)
	if len(lines) != 2 {
		t.Fatalf("want 2 records, got %d: %v", len(lines), lines)
	}

	var first, second AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if first.Context.Metadata["password"] != RedactedValue {
		t.Errorf("hook persisted password = %q, want %q", first.Context.Metadata["password"], RedactedValue)
	}
	if first.Context.Metadata["api_key"] != RedactedValue {
		t.Errorf("hook persisted api_key = %q, want %q", first.Context.Metadata["api_key"], RedactedValue)
	}
	if second.Context.Metadata["authorization"] != RedactedValue {
		t.Errorf("hook persisted authorization = %q, want %q", second.Context.Metadata["authorization"], RedactedValue)
	}
	if second.PreviousHash != first.Hash {
		t.Errorf("second record previous_hash = %q, want %q", second.PreviousHash, first.Hash)
	}
	if err := VerifyChain(sink); err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
}

// TestAuditEvent_ChainFieldWireShape pins the snake_case names the JSONL
// consumers read (the audit sink is a machine-readable wire format).
func TestAuditEvent_ChainFieldWireShape(t *testing.T) {
	event := &AuditEvent{
		ID:           "abc",
		Timestamp:    time.Unix(0, 0).UTC(),
		EventType:    EventTypeCommand,
		Action:       "command.run",
		Hash:         "deadbeef",
		PreviousHash: "cafebabe",
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	line := string(raw)
	for _, key := range []string{`"id":"abc"`, `"hash":"deadbeef"`, `"previous_hash":"cafebabe"`} {
		if !strings.Contains(line, key) {
			t.Errorf("wire shape missing %s: %s", key, line)
		}
	}
	for _, bad := range []string{`"ID":`, `"Hash":`, `"PreviousHash":`} {
		if strings.Contains(line, bad) {
			t.Errorf("wire shape leaks Go field name %s: %s", bad, line)
		}
	}
}

// TestFileStorage_SHA512Chain proves AuditConfig.HashAlgorithm is actually
// consumed: a sha512 sink chains with 128-char digests and verifies only under
// the algorithm it was written with.
func TestFileStorage_SHA512Chain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	fs, err := NewFileStorageWithHashAlgorithm(path, DefaultFileStorageOptions(), HashAlgorithmSHA512)
	if err != nil {
		t.Fatalf("NewFileStorageWithHashAlgorithm: %v", err)
	}
	if err := fs.Write(&AuditEvent{EventType: EventTypeCommand, Action: "command.run"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var stored AuditEvent
	if err := json.Unmarshal([]byte(readSinkLines(t, path)[0]), &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(stored.Hash) != 128 {
		t.Errorf("sha512 hash len = %d, want 128 (%s)", len(stored.Hash), stored.Hash)
	}
	if err := VerifyChainWithAlgorithm(path, HashAlgorithmSHA512); err != nil {
		t.Fatalf("VerifyChainWithAlgorithm(sha512): %v", err)
	}
	if err := VerifyChain(path); err == nil {
		t.Error("VerifyChain with the default sha256 accepted a sha512 sink")
	}
}
