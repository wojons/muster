/**
 * Conformance tests for SSE event ID format in the Listen/Notify Plugin.
 *
 * Spec: specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability
 * Required format: evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN>
 * Example: evt_2026-02-06T10-31-00-123Z_001
 *
 * axiom:trace work_item=sse-event-id-format-01 spec=specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability plan=phase-1/task-1/step-1 test=.axiom/plugin/src/layers/listeners/types.test.ts jira_ref=SWDE-17
 */

import { describe, it, expect } from "bun:test";
import { generateEventId } from "./types";

// Canonical spec regex: evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN>
// e.g. evt_2026-02-06T10-31-00-123Z_001
const SPEC_PATTERN = /^evt_\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z_\d{3}$/;

describe("generateEventId — spec/30 SSE event ID format conformance", () => {
	// ── Format ────────────────────────────────────────────────────────────

	it("produces the evt_ prefix", () => {
		const id = generateEventId();
		expect(id.startsWith("evt_")).toBe(true);
	});

	it("matches the canonical spec pattern evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN>", () => {
		const id = generateEventId();
		expect(SPEC_PATTERN.test(id)).toBe(true);
	});

	it("timestamp part uses hyphens instead of colons and dots (URL-safe)", () => {
		const id = generateEventId();
		// Strip prefix and suffix: "evt_" (4) and "_NNN" (4)
		const timestampPart = id.slice(4, -4);
		expect(timestampPart).not.toContain(":");
		expect(timestampPart).not.toContain(".");
	});

	it("sequence number is zero-padded to exactly 3 digits", () => {
		// The suffix after the last underscore must be exactly 3 digits
		const id = generateEventId();
		const suffix = id.split("_").at(-1)!;
		expect(suffix).toMatch(/^\d{3}$/);
	});

	it("timestamp part has the YYYY-MM-DDTHH-MM-SS-mmmZ shape", () => {
		const id = generateEventId();
		// e.g. evt_2026-02-06T10-31-00-123Z_001
		// timestamp = everything between "evt_" and the last "_NNN"
		const withoutPrefix = id.slice(4); // "2026-02-06T10-31-00-123Z_001"
		const lastUnderscore = withoutPrefix.lastIndexOf("_");
		const timestampPart = withoutPrefix.slice(0, lastUnderscore);
		// Must match YYYY-MM-DDTHH-MM-SS-mmmZ
		expect(timestampPart).toMatch(
			/^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z$/,
		);
	});

	it("timestamp encodes the current UTC time (within 5 seconds)", () => {
		const before = Date.now();
		const id = generateEventId();
		const after = Date.now();

		// Extract timestamp from id: evt_2026-02-06T10-31-00-123Z_001
		// → "2026-02-06T10-31-00-123Z"
		const withoutPrefix = id.slice(4);
		const lastUnderscore = withoutPrefix.lastIndexOf("_");
		const safeTs = withoutPrefix.slice(0, lastUnderscore);
		// Convert back: replace hyphens in time portion with colons/dots
		// "2026-02-06T10-31-00-123Z" → "2026-02-06T10:31:00.123Z"
		const isoTs = safeTs
			.replace(/T(\d{2})-(\d{2})-(\d{2})-(\d{3})Z$/, "T$1:$2:$3.$4Z");
		const parsed = new Date(isoTs).getTime();

		expect(parsed).toBeGreaterThanOrEqual(before - 1000);
		expect(parsed).toBeLessThanOrEqual(after + 1000);
	});

	// ── Uniqueness ────────────────────────────────────────────────────────

	it("generates unique IDs across 100 rapid calls", () => {
		const ids = new Set<string>();
		for (let i = 0; i < 100; i++) {
			ids.add(generateEventId());
		}
		expect(ids.size).toBe(100);
	});

	// axiom:trace work_item=sse-event-id-format-01 spec=specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability plan=phase-1/task-1/step-INJ-CRIT-1 jira_ref=SWDE-17
	it("generates 999 unique IDs within the same millisecond (full sequence cycle without collision)", () => {
		const originalNow = Date.now;
		const fixedMs = 1738843860123; // 2025-02-06T12:11:00.123Z
		Date.now = () => fixedMs;
		try {
			const ids = new Set<string>();
			for (let i = 0; i < 999; i++) {
				ids.add(generateEventId());
			}
			expect(ids.size).toBe(999);
			// Verify all IDs have the same timestamp
			for (const id of ids) {
				expect(id).toStartWith("evt_2025-02-06T12-11-00-123Z_");
			}
		} finally {
			Date.now = originalNow;
		}
	});

	// axiom:trace work_item=sse-event-id-format-01 spec=specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability plan=phase-1/task-1/step-INJ-CRIT-1 jira_ref=SWDE-17
	it("sequence resets to 001 when timestamp changes (per-millisecond counter)", () => {
		let callCount = 0;
		const originalNow = Date.now;
		// First call: ms=1000, second call: ms=2000 (different timestamp)
		Date.now = () => callCount++ === 0 ? 1000 : 2000;
		try {
			const id1 = generateEventId(); // ts=1970-01-01T00-00-01-000Z_001
			const id2 = generateEventId(); // ts=1970-01-01T00-00-02-000Z_001 (reset!)
			const seq1 = id1.split("_").at(-1)!;
			const seq2 = id2.split("_").at(-1)!;
			expect(seq1).toBe("001");
			expect(seq2).toBe("001"); // resets on new timestamp
		} finally {
			Date.now = originalNow;
		}
	});

	// ── Monotonic sequence ────────────────────────────────────────────────

	it("sequence numbers are monotonically increasing across calls", () => {
		const ids: string[] = [];
		for (let i = 0; i < 10; i++) {
			ids.push(generateEventId());
		}
		const seqs = ids.map((id) => {
			const suffix = id.split("_").at(-1)!;
			return parseInt(suffix, 10);
		});
		for (let i = 1; i < seqs.length; i++) {
			expect(seqs[i]).toBeGreaterThan(seqs[i - 1]);
		}
	});

	// ── Last-Event-ID resumability ────────────────────────────────────────

	it("generated IDs are valid as Last-Event-ID header values (no whitespace, URL-safe)", () => {
		const id = generateEventId();
		// Must not contain whitespace
		expect(id).not.toMatch(/\s/);
		// Must not contain characters that break HTTP headers
		expect(id).not.toMatch(/[^\x21-\x7E]/);
	});

	it("two IDs generated at the same millisecond differ by sequence", () => {
		// Force same-millisecond by mocking Date.now — use a fixed timestamp
		const originalNow = Date.now;
		const fixedMs = 1738843860123; // 2026-02-06T10:31:00.123Z
		Date.now = () => fixedMs;
		try {
			const id1 = generateEventId();
			const id2 = generateEventId();
			// Both should have the same timestamp part but different sequences
			const ts1 = id1.slice(4, id1.lastIndexOf("_"));
			const ts2 = id2.slice(4, id2.lastIndexOf("_"));
			expect(ts1).toBe(ts2);
			const seq1 = parseInt(id1.split("_").at(-1)!, 10);
			const seq2 = parseInt(id2.split("_").at(-1)!, 10);
			expect(seq2).toBeGreaterThan(seq1);
		} finally {
			Date.now = originalNow;
		}
	});
});
