package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// PruneRetention removes records older than retentionDays from path and returns
// the number of records removed. The removed prefix is rotated to a sibling
// .archive-<timestamp>.jsonl file before the active sink is atomically replaced.
//
// The active file is re-anchored and re-hashed after pruning, so VerifyChain
// remains valid. The rotated prefix is a complete, independently verifiable
// chain segment. This deliberately does not claim hash continuity across the
// retention boundary: retention is an explicit deletion policy.
func PruneRetention(path string, retentionDays int, algorithm string) (int, error) {
	if path == "" {
		return 0, fmt.Errorf("audit: empty sink path")
	}
	if retentionDays <= 0 {
		return 0, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("audit: open sink for retention: %w", err)
	}
	defer func() { _ = file.Close() }()

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	// Fast path: the sink is append-ordered, so if its OLDEST record is still
	// inside the window there is nothing to rotate. Reading just the first
	// record keeps the per-process cost O(1); without it every invocation that
	// enables audit paid a full-file scan before doing any work, which defeats
	// the bounded --limit reader (measured 7.7x slowdown at 10k rows).
	firstTimestamp, hasFirst, err := peekOldestAuditTimestamp(file)
	if err != nil {
		return 0, err
	}
	if hasFirst && !firstTimestamp.Before(cutoff) {
		return 0, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("audit: rewind sink for retention: %w", err)
	}

	var events []AuditEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if scanner.Text() == "" {
			continue
		}
		var event AuditEvent
		if err := json.Unmarshal([]byte(scanner.Text()), &event); err != nil {
			return 0, fmt.Errorf("audit: retention found invalid JSON: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("audit: read sink for retention: %w", err)
	}
	keepAt := 0
	for keepAt < len(events) && events[keepAt].Timestamp.Before(cutoff) {
		keepAt++
	}
	if keepAt == 0 {
		return 0, nil
	}

	archivePath := fmt.Sprintf("%s.archive-%d.jsonl", path, time.Now().UTC().UnixNano())
	algorithm = normalizeHashAlgorithm(algorithm)
	archived := append([]AuditEvent(nil), events[:keepAt]...)
	previous := ""
	for i := range archived {
		archived[i].PreviousHash = previous
		archived[i].Hash = computeEventHash(algorithm, &archived[i])
		previous = archived[i].Hash
	}
	if err := writeAuditEvents(archivePath, archived); err != nil {
		return 0, err
	}

	kept := events[keepAt:]
	previous = ""
	for i := range kept {
		kept[i].PreviousHash = previous
		kept[i].Hash = computeEventHash(algorithm, &kept[i])
		previous = kept[i].Hash
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".audit-retention-*")
	if err != nil {
		return 0, fmt.Errorf("audit: create retention temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := writeAuditEventsTo(tmp, kept); err != nil {
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("audit: sync retention temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("audit: close retention temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return 0, fmt.Errorf("audit: secure retention temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return 0, fmt.Errorf("audit: replace sink after retention: %w", err)
	}
	return keepAt, nil
}

// peekOldestAuditTimestamp returns the timestamp of the sink's first non-empty
// record. Retention is append-ordered, so an in-window oldest record proves the
// whole file is in-window and the caller can skip a full scan.
func peekOldestAuditTimestamp(file *os.File) (time.Time, bool, error) {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if scanner.Text() == "" {
			continue
		}
		var event AuditEvent
		if err := json.Unmarshal([]byte(scanner.Text()), &event); err != nil {
			return time.Time{}, false, fmt.Errorf("audit: retention found invalid JSON: %w", err)
		}
		return event.Timestamp, true, nil
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, false, fmt.Errorf("audit: read sink head for retention: %w", err)
	}
	return time.Time{}, false, nil
}

func writeAuditEvents(path string, events []AuditEvent) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("audit: create rotated segment %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := writeAuditEventsTo(file, events); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("audit: sync rotated segment: %w", err)
	}
	return nil
}

func writeAuditEventsTo(w io.Writer, events []AuditEvent) error {
	encoder := json.NewEncoder(w)
	for i := range events {
		if err := encoder.Encode(&events[i]); err != nil {
			return fmt.Errorf("audit: encode retained event: %w", err)
		}
	}
	return nil
}
