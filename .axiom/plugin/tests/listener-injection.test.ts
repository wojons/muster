/**
 * Tests for the context injection engine.
 *
 * REQ-LNP-040 through REQ-LNP-048, REQ-LNP-061, REQ-LNP-064.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#5
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-060 plan=step-fix-003
 */
import { describe, expect, it } from "bun:test";
import { createInjectionEngine } from "../src/layers/listeners/injection";
import type { EventEnvelope } from "../src/layers/listeners/types";
import { generateEventId } from "../src/layers/listeners/types";

// ── Helpers ──────────────────────────────────────────────────────────────

function makeEvent(overrides: Partial<EventEnvelope> = {}): EventEnvelope {
	return {
		id: generateEventId(),
		listener_name: "jira-completion",
		source_type: "sqs",
		source_ref: "arn:aws:sqs:us-east-1:123:test",
		received_at: "2026-04-13T10:00:00Z",
		payload: { event_type: "jira:issue_updated", issue_key: "ABC-123" },
		delivery: {
			mode: "non-interruptive",
			status: "pending",
			attempts: 0,
			last_attempt_at: null,
		},
		...overrides,
	};
}

// ── formatEventMessage ───────────────────────────────────────────────────

describe("formatEventMessage — REQ-LNP-041, REQ-LNP-042", () => {
	it("includes listener name in message", () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		const msg = engine.formatEventMessage(event);
		expect(msg).toContain("[Event: jira-completion]");
	});

	it("includes event ID in message", () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		const msg = engine.formatEventMessage(event);
		expect(msg).toContain(event.id);
	});

	it("includes received_at timestamp", () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		const msg = engine.formatEventMessage(event);
		expect(msg).toContain("2026-04-13T10:00:00Z");
	});

	it("includes source type", () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		const msg = engine.formatEventMessage(event);
		expect(msg).toContain("sqs");
	});

	it("includes payload content", () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		const msg = engine.formatEventMessage(event);
		expect(msg).toContain("ABC-123");
	});

	it("includes [TRUNCATED] marker for truncated payloads", () => {
		const engine = createInjectionEngine();
		const event = makeEvent({
			payload: "x".repeat(70 * 1024), // > 64KB
		});
		const msg = engine.formatEventMessage(event);
		expect(msg).toContain("[TRUNCATED]");
	});
});

// ── queueForInjection ────────────────────────────────────────────────────

describe("queueForInjection", () => {
	it("adds event to pending queue", () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		engine.queueForInjection(event, "session-1", "non-interruptive");
		const pending = engine.getPendingInjections();
		expect(pending).toHaveLength(1);
		expect(pending[0].event.id).toBe(event.id);
		expect(pending[0].session_id).toBe("session-1");
	});

	it("queues multiple events", () => {
		const engine = createInjectionEngine();
		const event1 = makeEvent();
		const event2 = makeEvent();
		engine.queueForInjection(event1, "session-1", "interruptive");
		engine.queueForInjection(event2, "session-1", "non-interruptive");
		expect(engine.getPendingInjections()).toHaveLength(2);
	});
});

// ── drainQueue ───────────────────────────────────────────────────────────

describe("drainQueue", () => {
	it("drains events for a specific session", () => {
		const engine = createInjectionEngine();
		const event1 = makeEvent();
		const event2 = makeEvent();
		engine.queueForInjection(event1, "session-1", "non-interruptive");
		engine.queueForInjection(event2, "session-2", "non-interruptive");

		const drained = engine.drainQueue("session-1");
		expect(drained).toHaveLength(1);
		expect(drained[0].event.id).toBe(event1.id);

		// session-2 event should still be pending
		expect(engine.getPendingInjections()).toHaveLength(1);
	});

	it("returns empty array when no events for session", () => {
		const engine = createInjectionEngine();
		const drained = engine.drainQueue("session-1");
		expect(drained).toHaveLength(0);
	});
});

// ── sanitizePayload — Rubix disabled ────────────────────────────────────

describe("sanitizePayload — REQ-LNP-044, REQ-LNP-061", () => {
	it("returns event unchanged when Rubix is disabled", async () => {
		const engine = createInjectionEngine({ rubixEnabled: false });
		const event = makeEvent();
		const sanitized = await engine.sanitizePayload(event);
		expect(sanitized.payload).toEqual(event.payload);
		expect(sanitized.rubix?.processed).toBe(false);
	});

	it("returns event unchanged when no Rubix endpoint configured", async () => {
		const engine = createInjectionEngine({ rubixEnabled: true }); // no endpoint
		const event = makeEvent();
		const sanitized = await engine.sanitizePayload(event);
		expect(sanitized.rubix?.processed).toBe(false);
	});
});

// ── injectViaHttpApi ─────────────────────────────────────────────────────

describe("injectViaHttpApi — REQ-LNP-043", () => {
	it("returns error when no OpenCode base URL configured", async () => {
		const engine = createInjectionEngine();
		const event = makeEvent();
		const result = await engine.injectViaHttpApi(event, "session-1");
		expect(result.success).toBe(false);
		expect(result.error).toContain("No OpenCode base URL");
	});
});

// ── sanitizePayload — Rubix live paths (REQ-LNP-061, REQ-LNP-064) ──────────────────

describe("sanitizePayload — Rubix live paths (REQ-LNP-061, REQ-LNP-064)", () => {
	// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-061 plan=step-fix-003

	it("proceeds fail-open when Rubix endpoint is unreachable (REQ-LNP-061)", async () => {
		const engine = createInjectionEngine({
			rubixEnabled: true,
			rubixEndpoint: "http://127.0.0.1:1", // unreachable port
			rubixTimeoutMs: 100,
		});
		const event = makeEvent();
		const sanitized = await engine.sanitizePayload(event);
		expect(sanitized.rubix?.processed).toBe(false);
		expect(sanitized.payload).toEqual(event.payload); // original payload preserved
	});

	it("flags high-risk payload and still injects (REQ-LNP-064)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-064 plan=step-fix-003
		// Use Bun.serve to create a mock Rubix server
		const mockServer = Bun.serve({
			port: 0,
			fetch() {
				return Response.json({
					payload: { safe: true },
					modified: true,
					flagged: true,
					risk_level: "high",
				});
			},
		});
		try {
			const engine = createInjectionEngine({
				rubixEnabled: true,
				rubixEndpoint: `http://127.0.0.1:${mockServer.port}`,
				rubixTimeoutMs: 1000,
			});
			const event = makeEvent();
			const sanitized = await engine.sanitizePayload(event);
			expect(sanitized.rubix?.flagged).toBe(true);
			expect(sanitized.rubix?.processed).toBe(true);
			expect(sanitized.payload).toBeDefined(); // event not dropped
		} finally {
			mockServer.stop();
		}
	});
});

// ── Batch injection limit ────────────────────────────────────────────────

describe("injection batch limit — REQ-LNP-040 (MAX_EVENTS_PER_INJECT)", () => {
	// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-040 plan=step-fix-012

	it("drainQueue returns all pending events (batch cap is enforced at hook level)", () => {
		// The injection engine itself returns all queued events.
		// The batch cap (MAX_EVENTS_PER_INJECT) is enforced in the messages.transform hook.
		// This test verifies the underlying mechanism: queue 20, drain all 20.
		const engine = createInjectionEngine();
		const events = Array.from({ length: 20 }, () => makeEvent());
		for (const e of events) {
			engine.queueForInjection(e, "session-1", "non-interruptive");
		}
		const drained = engine.drainQueue("session-1");
		expect(drained).toHaveLength(20);
		expect(engine.getPendingInjections()).toHaveLength(0);
	});

	it("re-queuing overflow events preserves them for next drain", () => {
		// Simulate what the messages.transform hook does with overflow:
		// drain all, take first MAX_EVENTS_PER_INJECT, re-queue the rest
		const engine = createInjectionEngine();
		const events = Array.from({ length: 15 }, () => makeEvent());
		for (const e of events) {
			engine.queueForInjection(e, "session-1", "non-interruptive");
		}

		const allPending = engine.drainQueue("session-1");
		const MAX = 10;
		const batch = allPending.slice(0, MAX);
		const overflow = allPending.slice(MAX);

		// Re-queue overflow
		for (const item of overflow) {
			engine.queueForInjection(item.event, "session-1", item.delivery_mode);
		}

		expect(batch).toHaveLength(10);
		expect(overflow).toHaveLength(5);
		expect(engine.getPendingInjections()).toHaveLength(5);

		// Second drain gets the remaining 5
		const secondDrain = engine.drainQueue("session-1");
		expect(secondDrain).toHaveLength(5);
	});
});
