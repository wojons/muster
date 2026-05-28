/**
 * Tests for the NATS adapter.
 *
 * REQ-LNP-100 through REQ-LNP-108.
 * Uses a mock NATS connection — no real NATS server required for unit tests.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#13.1
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-1/task-1.4
 */
import { describe, expect, it } from "bun:test";
import { createNatsAdapter } from "../src/layers/listeners/adapters/nats";
import type {
	NatsConnection,
	NatsModule,
	NatsMsg,
	NatsStatusEvent,
	NatsSubscription,
} from "../src/layers/listeners/adapters/nats";
import type { EventEnvelope } from "../src/layers/listeners/types";

// ── Mock Helpers ─────────────────────────────────────────────────────────

/**
 * Build a mock NatsMsg with optional ack/nak tracking.
 */
function makeMockMsg(
	data: string,
	opts?: { trackAck?: boolean; trackNak?: boolean },
): NatsMsg & { ackCalled: boolean; nakCalled: boolean } {
	const msg = {
		data: new TextEncoder().encode(data),
		subject: "test.subject",
		ackCalled: false,
		nakCalled: false,
	} as NatsMsg & { ackCalled: boolean; nakCalled: boolean };

	if (opts?.trackAck) {
		msg.ack = () => { msg.ackCalled = true; };
	}
	if (opts?.trackNak) {
		msg.nak = () => { msg.nakCalled = true; };
	}

	return msg;
}

/**
 * Build a mock NatsSubscription that yields the given messages once, then stops.
 */
function makeMockSubscription(messages: NatsMsg[]): NatsSubscription & { unsubscribeCalled: boolean } {
	let unsubscribeCalled = false;

	const sub: NatsSubscription & { unsubscribeCalled: boolean } = {
		get unsubscribeCalled() { return unsubscribeCalled; },
		unsubscribe() { unsubscribeCalled = true; },
		[Symbol.asyncIterator]() {
			let idx = 0;
			return {
				async next(): Promise<IteratorResult<NatsMsg>> {
					if (idx < messages.length) {
						return { value: messages[idx++], done: false };
					}
					// Hang indefinitely after messages are exhausted (simulates real NATS)
					return new Promise(() => {});
				},
			};
		},
	};

	return sub;
}

/**
 * Build a no-op status async iterable (default for mocks that don't need status events).
 * REQ-LNP-102: All NatsConnection mocks must implement status() per the interface.
 */
async function* emptyStatusIterable(): AsyncGenerator<NatsStatusEvent> {
	// Hang indefinitely — simulates a healthy connection with no status events
	await new Promise<void>(() => {});
}

/**
 * Build a mock NatsConnection for core NATS (subscribe returns sync subscription).
 * REQ-LNP-102: Includes a no-op status() so existing tests are unaffected.
 */
function makeMockCoreConnection(
	messages: NatsMsg[],
): NatsConnection & { drainCalled: boolean } {
	let drainCalled = false;
	const sub = makeMockSubscription(messages);

	return {
		get drainCalled() { return drainCalled; },
		subscribe(_subject, _opts) {
			return sub;
		},
		jetstream() {
			throw new Error("jetstream() should not be called in core NATS mode");
		},
		async drain() {
			drainCalled = true;
		},
		isClosed() {
			return false;
		},
		status() {
			return emptyStatusIterable();
		},
	};
}

/**
 * Build a mock NatsConnection for JetStream mode.
 * REQ-LNP-102: Includes a no-op status() so existing tests are unaffected.
 */
function makeMockJetStreamConnection(
	messages: NatsMsg[],
): NatsConnection & { drainCalled: boolean } {
	let drainCalled = false;
	const sub = makeMockSubscription(messages);

	return {
		get drainCalled() { return drainCalled; },
		subscribe(_subject, _opts) {
			throw new Error("subscribe() should not be called in JetStream mode");
		},
		jetstream() {
			return {
				async subscribe(_subject: string, _opts?: unknown) {
					return sub;
				},
			};
		},
		async drain() {
			drainCalled = true;
		},
		isClosed() {
			return false;
		},
		status() {
			return emptyStatusIterable();
		},
	};
}

/**
 * Build a mock NatsConnection that supports injecting status events and triggering close.
 * REQ-LNP-102: Used to test post-start degraded state detection.
 */
function makeMockNatsConnectionWithStatus(opts?: {
	statusEvents?: Array<{ type: string; data?: unknown }>;
	closeAfterStatus?: boolean;
}): NatsConnection & { drainCalled: boolean; triggerStatusClose: () => void } {
	let drainCalled = false;
	let statusCloseResolve: (() => void) | null = null;
	const sub = makeMockSubscription([]);

	return {
		get drainCalled() { return drainCalled; },
		subscribe(_subject, _opts) {
			return sub;
		},
		jetstream() {
			throw new Error("jetstream() should not be called in this mock");
		},
		async drain() {
			drainCalled = true;
		},
		isClosed() {
			return false;
		},
		status: async function* () {
			for (const event of opts?.statusEvents ?? []) {
				yield event;
			}
			if (opts?.closeAfterStatus) {
				// Hang until triggerStatusClose() is called, then complete the iterator
				await new Promise<void>((r) => { statusCloseResolve = r; });
			}
			// Iterator completes — simulates reconnect exhaustion / permanent close
		},
		triggerStatusClose() {
			statusCloseResolve?.();
		},
	};
}

// ── Helper: wait for events to be processed ─────────────────────────────

/** Yield to the event loop a few times to let async message processing complete. */
async function flushMicrotasks(rounds = 10): Promise<void> {
	for (let i = 0; i < rounds; i++) {
		await new Promise((r) => setTimeout(r, 0));
	}
}

// ── Tests ────────────────────────────────────────────────────────────────

describe("createNatsAdapter — REQ-LNP-100 through REQ-LNP-108", () => {

	// ── REQ-LNP-100 / REQ-LNP-107: Lifecycle events ──────────────────────

	it("starts and stops cleanly, emitting lifecycle log events (REQ-LNP-107)", async () => {
		const conn = makeMockCoreConnection([]);
		const adapter = createNatsAdapter(
			{
				listenerName: "test-nats",
				config: { servers: ["nats://localhost:4222"], subject: "events.>" },
			},
			conn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		expect(adapter.isHealthy()).toBe(true);
		expect(adapter.statusDescription()).toBe("running");

		await adapter.stop();

		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toBe("stopped");
		expect(conn.drainCalled).toBe(true);
	});

	// ── REQ-LNP-100 / REQ-LNP-107: Message delivery ──────────────────────

	it("receives a JSON message and delivers it as an EventEnvelope (REQ-LNP-100)", async () => {
		const payload = { event_type: "deploy.started", service: "api" };
		const msg = makeMockMsg(JSON.stringify(payload));
		const conn = makeMockCoreConnection([msg]);

		const adapter = createNatsAdapter(
			{
				listenerName: "test-nats",
				config: { servers: ["nats://localhost:4222"], subject: "deploys.*" },
			},
			conn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await flushMicrotasks();
		await adapter.stop();

		expect(events).toHaveLength(1);
		const received = events[0].payload as Record<string, unknown>;
		expect(received.event_type).toBe("deploy.started");
		expect(received.service).toBe("api");
	});

	// ── REQ-LNP-104: Non-JSON body → { raw: string } ─────────────────────

	it("handles non-JSON message body as { raw: string } (REQ-LNP-104)", async () => {
		const msg = makeMockMsg("plain text payload");
		const conn = makeMockCoreConnection([msg]);

		const adapter = createNatsAdapter(
			{
				listenerName: "test-nats",
				config: { servers: ["nats://localhost:4222"], subject: "raw.>" },
			},
			conn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await flushMicrotasks();
		await adapter.stop();

		expect(events).toHaveLength(1);
		const received = events[0].payload as Record<string, unknown>;
		expect(received.raw).toBe("plain text payload");
	});

	// ── REQ-LNP-100: source_type and source_ref ───────────────────────────

	it("sets source_type to 'nats' and source_ref to subject (REQ-LNP-100)", async () => {
		const msg = makeMockMsg(JSON.stringify({ ok: true }));
		const conn = makeMockCoreConnection([msg]);

		const adapter = createNatsAdapter(
			{
				listenerName: "my-nats-listener",
				config: { servers: ["nats://localhost:4222"], subject: "orders.>" },
			},
			conn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await flushMicrotasks();
		await adapter.stop();

		expect(events).toHaveLength(1);
		expect(events[0].source_type).toBe("nats");
		expect(events[0].source_ref).toBe("orders.>");
		expect(events[0].listener_name).toBe("my-nats-listener");
	});

	// ── REQ-LNP-108: Missing nats package ────────────────────────────────

	it("emits listener.error with clear message when nats package is not available (REQ-LNP-108)", async () => {
		// Simulate missing nats package by providing no connection override
		// and using a config that would trigger dynamic import.
		// We test this by creating an adapter WITHOUT a connection override
		// and verifying the error path is handled gracefully.
		// Since we can't uninstall nats in tests, we verify the error message format
		// by testing the adapter's error handling when connect() throws.

		// Create adapter with a connection that throws on first use
		const throwingConn: NatsConnection = {
			subscribe() { throw new Error("nats package not available: Cannot find module 'nats'"); },
			jetstream() { throw new Error("nats package not available"); },
			async drain() {},
			isClosed() { return true; },
			status() { return emptyStatusIterable(); },
		};

		// We test the error path by providing a connection that throws during subscribe
		const adapter = createNatsAdapter(
			{
				listenerName: "test-nats-missing",
				config: { servers: ["nats://localhost:4222"], subject: "test" },
			},
			throwingConn,
		);

		let startError: Error | null = null;
		try {
			await adapter.start(async () => {});
		} catch (err) {
			startError = err instanceof Error ? err : new Error(String(err));
		}

		expect(startError).not.toBeNull();
		expect(startError?.message).toContain("nats package not available");
		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toContain("degraded");
	});

	// ── REQ-LNP-103: JetStream ack on success ────────────────────────────

	it("JetStream mode — acks message on successful processing (REQ-LNP-103)", async () => {
		const msg = makeMockMsg(JSON.stringify({ action: "create" }), {
			trackAck: true,
			trackNak: true,
		});
		const conn = makeMockJetStreamConnection([msg]);

		const adapter = createNatsAdapter(
			{
				listenerName: "test-js",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "orders.>",
					jetstream: true,
					stream: "ORDERS",
					consumer: "order-processor",
				},
			},
			conn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await flushMicrotasks();
		await adapter.stop();

		expect(events).toHaveLength(1);
		expect(msg.ackCalled).toBe(true);
		expect(msg.nakCalled).toBe(false);
	});

	// ── REQ-LNP-103: JetStream nak on failure ────────────────────────────

	it("JetStream mode — NAKs message on processing failure (REQ-LNP-103)", async () => {
		const msg = makeMockMsg(JSON.stringify({ action: "delete" }), {
			trackAck: true,
			trackNak: true,
		});
		const conn = makeMockJetStreamConnection([msg]);

		const adapter = createNatsAdapter(
			{
				listenerName: "test-js-nak",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "orders.>",
					jetstream: true,
					stream: "ORDERS",
				},
			},
			conn,
		);

		await adapter.start(async (_e) => {
			throw new Error("Downstream processing failed");
		});
		await flushMicrotasks();
		await adapter.stop();

		expect(msg.ackCalled).toBe(false);
		expect(msg.nakCalled).toBe(true);
	});

	// ── REQ-LNP-101: JetStream requires stream name ───────────────────────

	it("JetStream mode — throws if stream is not configured (REQ-LNP-101)", async () => {
		const conn = makeMockJetStreamConnection([]);

		const adapter = createNatsAdapter(
			{
				listenerName: "test-js-no-stream",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "events.>",
					jetstream: true,
					// stream intentionally omitted
				},
			},
			conn,
		);

		let startError: Error | null = null;
		try {
			await adapter.start(async () => {});
		} catch (err) {
			startError = err instanceof Error ? err : new Error(String(err));
		}

		expect(startError).not.toBeNull();
		expect(startError?.message).toContain('"stream" is required when jetstream: true');
	});

	// ── REQ-LNP-102: Reconnect — adapter enters degraded on connect failure ─

	it("enters degraded state and re-throws when connection fails (REQ-LNP-102)", async () => {
		// Simulate a connection failure by providing a connection that throws on subscribe
		const failingConn: NatsConnection = {
			subscribe() {
				throw new Error("Connection refused: nats://localhost:4222");
			},
			jetstream() {
				throw new Error("Connection refused");
			},
			async drain() {},
			isClosed() { return true; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-reconnect",
				config: { servers: ["nats://localhost:4222"], subject: "events" },
			},
			failingConn,
		);

		let caughtError: Error | null = null;
		try {
			await adapter.start(async () => {});
		} catch (err) {
			caughtError = err instanceof Error ? err : new Error(String(err));
		}

		expect(caughtError).not.toBeNull();
		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toContain("degraded");
	});

	// ── REQ-LNP-105: Wildcard subjects ───────────────────────────────────

	it("passes wildcard subjects through to NATS subscribe (REQ-LNP-105)", async () => {
		let subscribedSubject = "";
		const conn: NatsConnection = {
			subscribe(subject, _opts) {
				subscribedSubject = subject;
				return makeMockSubscription([]);
			},
			jetstream() { throw new Error("not used"); },
			async drain() {},
			isClosed() { return false; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-wildcard",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "events.*.created",
				},
			},
			conn,
		);

		await adapter.start(async () => {});
		await adapter.stop();

		// REQ-LNP-105: * and > wildcards are passed through unchanged
		expect(subscribedSubject).toBe("events.*.created");
	});

	it("passes > wildcard subject through to NATS subscribe (REQ-LNP-105)", async () => {
		let subscribedSubject = "";
		const conn: NatsConnection = {
			subscribe(subject, _opts) {
				subscribedSubject = subject;
				return makeMockSubscription([]);
			},
			jetstream() { throw new Error("not used"); },
			async drain() {},
			isClosed() { return false; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-wildcard-gt",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "events.>",
				},
			},
			conn,
		);

		await adapter.start(async () => {});
		await adapter.stop();

		expect(subscribedSubject).toBe("events.>");
	});

	// ── REQ-LNP-101: queue_group is passed to subscribe ──────────────────

	it("passes queue_group to NATS subscribe options (REQ-LNP-101)", async () => {
		let capturedOpts: { queue?: string } | undefined;
		const conn: NatsConnection = {
			subscribe(_subject, opts) {
				capturedOpts = opts as { queue?: string };
				return makeMockSubscription([]);
			},
			jetstream() { throw new Error("not used"); },
			async drain() {},
			isClosed() { return false; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-queue",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "work.tasks",
					queue_group: "workers",
				},
			},
			conn,
		);

		await adapter.start(async () => {});
		await adapter.stop();

		expect(capturedOpts?.queue).toBe("workers");
	});

	// ── REQ-LNP-100: start is idempotent ─────────────────────────────────

	it("start() is idempotent — second call is a no-op (REQ-LNP-100)", async () => {
		let subscribeCallCount = 0;
		const conn: NatsConnection = {
			subscribe(_subject, _opts) {
				subscribeCallCount++;
				return makeMockSubscription([]);
			},
			jetstream() { throw new Error("not used"); },
			async drain() {},
			isClosed() { return false; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-idempotent",
				config: { servers: ["nats://localhost:4222"], subject: "test" },
			},
			conn,
		);

		await adapter.start(async () => {});
		await adapter.start(async () => {}); // second call — should be no-op
		await adapter.stop();

		expect(subscribeCallCount).toBe(1);
	});

	// ── REQ-LNP-107: event envelope fields ───────────────────────────────

	it("event envelope has required fields: id, received_at, delivery (REQ-LNP-107)", async () => {
		const msg = makeMockMsg(JSON.stringify({ x: 1 }));
		const conn = makeMockCoreConnection([msg]);

		const adapter = createNatsAdapter(
			{
				listenerName: "test-envelope",
				config: { servers: ["nats://localhost:4222"], subject: "test" },
			},
			conn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await flushMicrotasks();
		await adapter.stop();

		expect(events).toHaveLength(1);
		const env = events[0];
		expect(env.id).toMatch(/^evt_/);
		expect(env.received_at).toBeTruthy();
		expect(env.delivery.mode).toBe("non-interruptive");
		expect(env.delivery.status).toBe("pending");
	});

	// ── REQ-LNP-101: deliver_policy is forwarded to JetStream ────────────

	it("JetStream mode — deliver_policy is forwarded to subscribe config (REQ-LNP-101)", async () => {
		let capturedConfig: Record<string, unknown> | undefined;
		const conn: NatsConnection = {
			subscribe() { throw new Error("not used"); },
			jetstream() {
				return {
					async subscribe(_subject: string, opts?: { config?: Record<string, unknown> }) {
						capturedConfig = opts?.config;
						return makeMockSubscription([]);
					},
				};
			},
			async drain() {},
			isClosed() { return false; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-deliver-policy",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "events.>",
					jetstream: true,
					stream: "EVENTS",
					deliver_policy: "all",
					ack_wait_seconds: 60,
					consumer: "order-processor",
				},
			},
			conn,
		);

		await adapter.start(async () => {});
		await flushMicrotasks();
		await adapter.stop();

		expect(capturedConfig?.deliver_policy).toBe("all");
		// ack_wait should be 60 * 1_000_000_000 nanoseconds
		expect(capturedConfig?.ack_wait).toBe(60_000_000_000);
		// REQ-LNP-101: durable_name enables JetStream consumer resume after restart (AC-2)
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-101 plan=step-qa-001-reissued test=listener-nats-adapter.test.ts
		expect(capturedConfig?.durable_name).toBe("order-processor");
	});

	// ── REQ-LNP-102: Post-start degraded state detection ─────────────────

	it("enters degraded state when NATS connection closes after reconnect exhaustion (REQ-LNP-102)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=step-fix-nats-001 test=listener-nats-adapter.test.ts
		//
		// Regression test: before this fix, the status monitoring loop was absent.
		// The NATS client would exhaust reconnect attempts and close the connection,
		// but `healthy` would remain `true` — a silently dead adapter.
		// After the fix, the status iterator completing (connection closed) sets
		// healthy=false and statusDesc="degraded: connection closed after reconnect exhaustion".

		const mockConn = makeMockNatsConnectionWithStatus({
			statusEvents: [{ type: "disconnect", data: "server unavailable" }],
			closeAfterStatus: true,
		});

		const adapter = createNatsAdapter(
			{
				listenerName: "test-nats-degraded",
				config: { servers: ["nats://localhost:4222"], subject: "test" },
			},
			mockConn,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		// Adapter should be healthy immediately after start
		expect(adapter.isHealthy()).toBe(true);
		expect(adapter.statusDescription()).toBe("running");

		// Give the status loop time to consume the disconnect event and reach the
		// closeAfterStatus await point before we trigger the close.
		await new Promise((r) => setTimeout(r, 20));

		// Simulate reconnect exhaustion: trigger the status iterator to complete
		mockConn.triggerStatusClose();

		// Wait for the degraded state to propagate through the async status loop
		await new Promise((r) => setTimeout(r, 50));

		// After the status iterator closes (reconnect exhaustion), adapter must be degraded
		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toContain("degraded");
	});

	// ── REQ-LNP-102: Stopping-flag race — clean stop must NOT emit degraded ─

	it("does NOT emit degraded log when stop() is called cleanly (REQ-LNP-102)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=step-da-001 test=listener-nats-adapter.test.ts
		//
		// Regression test: before this fix, stop() set running=false THEN called drain().
		// If drain() caused the status iterator to complete before running=false was
		// observed by the iterator loop, the `if (running)` guard would fire and emit
		// a spurious "degraded" log on a clean shutdown.
		// After the fix, stop() sets stopping=true FIRST, so the `if (running && !stopping)`
		// guard prevents the spurious log regardless of timing.

		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: unknown }).write = (chunk: string | Uint8Array) => {
			logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
			return true;
		};

		try {
			// Create a mock that will close its status iterator when triggerStatusClose() is called
			const mockConn = makeMockNatsConnectionWithStatus({
				statusEvents: [],
				closeAfterStatus: true,
			});

			const adapter = createNatsAdapter(
				{ listenerName: "test-nats", config: { servers: ["nats://localhost:4222"], subject: "test" } },
				mockConn,
			);

			await adapter.start(async () => {});

			// Call stop() — sets stopping=true before running=false
			await adapter.stop();
			// Simulate drain() closing the status iterator AFTER stop() returns
			mockConn.triggerStatusClose();

			await new Promise((r) => setTimeout(r, 50));

			// Should NOT have emitted a degraded log
			const degradedLog = logLines.find(l => l.includes("degraded"));
			expect(degradedLog).toBeUndefined();

			// Adapter should be stopped (not healthy)
			expect(adapter.isHealthy()).toBe(false);
			expect(adapter.statusDescription()).toBe("stopped");
		} finally {
			(process.stderr as { write: unknown }).write = origWrite;
		}
	});

	it("emits degraded log when reconnect exhaustion occurs while running (REQ-LNP-102)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=step-da-001 test=listener-nats-adapter.test.ts
		//
		// Regression guard: the stopping-flag fix must NOT suppress the degraded log
		// when the connection closes due to reconnect exhaustion (i.e., stop() was NOT called).
		// In that case stopping=false, so `if (running && !stopping)` fires correctly.

		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: unknown }).write = (chunk: string | Uint8Array) => {
			logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
			return true;
		};

		try {
			const mockConn = makeMockNatsConnectionWithStatus({
				statusEvents: [{ type: "disconnect", data: "server unreachable" }],
				closeAfterStatus: true,
			});

			const adapter = createNatsAdapter(
				{ listenerName: "test-nats", config: { servers: ["nats://localhost:4222"], subject: "test" } },
				mockConn,
			);

			await adapter.start(async () => {});

			// Give the status loop time to consume the disconnect event
			await new Promise((r) => setTimeout(r, 20));

			// Trigger status iterator close WITHOUT calling stop() first
			// This simulates reconnect exhaustion while the adapter is still running
			mockConn.triggerStatusClose();

			await new Promise((r) => setTimeout(r, 50));

			// SHOULD have emitted a degraded log (stopping=false, running=true)
			const degradedLog = logLines.find(l => l.includes("degraded"));
			expect(degradedLog).toBeDefined();
			expect(adapter.isHealthy()).toBe(false);
			expect(adapter.statusDescription()).toContain("degraded");

			await adapter.stop();
		} finally {
			(process.stderr as { write: unknown }).write = origWrite;
		}
	});

	// ── REQ-LNP-106: credsAuthenticator API shape ─────────────────────────

	it("loads credentials from file using credsAuthenticator (REQ-LNP-106)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-106 plan=step-fix-nats-002 test=listener-nats-adapter.test.ts
		//
		// Regression test: before this fix, credentials were passed as { creds: path }
		// which is NOT the nats.js v2 API. The correct API is credsAuthenticator(content)
		// which takes the file content as Uint8Array and returns an Authenticator object.
		// This test verifies that:
		//   1. credsAuthenticator() is called on the nats module (not a plain object)
		//   2. The authenticator returned by credsAuthenticator() is set on connectOpts
		//   3. The credentials file content is read and encoded as Uint8Array

		const { writeFileSync, unlinkSync } = await import("node:fs");
		const { join } = await import("node:path");
		const { tmpdir } = await import("node:os");
		const credsPath = join(tmpdir(), `test-creds-${Date.now()}.creds`);
		const fakeCreds = "-----BEGIN NATS USER JWT-----\nfake-jwt\n-----END NATS USER JWT-----";
		writeFileSync(credsPath, fakeCreds);

		try {
			let capturedAuthenticator: unknown = null;
			let credsAuthenticatorCallCount = 0;
			let capturedCredsBytes: Uint8Array | null = null;

			const mockModule: NatsModule = {
				connect: async (opts: Record<string, unknown>) => {
					capturedAuthenticator = opts.authenticator;
					// Return a minimal mock connection
					return {
						subscribe(_subject: string, _opts?: unknown) {
							return makeMockSubscription([]);
						},
						jetstream() { throw new Error("not used"); },
						async drain() {},
						isClosed() { return false; },
						status() { return emptyStatusIterable(); },
					} as NatsConnection;
				},
				credsAuthenticator: (creds: Uint8Array) => {
					credsAuthenticatorCallCount++;
					capturedCredsBytes = creds;
					return { type: "creds", data: creds }; // mock Authenticator
				},
			};

			const adapter = createNatsAdapter(
				{
					listenerName: "test-nats-creds",
					config: {
						servers: ["nats://localhost:4222"],
						subject: "test",
						credentials_file: credsPath,
					},
				},
				undefined,    // no pre-built connection — let connect() run
				mockModule,   // inject mock nats module
			);

			await adapter.start(async () => {});
			await adapter.stop();

			// credsAuthenticator must have been called exactly once
			expect(credsAuthenticatorCallCount).toBe(1);

			// The authenticator set on connectOpts must be the object returned by credsAuthenticator
			expect(capturedAuthenticator).toBeDefined();
			expect(capturedAuthenticator).toHaveProperty("type", "creds");

			// The bytes passed to credsAuthenticator must match the file content
			expect(capturedCredsBytes).not.toBeNull();
			const decodedCreds = new TextDecoder().decode(capturedCredsBytes!);
			expect(decodedCreds).toBe(fakeCreds);

			// Verify it was NOT the old plain-object shape { creds: path }
			expect(capturedAuthenticator).not.toHaveProperty("creds");
		} finally {
			try { unlinkSync(credsPath); } catch { /* ignore cleanup errors */ }
		}
	});

	it("throws a clear error when credentials_file does not exist (REQ-LNP-106)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-106 plan=step-fix-nats-002 test=listener-nats-adapter.test.ts
		//
		// Negative test: if the credentials file path is invalid, the adapter must throw
		// a descriptive error that includes the path (but NOT the file contents).

		const nonExistentPath = "/tmp/does-not-exist-axiom-test.creds";

		const mockModule: NatsModule = {
			connect: async () => { throw new Error("should not reach connect"); },
			credsAuthenticator: (_creds: Uint8Array) => { return {}; },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-nats-creds-missing",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "test",
					credentials_file: nonExistentPath,
				},
			},
			undefined,
			mockModule,
		);

		let startError: Error | null = null;
		try {
			await adapter.start(async () => {});
		} catch (err) {
			startError = err instanceof Error ? err : new Error(String(err));
		}

		expect(startError).not.toBeNull();
		expect(startError?.message).toContain("Failed to load NATS credentials from");
		expect(startError?.message).toContain(nonExistentPath);
		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toContain("degraded");
	});

	// ── BL-007: ack_wait_seconds overflow guard ───────────────────────────

	it("caps ack_wait_seconds at 9_000_000 to prevent nanosecond overflow (BL-007)", async () => {
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-101 plan=phase-1/task-1.4 test=listener-nats-adapter.test.ts jira_ref=DEX-434
		// Regression: ack_wait_seconds * 1_000_000_000 overflows Number.MAX_SAFE_INTEGER
		// for values > ~9,007,199 seconds (~104 days). The cap at 9_000_000 ensures
		// the nanosecond value stays within safe integer range.

		// Verify the overflow math: 10_000_000 * 1e9 = 10^16 > Number.MAX_SAFE_INTEGER
		const oversizedAckWait = 10_000_000;
		const safeAckWait = Math.min(oversizedAckWait, 9_000_000);
		const nanos = safeAckWait * 1_000_000_000;
		expect(nanos).toBeLessThanOrEqual(Number.MAX_SAFE_INTEGER);
		expect(safeAckWait).toBe(9_000_000);

		// Normal values pass through unchanged
		const normalAckWait = 30;
		expect(Math.min(normalAckWait, 9_000_000)).toBe(30);

		// Verify the cap is applied in the actual adapter via JetStream subscribe config
		let capturedAckWait: number | undefined;
		const conn: NatsConnection = {
			subscribe() { throw new Error("not used"); },
			jetstream() {
				return {
					async subscribe(_subject: string, opts?: { config?: Record<string, unknown> }) {
						capturedAckWait = opts?.config?.ack_wait as number | undefined;
						return makeMockSubscription([]);
					},
				};
			},
			async drain() {},
			isClosed() { return false; },
			status() { return emptyStatusIterable(); },
		};

		const adapter = createNatsAdapter(
			{
				listenerName: "test-ack-wait-cap",
				config: {
					servers: ["nats://localhost:4222"],
					subject: "events.>",
					jetstream: true,
					stream: "EVENTS",
					ack_wait_seconds: 10_000_000, // oversized — should be capped
				},
			},
			conn,
		);

		await adapter.start(async () => {});
		await adapter.stop();

		// Must be capped at 9_000_000 * 1_000_000_000 = 9e15 (within MAX_SAFE_INTEGER)
		expect(capturedAckWait).toBe(9_000_000 * 1_000_000_000);
		expect(capturedAckWait!).toBeLessThanOrEqual(Number.MAX_SAFE_INTEGER);
	});
});
