/**
 * Tests for the Listen/Notify Plugin core types and utilities.
 *
 * Covers: generateEventId, truncatePayload, emitListenerLog, constants.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.1
 */
import { describe, expect, it } from "bun:test";
import {
	generateEventId,
	HEALTH_CHECK_INTERVAL_MS,
	INTERRUPTIVE_QUEUE_MAX,
	INTERRUPTIVE_RATE_LIMIT_MS,
	MAX_LISTENERS_PER_MACHINE,
	MAX_LISTENERS_PER_SESSION,
	MAX_PAYLOAD_BYTES,
	NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE,
	ORPHAN_DETECTION_INTERVAL_MS,
	truncatePayload,
} from "../src/layers/listeners/types";

// ── generateEventId ──────────────────────────────────────────────────────

describe("generateEventId", () => {
	it("generates an ID with evt_ prefix", () => {
		const id = generateEventId();
		expect(id).toStartWith("evt_");
	});

	it("generates unique IDs", () => {
		const ids = new Set(Array.from({ length: 100 }, () => generateEventId()));
		expect(ids.size).toBe(100);
	});

	it("ID matches spec format evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN>", () => {
		// Per specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability
		// axiom:trace work_item=sse-event-id-format-01 spec=specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability jira_ref=SWDE-17
		const id = generateEventId();
		expect(id).toMatch(/^evt_\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z_\d{3}$/);
	});
});

// ── truncatePayload ──────────────────────────────────────────────────────

describe("truncatePayload — REQ-LNP-045", () => {
	it("returns payload unchanged when under 64KB", () => {
		const payload = { message: "hello", count: 42 };
		const result = truncatePayload(payload);
		expect(result.truncated).toBe(false);
		expect(result.payload).toEqual(payload);
	});

	it("truncates payload exceeding 64KB with [TRUNCATED] marker", () => {
		// Create a payload that exceeds 64KB
		const largePayload = { data: "x".repeat(MAX_PAYLOAD_BYTES + 1000) };
		const result = truncatePayload(largePayload);
		expect(result.truncated).toBe(true);
		// The truncated payload should contain the marker
		const serialized = JSON.stringify(result.payload);
		expect(serialized).toContain("[TRUNCATED]");
	});

	it("handles null payload", () => {
		const result = truncatePayload(null);
		expect(result.truncated).toBe(false);
		expect(result.payload).toBeNull();
	});

	it("handles string payload", () => {
		const result = truncatePayload("hello world");
		expect(result.truncated).toBe(false);
		expect(result.payload).toBe("hello world");
	});

	it("handles array payload", () => {
		const payload = [1, 2, 3];
		const result = truncatePayload(payload);
		expect(result.truncated).toBe(false);
		expect(result.payload).toEqual([1, 2, 3]);
	});
});

// ── Constants ────────────────────────────────────────────────────────────

describe("constants — REQ-LNP-053, REQ-LNP-054, REQ-LNP-073, REQ-LNP-076, REQ-LNP-077", () => {
	it("MAX_PAYLOAD_BYTES is 64KB", () => {
		expect(MAX_PAYLOAD_BYTES).toBe(64 * 1024);
	});

	it("MAX_LISTENERS_PER_SESSION is 10 (REQ-LNP-073)", () => {
		expect(MAX_LISTENERS_PER_SESSION).toBe(10);
	});

	it("MAX_LISTENERS_PER_MACHINE is 50 (REQ-LNP-073)", () => {
		expect(MAX_LISTENERS_PER_MACHINE).toBe(50);
	});

	it("INTERRUPTIVE_RATE_LIMIT_MS is 5 seconds (REQ-LNP-053)", () => {
		expect(INTERRUPTIVE_RATE_LIMIT_MS).toBe(5_000);
	});

	it("INTERRUPTIVE_QUEUE_MAX is 100 (REQ-LNP-053)", () => {
		expect(INTERRUPTIVE_QUEUE_MAX).toBe(100);
	});

	it("NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE is 10 (REQ-LNP-054)", () => {
		expect(NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE).toBe(10);
	});

	it("HEALTH_CHECK_INTERVAL_MS is 30 seconds (REQ-LNP-076)", () => {
		expect(HEALTH_CHECK_INTERVAL_MS).toBe(30_000);
	});

	it("ORPHAN_DETECTION_INTERVAL_MS is 60 seconds (REQ-LNP-077)", () => {
		expect(ORPHAN_DETECTION_INTERVAL_MS).toBe(60_000);
	});
});
