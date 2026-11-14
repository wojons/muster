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
// .archive-<timestamp>.jsonl file before the active sink is rewritten in place so open readers do not block retention.
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

	// Fast path: the sink is append-ordered (oldest first), so if its FIRST
	// record is still inside the window nothing can be expired and the full
	// scan is skipped. Retention runs on every writer open, i.e. once per
	// audited invocation, so without this an O(file) scan is paid before any
	// command does its own work — which would swallow the bound the tail
	// reader gives `audit log`.
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
	// Windows cannot replace the sink while our own reader is open.
	if err := file.Close(); err != nil {
		return 0, fmt.Errorf("audit: close sink for retention: %w", err)
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
	if err := replaceAuditSink(tmpPath, path); err != nil {
		return 0, fmt.Errorf("audit: replace sink after retention: %w", err)
	}
	return keepAt, nil
}

// replaceAuditSink copies the closed temporary sink into the existing sink.
// Replacing bytes in place works on Windows even when another FileStorage has
// the destination open; renaming or removing that destination does not.
func replaceAuditSink(tmpPath, path string) error {
	tmp, err := os.Open(tmpPath)
	if err != nil {
		return err
	}

	destination, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(destination, tmp); err != nil {
		_ = destination.Close()
		_ = tmp.Close()
		return err
	}
	if err := destination.Sync(); err != nil {
		_ = destination.Close()
		_ = tmp.Close()
		return err
	}
	if err := destination.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Remove(tmpPath)
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
