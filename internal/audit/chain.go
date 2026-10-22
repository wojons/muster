package audit

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Hash algorithms the sink can use for its chain. sha256 is the documented
// default (AuditConfig.HashAlgorithm); sha512 is the other value the spec
// validates, and an unknown value falls back to sha256 rather than writing an
// unverifiable record.
const (
	HashAlgorithmSHA256 = "sha256"
	HashAlgorithmSHA512 = "sha512"
)

// hashFieldSeparator keeps the hash preimage unambiguous: a NUL byte cannot
// appear in the event_type or previous_hash values, so
// ("a", "bc") and ("ab", "c") can never collide on the concatenation.
const hashFieldSeparator = "\x00"

// maxSinkTailBytes bounds how much of the sink is read when re-seeding the
// chain from the last persisted record.
const maxSinkTailBytes = 1 << 20

// normalizeHashAlgorithm returns algo when it is implemented, else sha256.
func normalizeHashAlgorithm(algo string) string {
	switch strings.ToLower(strings.TrimSpace(algo)) {
	case HashAlgorithmSHA512:
		return HashAlgorithmSHA512
	default:
		return HashAlgorithmSHA256
	}
}

// digest returns the hex digest of preimage under algo.
func digest(algo, preimage string) string {
	if normalizeHashAlgorithm(algo) == HashAlgorithmSHA512 {
		sum := sha512.Sum512([]byte(preimage))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(sum[:])
}

// newEventID returns a random 128-bit event identifier in hex.
func newEventID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// A broken entropy source must not lose the audit record; a
		// timestamp-derived id keeps the event writable and unique enough.
		return fmt.Sprintf("evt-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

// eventHashPreimage builds the canonical string a record's hash covers.
//
// It is the NUL-joined triple the chain link is defined on — event_type,
// timestamp, previous_hash — followed by the record's full JSON body with the
// two hash fields cleared.
//
// Covering the body as well as the link is what makes the store tamper-EVIDENT
// rather than merely tamper-linked: with the triple alone, editing an event's
// action, actor or details would leave its hash matching, so a rewritten record
// would still verify. Because Redaction runs before hashing (see prepareEvent),
// the body digested here is exactly the body that reaches the disk.
func eventHashPreimage(event *AuditEvent) string {
	link := strings.Join([]string{
		event.EventType,
		event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.PreviousHash,
	}, hashFieldSeparator)

	body := *event
	body.Hash = ""
	body.PreviousHash = ""
	body.Timestamp = body.Timestamp.UTC()
	encoded, err := json.Marshal(body)
	if err != nil {
		// Marshalling a plain struct cannot fail for our field types; if it ever
		// does, the link fields alone still give a deterministic digest.
		return link
	}
	return link + hashFieldSeparator + string(encoded)
}

// computeEventHash returns the hex hash of the event's canonical preimage.
func computeEventHash(algo string, event *AuditEvent) string {
	return digest(algo, eventHashPreimage(event))
}

// lastSinkHash returns the Hash of the last record persisted in path, or "" when
// the sink is missing, empty, unparsable, or its last record predates the hash
// chain. It deliberately reports no error: an unreadable sink must not block
// writing a new audit record, and a chain that restarts from "" is still
// verifiable from that point forward.
func lastSinkHash(path string) string {
	line := lastNonBlankLine(path)
	if line == "" {
		return ""
	}
	var event AuditEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return ""
	}
	return event.Hash
}

// lastNonBlankLine returns the last non-empty line of path, reading at most
// maxSinkTailBytes from the end of the file.
func lastNonBlankLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return ""
	}
	read := info.Size()
	if read > maxSinkTailBytes {
		read = maxSinkTailBytes
	}
	buf := make([]byte, read)
	n, err := f.ReadAt(buf, info.Size()-read)
	if n == 0 || (err != nil && n == 0) {
		return ""
	}
	lines := strings.Split(string(buf[:n]), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// VerifyChain walks the audit sink at path and returns the first integrity
// failure it finds, or nil when every record is intact and correctly linked to
// its predecessor. A missing sink verifies trivially (nothing was written).
//
// Records written before the hash chain existed carry no hash and are reported
// as unverifiable rather than skipped: silently accepting an unhashed record
// would let a tamperer blank a hash to escape detection.
func VerifyChain(path string) error {
	return VerifyChainWithAlgorithm(path, HashAlgorithmSHA256)
}

// VerifyChainWithAlgorithm verifies the sink at path using algo.
func VerifyChainWithAlgorithm(path, algo string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("audit: open sink %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	previous := ""
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNo++

		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("audit: sink %s line %d: not a JSON audit record: %w", path, lineNo, err)
		}
		if event.Hash == "" {
			return fmt.Errorf("audit: sink %s line %d: record %q carries no hash (written before the chain was enabled); integrity cannot be verified", path, lineNo, event.ID)
		}
		if want := computeEventHash(algo, &event); event.Hash != want {
			return fmt.Errorf("audit: sink %s line %d: hash mismatch (stored %s, recomputed %s) — record modified after it was written", path, lineNo, event.Hash, want)
		}
		if event.PreviousHash != previous {
			return fmt.Errorf("audit: sink %s line %d: chain break (previous_hash %q, want %q) — a record was removed, inserted or reordered", path, lineNo, event.PreviousHash, previous)
		}
		previous = event.Hash
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("audit: read sink %s: %w", path, err)
	}
	return nil
}
