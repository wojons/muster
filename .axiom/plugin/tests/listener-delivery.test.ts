/**
 * Tests for the delivery queue and rate limiter.
 *
 * REQ-LNP-050 through REQ-LNP-058.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#6
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.3
 */
import { describe, expect, it } from "bun:test";
import { createDeliveryQueue } from "../src/layers/listeners/delivery";
import type { EventEnvelope } from "../src/layers/listeners/types";
import { generateEventId } from "../src/layers/listeners/types";

// ── Helpers ──────────────────────────────────────────────────────────────

function makeEvent(listenerName = "test"): EventEnvelope {
	return {
		id: generateEventId(),
		listener_name: listenerName,
		source_type: "sqs",
		source_ref: "arn:aws:sqs:us-east-1:123:test",
		received_at: new Date().toISOString(),
		payload: { test: true },
		delivery: {
			mode: "non-interruptive",
			status: "pending",
			attempts: 0,
			last_attempt_at: null,
		},
	};
}

// ── Interruptive delivery ────────────────────────────────────────────────

describe("interruptive delivery — REQ-LNP-051, REQ-LNP-053", () => {
	it("delivers first interruptive event immediately", () => {
		const queue = createDeliveryQueue({ interruptiveRateLimitMs: 5000 });
		const event = makeEvent();
		const result = queue.attempt(event, "session-1", "interruptive");
		expect(result.deliver_now).toBe(true);
		expect(result.queued).toBe(false);
		expect(result.dropped).toBe(false);
	});

	it("queues second interruptive event within rate limit window", () => {
		const queue = createDeliveryQueue({ interruptiveRateLimitMs: 5000 });
		const event1 = makeEvent();
		const event2 = makeEvent();
		queue.attempt(event1, "session-1", "interruptive");
		const result = queue.attempt(event2, "session-1", "interruptive");
		expect(result.deliver_now).toBe(false);
		expect(result.queued).toBe(true);
	});

	it("delivers queued interruptive event after rate limit window", async () => {
		// Use a very short rate limit for testing — but we need to actually queue first
		// then wait for the window to pass
		const queue = createDeliveryQueue({ interruptiveRateLimitMs: 50 });
		const event1 = makeEvent();
		const event2 = makeEvent();
		// First event delivers immediately
		queue.attempt(event1, "session-1", "interruptive");
		// Second event queues (within 50ms window)
		queue.attempt(event2, "session-1", "interruptive");

		// Wait for rate limit window to pass
		await new Promise((r) => setTimeout(r, 60));

		const drained = queue.drainInterruptiveQueue("session-1");
		expect(drained).toHaveLength(1);
		expect(drained[0].event.id).toBe(event2.id);
	});

	it("drops oldest events when queue overflows (REQ-LNP-053)", () => {
		const queue = createDeliveryQueue({
			interruptiveRateLimitMs: 60_000, // long window so everything queues
			interruptiveQueueMax: 3,
		});

		// First event delivers immediately
		const event0 = makeEvent();
		queue.attempt(event0, "session-1", "interruptive");

		// Next 3 fill the queue
		const events = Array.from({ length: 4 }, () => makeEvent());
		for (const e of events) {
			queue.attempt(e, "session-1", "interruptive");
		}

		// Queue should have max 3 items (oldest dropped)
		const depth = queue.getQueueDepth("session-1");
		expect(depth.interruptive).toBe(3);
	});

	it("tracks queue depth correctly", () => {
		const queue = createDeliveryQueue({ interruptiveRateLimitMs: 60_000 });
		const event1 = makeEvent();
		const event2 = makeEvent();
		queue.attempt(event1, "session-1", "interruptive");
		queue.attempt(event2, "session-1", "interruptive");
		const depth = queue.getQueueDepth("session-1");
		expect(depth.interruptive).toBe(1); // event2 queued; event1 delivered
	});
});

// ── Non-interruptive delivery ────────────────────────────────────────────

describe("non-interruptive delivery — REQ-LNP-052, REQ-LNP-054", () => {
	it("queues non-interruptive events for later delivery", () => {
		const queue = createDeliveryQueue();
		const event = makeEvent();
		const result = queue.attempt(event, "session-1", "non-interruptive");
		expect(result.deliver_now).toBe(false);
		expect(result.queued).toBe(true);
		expect(result.dropped).toBe(false);
	});

	it("drains non-interruptive queue", () => {
		const queue = createDeliveryQueue();
		const event1 = makeEvent();
		const event2 = makeEvent();
		queue.attempt(event1, "session-1", "non-interruptive");
		queue.attempt(event2, "session-1", "non-interruptive");
		const drained = queue.drainNonInterruptiveQueue("session-1");
		expect(drained).toHaveLength(2);
	});

	it("drops events when non-interruptive rate limit exceeded (REQ-LNP-054)", () => {
		const queue = createDeliveryQueue({ nonInterruptiveRateLimitPerMinute: 3 });
		const events = Array.from({ length: 5 }, () => makeEvent());
		const results = events.map((e) =>
			queue.attempt(e, "session-1", "non-interruptive"),
		);

		const queued = results.filter((r) => r.queued).length;
		const dropped = results.filter((r) => r.dropped).length;
		expect(queued).toBe(3);
		expect(dropped).toBe(2);
	});

	it("drains only events for the specified session", () => {
		const queue = createDeliveryQueue();
		const event1 = makeEvent();
		const event2 = makeEvent();
		queue.attempt(event1, "session-1", "non-interruptive");
		queue.attempt(event2, "session-2", "non-interruptive");
		const drained = queue.drainNonInterruptiveQueue("session-1");
		expect(drained).toHaveLength(1);
		expect(drained[0].event.id).toBe(event1.id);
	});

	it("drops oldest non-interruptive events when queue depth cap exceeded (REQ-LNP-054)", () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-054 plan=step-fix-005
		const queue = createDeliveryQueue({
			nonInterruptiveRateLimitPerMinute: 1000, // high limit so rate doesn't interfere
			nonInterruptiveQueueMax: 3,
		});

		// Queue 5 events
		const events = Array.from({ length: 5 }, () => makeEvent());
		for (const e of events) {
			queue.attempt(e, "session-1", "non-interruptive");
		}

		// Queue should have max 3 items (oldest 2 dropped)
		const depth = queue.getQueueDepth("session-1");
		expect(depth.non_interruptive).toBe(3);

		// The 3 remaining should be the NEWEST events (oldest dropped)
		const drained = queue.drainNonInterruptiveQueue("session-1");
		expect(drained).toHaveLength(3);
		expect(drained[0].event.id).toBe(events[2].id);
		expect(drained[1].event.id).toBe(events[3].id);
		expect(drained[2].event.id).toBe(events[4].id);
	});
});

// ── Session cleanup ──────────────────────────────────────────────────────

describe("clearSession", () => {
	it("clears all state for a session", () => {
		const queue = createDeliveryQueue({ interruptiveRateLimitMs: 60_000 });
		const event1 = makeEvent();
		const event2 = makeEvent();
		queue.attempt(event1, "session-1", "interruptive");
		queue.attempt(event2, "session-1", "non-interruptive");

		queue.clearSession("session-1");

		const depth = queue.getQueueDepth("session-1");
		expect(depth.interruptive).toBe(0);
		expect(depth.non_interruptive).toBe(0);
	});
});
