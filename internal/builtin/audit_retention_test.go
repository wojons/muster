package builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/internal/audit"
)

// countingAuditSink wraps the audit sink and counts every byte the bounded
// tail reader pulls off it (INT-CI-016). Bytes read is the deterministic
// stand-in for "the read is bounded by --limit, not by the sink size".
type countingAuditSink struct {
	inner auditTailSink
	read  int64
}

func (c *countingAuditSink) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.inner.ReadAt(p, off)
	c.read += int64(n)
	return n, err
}

func (c *countingAuditSink) Stat() (os.FileInfo, error) { return c.inner.Stat() }

func (c *countingAuditSink) Close() error { return c.inner.Close() }

// TestAuditCommand_InfoRetentionMatchesEnforcement is the DOC-15 (a) contract:
// the value `audit info` prints must be the value the retention mechanism
// actually enforces, and it must agree with specs/EVENTS-AUDIT.md's documented
// default. Before this change `info` printed 90 days while the spec documented
// 365 and nothing enforced either number.
func TestAuditCommand_InfoRetentionMatchesEnforcement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"info"})
		return cmd.Execute()
	})
	require.NoError(t, err)

	line := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Retention:") {
			line = strings.TrimSpace(l)
			break
		}
	}
	require.NotEmpty(t, line, "audit info must report Retention, got:\n%s", out)

	var days int
	_, err = fmt.Sscanf(line, "Retention: %d days", &days)
	require.NoError(t, err, "unparsable retention line %q", line)
	assert.Equal(t, 365, days, "info must report the documented default (365 days)")

	// The same number is what the enforcement mechanism uses: the storage
	// options the sink is opened with, and the config the hook builds.
	assert.Equal(t, days, audit.DefaultFileStorageOptions().RetentionDays,
		"audit info value must equal the RetentionDays the file sink enforces")
	assert.Equal(t, days, audit.DefaultConfig().RetentionDays,
		"audit info value must equal the config-driven enforcement value")

	// The spec is the design authority: its documented default must match.
	spec, readErr := os.ReadFile(filepath.Join("..", "..", "specs", "EVENTS-AUDIT.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(spec), fmt.Sprintf("RetentionDays: %d", days),
		"specs/EVENTS-AUDIT.md must document the enforced default")
}

// TestAuditCommand_PruneRotatesExpiredRows proves the documented retention verb
// actually removes expired rows (non-vacuously): a sink seeded with rows older
// than the window loses exactly those rows, keeps the fresh ones, writes a
// verifiable archive segment, and leaves the active log's chain valid.
func TestAuditCommand_PruneRotatesExpiredRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	now := time.Now().UTC()

	var seeded []audit.AuditEvent
	for i := 0; i < 3; i++ {
		e := testAuditEvent("expired.event", fmt.Sprintf("expired-%d", i))
		e.Timestamp = now.AddDate(0, 0, -400).Add(-time.Duration(i) * time.Minute)
		seeded = append(seeded, e)
	}
	for i := 0; i < 2; i++ {
		e := testAuditEvent("fresh.event", fmt.Sprintf("fresh-%d", i))
		e.Timestamp = now.Add(-time.Duration(i) * time.Minute)
		seeded = append(seeded, e)
	}
	writeAuditLog(t, path, seeded...)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"prune"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Pruned 3", "prune must report the rows it removed, got:\n%s", out)

	remaining, err := readAuditEvents(path, 0)
	require.NoError(t, err)
	require.Len(t, remaining, 2, "exactly the expired rows must be removed")
	for _, e := range remaining {
		assert.Equal(t, "fresh.event", e.EventType, "a retained row must be inside the window")
	}
	assert.NoError(t, audit.VerifyChain(path), "retained chain must stay valid after rotation")

	archives, err := filepath.Glob(path + ".archive-*.jsonl")
	require.NoError(t, err)
	require.Len(t, archives, 1, "the expired prefix must be rotated, not silently dropped")
	archived, err := readAuditEvents(archives[0], 0)
	require.NoError(t, err)
	assert.Len(t, archived, 3)
	for _, e := range archived {
		assert.Equal(t, "expired.event", e.EventType)
	}
	assert.NoError(t, audit.VerifyChain(archives[0]), "the archive segment must be independently verifiable")

	// Idempotent: a second run inside the window removes nothing.
	out2, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"prune"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out2, "Pruned 0")
}

// TestAuditCommand_PruneDryRunLeavesSinkUntouched covers the operator-facing
// dry run: it reports the expired count and mutates nothing.
func TestAuditCommand_PruneDryRunLeavesSinkUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	old := testAuditEvent("expired.event", "expired-0")
	old.Timestamp = time.Now().UTC().AddDate(0, 0, -400)
	writeHashedAuditLog(t, path, old)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"prune", "--dry-run"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Would prune 1 of 1")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "dry-run must not modify the sink")
	archives, _ := filepath.Glob(path + ".archive-*.jsonl")
	assert.Empty(t, archives)
}

// TestAuditCommand_LogLimitNewestOnLargeSink pins the newest-row selection that
// the bounded reader must preserve: the tail read has to return the LAST rows
// on disk, in newest-first order, not the first ones it happens to see.
func TestAuditCommand_LogLimitNewestOnLargeSink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	const rows = 4000
	events := make([]audit.AuditEvent, 0, rows)
	for i := 0; i < rows; i++ {
		e := testAuditEvent("bench.event", fmt.Sprintf("actor-%d", i))
		e.Timestamp = base.Add(time.Duration(i) * time.Minute)
		events = append(events, e)
	}
	writeAuditLog(t, path, events...)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log", "--limit", "1"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	lines := nonEmptyLines(out)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], fmt.Sprintf("actor-%d", rows-1),
		"--limit 1 must return the newest row on disk")
	assert.NotContains(t, lines[0], "actor-0")

	// A limit larger than the file's tail block still returns the newest rows
	// in newest-first order.
	out5, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log", "--limit", "150"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	got := nonEmptyLines(out5)
	require.Len(t, got, 150)
	for i, line := range got {
		assert.Contains(t, line, fmt.Sprintf("actor-%d", rows-1-i),
			"row %d must be the #%d-newest event", i, i+1)
	}
}

// TestAuditCommand_LogLimitStaysBounded is the DOC-15 (c) acceptance check:
// `audit log --limit 1` on a 10,000-row sink must run within 2x the time it
// takes on a 200-row sink. Before the fix the reader read and unmarshalled the
// entire file regardless of --limit.
func TestAuditCommand_LogLimitStaysBounded(t *testing.T) {
	tmp := t.TempDir()
	small := filepath.Join(tmp, "small.jsonl")
	large := filepath.Join(tmp, "large.jsonl")
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	seed := func(path string, rows int) {
		events := make([]audit.AuditEvent, 0, rows)
		for i := 0; i < rows; i++ {
			e := testAuditEvent("bench.event", fmt.Sprintf("actor-%d", i))
			e.Timestamp = base.Add(time.Duration(i) * time.Minute)
			events = append(events, e)
		}
		writeAuditLog(t, path, events...)
	}
	seed(small, 200)
	seed(large, 10000)

	// INT-CI-016: this used to compare wall-clock durations
	// (largeDur <= 2*smallDur and largeDur < 250ms). That measured how busy
	// the runner was, not whether the read is bounded: on macOS CI, 200 rows
	// took 115.6us and 10000 rows 131.8us — a 14% difference from pure
	// jitter — and failed the run. Boundedness is asserted below on the
	// quantity that actually defines it: bytes pulled off the sink.
	readBytes := func(t *testing.T, path, wantNewest string) int64 {
		t.Helper()
		sink := &countingAuditSink{}
		prev := auditLogOpener
		auditLogOpener = func(p string) (auditTailSink, error) {
			f, err := os.Open(p)
			if err != nil {
				return nil, err
			}
			sink.inner = f
			return sink, nil
		}
		t.Cleanup(func() { auditLogOpener = prev })
		t.Setenv("MUSTER_AUDIT_LOG", path)

		out, err := captureStdout(func() error {
			cmd := newAuditCommand()
			cmd.SetArgs([]string{"log", "--limit", "1"})
			return cmd.Execute()
		})
		require.NoError(t, err)
		// Newest row, not the oldest: proves the tail was actually read.
		require.Contains(t, out, wantNewest)
		require.NotContains(t, out, "	actor-0\n")
		return sink.read
	}

	largeInfo, err := os.Stat(large)
	require.NoError(t, err)
	require.Greater(t, largeInfo.Size(), int64(20*64*1024),
		"large fixture must dwarf one tail block for this bound to mean anything")

	smallBytes := readBytes(t, small, "actor-199")
	largeBytes := readBytes(t, large, "actor-9999")

	// One 64KiB tail block is all a single newest row may cost.
	const tailBlock = int64(64 * 1024)
	assert.LessOrEqual(t, largeBytes, tailBlock,
		"audit log --limit 1 must read at most one tail block: %d rows=%d bytes, %d rows=%d bytes",
		200, smallBytes, 10000, largeBytes)
	// And the cost must be independent of the sink: reading the tail of a
	// 2MB+ log cannot pull the file into memory.
	assert.Less(t, largeBytes, largeInfo.Size()/10,
		"bounded read must not scale with sink size: read %d of %d bytes",
		largeBytes, largeInfo.Size())
}

// TestReadNewestAuditEvents_AllocationsBounded is the non-flaky structural
// half of the bounded-read proof: reading one row out of a multi-megabyte sink
// must not allocate anything proportional to the file size.
func TestReadNewestAuditEvents_AllocationsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	events := make([]audit.AuditEvent, 0, 10000)
	for i := 0; i < 10000; i++ {
		e := testAuditEvent("bench.event", fmt.Sprintf("actor-%d", i))
		e.Timestamp = base.Add(time.Duration(i) * time.Minute)
		events = append(events, e)
	}
	writeAuditLog(t, path, events...)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(1<<20), "premise: sink must be multi-megabyte")

	measureAlloc := func(read func() []audit.AuditEvent) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		got := read()
		runtime.ReadMemStats(&after)
		require.NotEmpty(t, got)
		return after.TotalAlloc - before.TotalAlloc
	}

	bounded := measureAlloc(func() []audit.AuditEvent {
		got, err := readNewestAuditEvents(path, 1)
		require.NoError(t, err)
		return got
	})
	whole := measureAlloc(func() []audit.AuditEvent {
		got, err := readAuditEvents(path, 0)
		require.NoError(t, err)
		return got
	})

	// The whole-file read allocates at least the file size; the bounded read
	// must stay far below it (one 64 KiB tail block plus one decoded record).
	assert.GreaterOrEqual(t, whole, uint64(info.Size()),
		"premise: the unbounded reader allocates at least the file size")
	assert.Less(t, bounded, uint64(info.Size())/2,
		"bounded read allocated %d bytes for a %d-byte sink: it is still reading the whole file",
		bounded, info.Size())
}
