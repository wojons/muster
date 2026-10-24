package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneRetentionRemovesOldRowsAndPreservesChains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	base := time.Now().UTC().AddDate(0, 0, -10)
	for i, timestamp := range []time.Time{base, base.AddDate(0, 0, 2), time.Now().UTC()} {
		event := &AuditEvent{Timestamp: timestamp, EventType: "test", Action: "row-" + string(rune('0'+i))}
		if err := WriteEventTo(path, event); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := PruneRetention(path, 5, HashAlgorithmSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	if err := VerifyChain(path); err != nil {
		t.Fatalf("retained chain is invalid: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; got != 1 {
		t.Fatalf("retained row count = %d, want 1", got)
	}
	if !strings.Contains(string(data), "row-2") {
		t.Fatalf("newest row missing after prune: %s", data)
	}

	matches, err := filepath.Glob(path + ".archive-*.jsonl")
	if err != nil || len(matches) != 1 {
		t.Fatalf("archive count = %d, want 1", len(matches))
	}
	if err := VerifyChain(matches[0]); err != nil {
		t.Fatalf("rotated chain is invalid: %v", err)
	}
}

func TestPruneRetentionDoesNotRemoveRecentRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	event := &AuditEvent{Timestamp: time.Now().UTC(), Action: "recent"}
	if err := WriteEventTo(path, event); err != nil {
		t.Fatal(err)
	}
	removed, err := PruneRetention(path, 365, HashAlgorithmSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
}

func TestPruneRetentionRejectsMalformedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneRetention(path, 1, HashAlgorithmSHA256); err == nil {
		t.Fatal("expected malformed sink error")
	}
}

func TestPruneRetentionArchiveRowsAreNonVacuous(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	event := &AuditEvent{Timestamp: time.Now().UTC().AddDate(0, 0, -3), Action: "old-row"}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	removed, err := PruneRetention(path, 1, HashAlgorithmSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	matches, _ := filepath.Glob(path + ".archive-*.jsonl")
	if len(matches) != 1 {
		t.Fatalf("archive count = %d, want 1", len(matches))
	}
	archive, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(archive), "old-row") {
		t.Fatalf("archive does not contain removed row: %s", archive)
	}
}

// TestFileStorage_SurvivesRetentionRotation pins the concurrency contract: a
// writer that already holds the sink open must keep landing records AFTER a
// rotation renamed a new file into place, and the resulting trail must remain a
// valid chain. Without the inode check the writer appends to the unlinked
// inode and those records silently disappear.
func TestFileStorage_SurvivesRetentionRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	stale := &AuditEvent{Timestamp: time.Now().UTC().AddDate(0, 0, -30), Action: "stale-row", EventType: "test"}
	staleRaw, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(staleRaw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	storage, err := NewFileStorage(path, DefaultFileStorageOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = storage.Close() }()

	before := &AuditEvent{Timestamp: time.Now().UTC().Add(-time.Minute), Action: "before-rotation", EventType: "test"}
	if err := storage.Write(before); err != nil {
		t.Fatal(err)
	}

	removed, err := PruneRetention(path, 5, HashAlgorithmSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 (the stale row)", removed)
	}

	after := &AuditEvent{Timestamp: time.Now().UTC(), Action: "after-rotation", EventType: "test"}
	if err := storage.Write(after); err != nil {
		t.Fatalf("write after rotation: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(body, "stale-row") {
		t.Errorf("expired row survived rotation: %s", body)
	}
	if !strings.Contains(body, "after-rotation") {
		t.Fatalf("the post-rotation write was lost (appended to the unlinked inode): %s", body)
	}
	if err := VerifyChain(path); err != nil {
		t.Fatalf("chain invalid after a write that straddled the rotation: %v", err)
	}
	if after.PreviousHash != before.Hash {
		t.Errorf("post-rotation record linked to %q, want the retained tail %q", after.PreviousHash, before.Hash)
	}
}
