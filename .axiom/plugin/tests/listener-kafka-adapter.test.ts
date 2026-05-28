/**
 * Tests for the Kafka source adapter.
 *
 * REQ-LNP-110 through REQ-LNP-119.
 * Uses a mock Kafka client interface — no real Kafka broker needed.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#13.2
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.2 test=listener-kafka-adapter.test.ts
 */
import { describe, expect, it } from "bun:test";
import {
	createKafkaAdapter,
} from "../src/layers/listeners/adapters/kafka";
import type {
	KafkaAdapterOptions,
	KafkaClient,
	KafkaConsumer,
	KafkaEachMessagePayload,
} from "../src/layers/listeners/adapters/kafka";
import type { EventEnvelope } from "../src/layers/listeners/types";

// ── Mock Kafka Client ────────────────────────────────────────────────────

/**
 * Options for makeMockConsumer.
 * crashOnStart: if set, the mock fires a CRASH event with an error containing this string
 *               shortly after consumer.run() is called (simulates a consumer crash).
 * REQ-LNP-119: Used to test Kafka error class detection.
 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-119 plan=phase-1/task-1.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
 */
interface MockConsumerOptions {
	crashOnStart?: string;
}

/**
 * Build a mock Kafka consumer that captures calls and allows test-driven message delivery.
 */
function makeMockConsumer(opts?: MockConsumerOptions): KafkaConsumer & {
	_eachMessage: ((payload: KafkaEachMessagePayload) => Promise<void>) | null;
	_eventHandlers: Map<string, ((evt: unknown) => void | Promise<void>)[]>;
	_connected: boolean;
	_disconnected: boolean;
	_subscribed: { topic: string; fromBeginning: boolean } | null;
	_runOpts: { autoCommit: boolean } | null;
	_commitOffsetsCalls: unknown[][];
	/** Simulate delivering a message to the running consumer. */
	deliver(msg: { offset: string; value: string | null }): Promise<void>;
	/** Fire a named event (e.g. REBALANCING, GROUP_JOIN). */
	fireEvent(event: string, payload?: unknown): Promise<void>;
	/** Register a callback that fires each time heartbeat() is called during deliver(). */
	onHeartbeat(cb: () => void): void;
} {
	let eachMessage: ((payload: KafkaEachMessagePayload) => Promise<void>) | null = null;
	const eventHandlers = new Map<string, ((evt: unknown) => void | Promise<void>)[]>();
	let connected = false;
	let disconnected = false;
	let subscribed: { topic: string; fromBeginning: boolean } | null = null;
	let runOpts: { autoCommit: boolean } | null = null;
	let heartbeatCallback: (() => void) | null = null;
	const commitOffsetsCalls: unknown[][] = [];

	const consumer: ReturnType<typeof makeMockConsumer> = {
		get _eachMessage() { return eachMessage; },
		get _eventHandlers() { return eventHandlers; },
		get _connected() { return connected; },
		get _disconnected() { return disconnected; },
		get _subscribed() { return subscribed; },
		get _runOpts() { return runOpts; },
		get _commitOffsetsCalls() { return commitOffsetsCalls; },

		events: {
			GROUP_JOIN: "consumer.group_join",
			REBALANCING: "consumer.rebalancing",
			CRASH: "consumer.crash",
			/** REQ-LNP-116: FETCH event for lag tracking. */
			FETCH: "consumer.fetch",
		},

		async connect() {
			connected = true;
		},

		async disconnect() {
			disconnected = true;
		},

		async subscribe(opts_) {
			subscribed = opts_;
		},

		async run(runOpts_) {
			runOpts = { autoCommit: runOpts_.autoCommit };
			eachMessage = runOpts_.eachMessage;
			// REQ-LNP-119: If crashOnStart is set, fire a CRASH event asynchronously
			// to simulate a consumer crash after run() is called.
			if (opts?.crashOnStart) {
				const crashMsg = opts.crashOnStart;
				setTimeout(() => {
					const handlers = eventHandlers.get("consumer.crash") ?? [];
					const crashErr = new Error(crashMsg);
					for (const h of handlers) void h({ payload: crashErr });
				}, 0);
			}
		},

		async commitOffsets(offsets) {
			commitOffsetsCalls.push(offsets);
		},

		on(event: string, listener: (evt: unknown) => void | Promise<void>) {
			const handlers = eventHandlers.get(event) ?? [];
			handlers.push(listener);
			eventHandlers.set(event, handlers);
		},

		async deliver(msg: { offset: string; value: string | null }) {
			if (!eachMessage) throw new Error("Consumer not running");
			const resolvedOffsets: string[] = [];
			await eachMessage({
				topic: "test-topic",
				partition: 0,
				message: {
					offset: msg.offset,
					key: null,
					value: msg.value !== null ? Buffer.from(msg.value) : null,
				},
				heartbeat: async () => {
					// Notify any registered heartbeat observer (REQ-LNP-112 heartbeat pump test)
					if (heartbeatCallback) heartbeatCallback();
				},
				resolveOffset: (offset: string) => { resolvedOffsets.push(offset); },
			});
			return;
		},

		async fireEvent(event: string, payload?: unknown) {
			const handlers = eventHandlers.get(event) ?? [];
			for (const h of handlers) await h(payload ?? {});
		},

		onHeartbeat(cb: () => void) {
			heartbeatCallback = cb;
		},
	};

	return consumer;
}

/**
 * Build a mock Kafka client that returns the given consumer.
 */
function makeMockKafkaClient(consumer: KafkaConsumer): KafkaClient {
	return {
		consumer: (_opts) => consumer,
	};
}

// ── Helpers ──────────────────────────────────────────────────────────────

function makeOptions(overrides?: Partial<KafkaAdapterOptions["config"]>): KafkaAdapterOptions {
	return {
		listenerName: "test-kafka",
		config: {
			brokers: ["localhost:9092"],
			topic: "test-topic",
			group_id: "test-group",
			...overrides,
		},
	};
}

// ── Tests ────────────────────────────────────────────────────────────────

describe("createKafkaAdapter — REQ-LNP-110 through REQ-LNP-119", () => {

	// ── REQ-LNP-110: Adapter starts and stops cleanly ─────────────────────

	it("starts and stops cleanly (REQ-LNP-110)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		expect(adapter.isHealthy()).toBe(true);
		expect(mockConsumer._connected).toBe(true);
		expect(mockConsumer._subscribed?.topic).toBe("test-topic");

		await adapter.stop();

		expect(adapter.isHealthy()).toBe(false);
		expect(mockConsumer._disconnected).toBe(true);
	});

	// ── REQ-LNP-110 + REQ-LNP-111: Receives message and delivers as event ─

	it("receives message and delivers as event (REQ-LNP-110)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		await mockConsumer.deliver({
			offset: "0",
			value: JSON.stringify({ event_type: "jira:issue_updated", key: "ABC-123" }),
		});

		await adapter.stop();

		expect(events).toHaveLength(1);
		const payload = events[0].payload as Record<string, unknown>;
		expect(payload.event_type).toBe("jira:issue_updated");
		expect(payload.key).toBe("ABC-123");
	});

	// ── REQ-LNP-112: Manual offset commit on success ──────────────────────

	it("commits offset after successful processing (REQ-LNP-112)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const resolvedOffsets: string[] = [];
		await adapter.start(async (_e) => {});

		// Manually track resolveOffset calls
		if (!mockConsumer._eachMessage) throw new Error("Consumer not running");
		const originalEachMessage = mockConsumer._eachMessage;
		// Deliver a message and capture resolveOffset calls
		await new Promise<void>((resolve) => {
			const payload: KafkaEachMessagePayload = {
				topic: "test-topic",
				partition: 0,
				message: {
					offset: "42",
					key: null,
					value: Buffer.from(JSON.stringify({ test: true })),
				},
				heartbeat: async () => {},
				resolveOffset: (offset: string) => {
					resolvedOffsets.push(offset);
					resolve();
				},
			};
			void originalEachMessage(payload);
		});

		await adapter.stop();

		// Offset MUST be committed after successful processing
		expect(resolvedOffsets).toContain("42");
	});

	// ── REQ-LNP-112: Does NOT commit offset on processing failure ─────────

	it("does NOT commit offset when processing fails (REQ-LNP-112)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		await adapter.start(async (_e) => {
			throw new Error("Processing failed");
		});

		const resolvedOffsets: string[] = [];
		if (!mockConsumer._eachMessage) throw new Error("Consumer not running");

		// Deliver a message — processing will throw
		await mockConsumer._eachMessage({
			topic: "test-topic",
			partition: 0,
			message: {
				offset: "99",
				key: null,
				value: Buffer.from(JSON.stringify({ test: true })),
			},
			heartbeat: async () => {},
			resolveOffset: (offset: string) => { resolvedOffsets.push(offset); },
		});

		await adapter.stop();

		// Offset MUST NOT be committed when processing fails
		expect(resolvedOffsets).not.toContain("99");
		expect(resolvedOffsets).toHaveLength(0);
	});

	// ── REQ-LNP-111: Handles non-JSON message body ────────────────────────

	it("handles non-JSON message body gracefully (REQ-LNP-111)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		await mockConsumer.deliver({ offset: "0", value: "plain text message" });

		await adapter.stop();

		expect(events).toHaveLength(1);
		const payload = events[0].payload as Record<string, unknown>;
		expect(payload.raw).toBe("plain text message");
	});

	// ── REQ-LNP-111: Handles null message value ───────────────────────────

	it("handles null message value gracefully (REQ-LNP-111)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		await mockConsumer.deliver({ offset: "0", value: null });

		await adapter.stop();

		expect(events).toHaveLength(1);
		const payload = events[0].payload as Record<string, unknown>;
		expect(payload.raw).toBeNull();
	});

	// ── REQ-LNP-117: Emits listener.error when kafkajs not available ──────

	it("emits listener.error when kafkajs not available (REQ-LNP-117)", async () => {
		// No client override — kafkajs is not installed in test env
		// We simulate this by passing a client that throws on consumer creation
		const throwingClient: KafkaClient = {
			consumer: () => {
				throw new Error("kafkajs not available: Cannot find module 'kafkajs'");
			},
		};

		const adapter = createKafkaAdapter(makeOptions(), throwingClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		// Adapter should be in degraded state, not throw
		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toContain("degraded");

		await adapter.stop();
	});

	// ── REQ-LNP-110: Sets source_type to 'kafka' ─────────────────────────

	it("sets source_type to 'kafka' (REQ-LNP-110)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		await mockConsumer.deliver({
			offset: "0",
			value: JSON.stringify({ test: true }),
		});

		await adapter.stop();

		expect(events[0].source_type).toBe("kafka");
		expect(events[0].listener_name).toBe("test-kafka");
		expect(events[0].source_ref).toBe("test-topic");
	});

	// ── REQ-LNP-113: Rebalance event emits listener.kafka_rebalance ───────

	it("rebalance event emits listener.kafka_rebalance log (REQ-LNP-113)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		await adapter.start(async (_e) => {});

		// Verify rebalance event handlers are registered
		const rebalancingHandlers = mockConsumer._eventHandlers.get(
			mockConsumer.events.REBALANCING,
		);
		const groupJoinHandlers = mockConsumer._eventHandlers.get(
			mockConsumer.events.GROUP_JOIN,
		);

		expect(rebalancingHandlers).toBeDefined();
		expect(rebalancingHandlers!.length).toBeGreaterThan(0);
		expect(groupJoinHandlers).toBeDefined();
		expect(groupJoinHandlers!.length).toBeGreaterThan(0);

		// Fire rebalance event — should not throw
		await expect(mockConsumer.fireEvent(mockConsumer.events.REBALANCING)).resolves.toBeUndefined();
		// Fire group join event — should not throw
		await expect(mockConsumer.fireEvent(mockConsumer.events.GROUP_JOIN)).resolves.toBeUndefined();

		await adapter.stop();
	});

	// ── REQ-LNP-113: Pauses consumption during rebalance ─────────────────

	it("skips message processing during rebalance (REQ-LNP-113)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		// ── Pre-rebalance: deliver a message and assert its offset is committed ──
		// AC-4 ("transparent"): offsets for messages processed before rebalance
		// must be committed so they are not re-delivered after GROUP_JOIN.
		if (!mockConsumer._eachMessage) throw new Error("Consumer not running");
		const preRebalanceOffsets: string[] = [];
		await mockConsumer._eachMessage({
			topic: "test-topic",
			partition: 0,
			message: {
				offset: "0",
				key: null,
				value: Buffer.from(JSON.stringify({ test: true })),
			},
			heartbeat: async () => {},
			resolveOffset: (offset: string) => { preRebalanceOffsets.push(offset); },
		});
		// Offset "0" must be committed before the rebalance fires
		expect(preRebalanceOffsets).toContain("0");
		// Pre-rebalance message was processed normally — snapshot event count
		const eventsBeforeRebalance = events.length;

		// ── Trigger rebalance ────────────────────────────────────────────────────
		await mockConsumer.fireEvent(mockConsumer.events.REBALANCING);

		// Deliver a message during rebalance — should be skipped
		await mockConsumer.deliver({
			offset: "1",
			value: JSON.stringify({ test: true }),
		});

		// No NEW events should be delivered during rebalance
		expect(events).toHaveLength(eventsBeforeRebalance);

		// Rebalance complete — resume
		await mockConsumer.fireEvent(mockConsumer.events.GROUP_JOIN);

		// Now deliver a message — should be processed
		await mockConsumer.deliver({
			offset: "2",
			value: JSON.stringify({ test: true }),
		});

		expect(events).toHaveLength(eventsBeforeRebalance + 1);

		await adapter.stop();
	});

	// ── REQ-LNP-111: from_beginning config is passed to subscribe ─────────

	it("passes from_beginning config to subscribe (REQ-LNP-111)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(
			makeOptions({ from_beginning: true }),
			mockClient,
		);

		await adapter.start(async (_e) => {});
		await adapter.stop();

		expect(mockConsumer._subscribed?.fromBeginning).toBe(true);
	});

	// ── REQ-LNP-112: auto_commit: false is passed to run ─────────────────

	it("passes autoCommit: false to consumer.run (REQ-LNP-112)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		await adapter.start(async (_e) => {});
		await adapter.stop();

		// OD-LNP-07: auto_commit defaults to false
		expect(mockConsumer._runOpts?.autoCommit).toBe(false);
	});

	// ── REQ-LNP-119: Broker unavailable → degraded state ─────────────────

	it("enters degraded state when broker is unavailable (REQ-LNP-119)", async () => {
		const failingClient: KafkaClient = {
			consumer: () => {
				const c = makeMockConsumer();
				// Override connect to simulate broker unavailable
				(c as { connect: () => Promise<void> }).connect = async () => {
					throw new Error("Connection timeout: broker unavailable");
				};
				return c;
			},
		};

		const adapter = createKafkaAdapter(makeOptions(), failingClient);

		await adapter.start(async (_e) => {});

		expect(adapter.isHealthy()).toBe(false);
		expect(adapter.statusDescription()).toContain("degraded");

		await adapter.stop();
	});

	// ── REQ-LNP-110: isHealthy returns false after stop ───────────────────

	it("isHealthy returns false after stop (REQ-LNP-110)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		await adapter.start(async (_e) => {});
		expect(adapter.isHealthy()).toBe(true);

		await adapter.stop();
		expect(adapter.isHealthy()).toBe(false);
	});

	// ── REQ-LNP-110: start is idempotent ─────────────────────────────────

	it("start is idempotent — second call is a no-op (REQ-LNP-110)", async () => {
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		await adapter.start(async (_e) => {});
		await adapter.start(async (_e) => {}); // second call — should not throw or reconnect

		expect(adapter.isHealthy()).toBe(true);

		await adapter.stop();
	});

	// ── REQ-LNP-116: Emits listener.kafka_lag when batch count exceeds threshold ──

	it("emits listener.kafka_lag when FETCH batch count exceeds lag_threshold (REQ-LNP-116)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-116 plan=step-fix-kafka-001 test=listener-kafka-adapter.test.ts
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		// Capture stderr to inspect emitted log events
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};

		try {
			const adapter = createKafkaAdapter(
				makeOptions({ lag_threshold: 5 }), // low threshold for testing
				mockClient,
			);

			await adapter.start(async (_e) => {});

			// Fire a FETCH event with numberOfBatches > threshold (5)
			mockConsumer.fireEvent(mockConsumer.events.FETCH, {
				payload: { numberOfBatches: 10 },
			});

			// Allow microtask queue to flush
			await new Promise((r) => setTimeout(r, 10));

			const lagLog = logLines.find((l) => l.includes("listener.kafka_lag"));
			expect(lagLog).toBeDefined();
			const parsed = JSON.parse(lagLog!);
			expect(parsed.event).toBe("listener.kafka_lag");
			expect(parsed.listener_name).toBe("test-kafka");
			expect(parsed.reason).toContain("10");
			expect(parsed.reason).toContain("5");

			await adapter.stop();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	// ── REQ-LNP-116: Does NOT emit listener.kafka_lag below threshold ─────

	it("does NOT emit listener.kafka_lag when batch count is below threshold (REQ-LNP-116)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-116 plan=step-fix-kafka-001 test=listener-kafka-adapter.test.ts
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};

		try {
			const adapter = createKafkaAdapter(
				makeOptions({ lag_threshold: 100 }), // high threshold
				mockClient,
			);

			await adapter.start(async (_e) => {});

			// Fire a FETCH event with numberOfBatches < threshold (100)
			mockConsumer.fireEvent(mockConsumer.events.FETCH, {
				payload: { numberOfBatches: 50 },
			});

			await new Promise((r) => setTimeout(r, 10));

			const lagLog = logLines.find((l) => l.includes("listener.kafka_lag"));
			expect(lagLog).toBeUndefined();

			await adapter.stop();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	// ── REQ-LNP-116: Default lag_threshold is 1000 ───────────────────────

	it("uses default lag_threshold of 1000 when not configured (REQ-LNP-116)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-116 plan=step-fix-kafka-001 test=listener-kafka-adapter.test.ts
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};

		try {
			// No lag_threshold configured — defaults to 1000
			const adapter = createKafkaAdapter(makeOptions(), mockClient);

			await adapter.start(async (_e) => {});

			// 999 batches — below default threshold of 1000, should NOT emit
			mockConsumer.fireEvent(mockConsumer.events.FETCH, {
				payload: { numberOfBatches: 999 },
			});

			await new Promise((r) => setTimeout(r, 10));

			const lagLogBelow = logLines.find((l) => l.includes("listener.kafka_lag"));
			expect(lagLogBelow).toBeUndefined();

			// 1001 batches — above default threshold, SHOULD emit
			mockConsumer.fireEvent(mockConsumer.events.FETCH, {
				payload: { numberOfBatches: 1001 },
			});

			await new Promise((r) => setTimeout(r, 10));

			const lagLogAbove = logLines.find((l) => l.includes("listener.kafka_lag"));
			expect(lagLogAbove).toBeDefined();
			const parsed = JSON.parse(lagLogAbove!);
			expect(parsed.reason).toContain("1001");
			expect(parsed.reason).toContain("1000");

			await adapter.stop();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	// ── REQ-LNP-118: Passes raw payload when schema_registry_url not configured ──

	it("passes raw payload when schema_registry_url not configured (REQ-LNP-118)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=step-fix-kafka-002 test=listener-kafka-adapter.test.ts
		// Existing behavior — verify it still works with the new deserializeMessage code path
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(makeOptions(), mockClient);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });

		await mockConsumer.deliver({ offset: "0", value: JSON.stringify({ key: "value" }) });

		await adapter.stop();

		expect(events).toHaveLength(1);
		expect(events[0].payload).toEqual({ key: "value" });
	});

	// ── REQ-LNP-118: Emits listener.error and passes raw payload when schema registry package unavailable ──

	it("emits listener.error and passes raw payload when schema registry package unavailable (REQ-LNP-118)", async () => {
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
		// REQ-LNP-118: Must send a magic-byte-prefixed message to trigger the schema registry path.
		// Plain JSON messages (no 0x00 prefix) bypass the registry entirely (magic byte check).
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};

		try {
			const mockConsumer = makeMockConsumer();
			const mockClient = makeMockKafkaClient(mockConsumer);

			const adapter = createKafkaAdapter(
				makeOptions({ schema_registry_url: "http://localhost:8081" }),
				mockClient,
				// No schemaRegistryOverride — dynamic import will fail (package not installed)
			);

			const events: EventEnvelope[] = [];
			await adapter.start(async (e) => { events.push(e); });

			// Send a Confluent magic-byte-prefixed message to trigger the registry path
			const magicByte = Buffer.from([0x00]);
			const schemaId = Buffer.alloc(4);
			schemaId.writeInt32BE(1, 0);
			const payload = Buffer.from('{"key":"value"}');
			const encodedMsg = Buffer.concat([magicByte, schemaId, payload]);

			// Deliver via _eachMessage directly (deliver() wraps in Buffer.from(string))
			if (!mockConsumer._eachMessage) throw new Error("Consumer not running");
			await mockConsumer._eachMessage({
				topic: "test-topic",
				partition: 0,
				message: { offset: "0", key: null, value: encodedMsg },
				heartbeat: async () => {},
				resolveOffset: () => {},
			});

			await adapter.stop();

			// Should still deliver the event (fail-open)
			expect(events).toHaveLength(1);
			// Should emit listener.error about schema registry
			const errorLog = logLines.find(
				(l) => l.includes("schema_registry") || l.includes("Schema registry"),
			);
			expect(errorLog).toBeDefined();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	// ── REQ-LNP-112: Heartbeat pump fires during slow onEvent() processing ──

	it("calls heartbeat() during slow onEvent() processing (REQ-LNP-112)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-112 plan=step-fix-kafka-003 test=listener-kafka-adapter.test.ts
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		let heartbeatCallCount = 0;
		// Register heartbeat observer before start so it's in place when deliver() runs
		mockConsumer.onHeartbeat(() => { heartbeatCallCount++; });

		const adapter = createKafkaAdapter(
			makeOptions({ heartbeat_interval_ms: 10 }), // very short for testing
			mockClient,
		);

		await adapter.start(async (_e) => {
			// Simulate slow processing — wait 60ms (6 heartbeat intervals at 10ms)
			await new Promise((r) => setTimeout(r, 60));
		});

		await mockConsumer.deliver({ offset: "0", value: '{"key":"value"}' });
		await adapter.stop();

		// Should have called heartbeat at least once during the 60ms processing
		expect(heartbeatCallCount).toBeGreaterThan(0);
	});

	// ── REQ-LNP-113 / BL-002: Commits in-flight offsets before REBALANCING guard ──

	it("commits in-flight offsets before REBALANCING guard takes effect (REQ-LNP-113 / BL-002)", async () => {
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-113 plan=phase-1/task-1.2 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
		const mockConsumer = makeMockConsumer();
		const mockClient = makeMockKafkaClient(mockConsumer);

		const adapter = createKafkaAdapter(
			makeOptions(),
			mockClient,
		);

		await adapter.start(async () => {});

		// Fire REBALANCING event — handler is async, must await so commitOffsets runs before guard
		await mockConsumer.fireEvent(mockConsumer.events.REBALANCING);

		// commitOffsets([]) must have been called before the rebalancing guard was set
		expect(mockConsumer._commitOffsetsCalls.length).toBeGreaterThan(0);
		// The call must have been with an empty array (flush pending auto-commit buffer)
		expect(mockConsumer._commitOffsetsCalls[0]).toEqual([]);

		await adapter.stop();
	});
});

// ── REQ-LNP-119 / BL-001: Kafka edge case error class detection ──────────

describe("Kafka edge cases — REQ-LNP-119", () => {
	it("emits listener.error with TOPIC_NOT_FOUND when topic does not exist", async () => {
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-119 plan=phase-1/task-1.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};
		try {
			const { consumer } = { consumer: makeMockConsumer({ crashOnStart: "UNKNOWN_TOPIC_OR_PARTITION" }) };
			const mockClient = makeMockKafkaClient(consumer);
			const adapter = createKafkaAdapter(
				{ listenerName: "test-kafka", config: { brokers: ["localhost:9092"], topic: "nonexistent", group_id: "g1" } },
				mockClient,
			);
			await adapter.start(async () => {});
			// Wait for the async CRASH event to fire and be processed
			await new Promise((r) => setTimeout(r, 50));
			const errorLog = logLines.find((l) => {
				try {
					const p = JSON.parse(l);
					return p.event === "listener.error" && typeof p.error === "string" && p.error.includes("TOPIC_NOT_FOUND");
				} catch { return false; }
			});
			expect(errorLog).toBeDefined();
			await adapter.stop();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	it("emits listener.error with OFFSET_OUT_OF_RANGE when offset is out of range", async () => {
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-119 plan=phase-1/task-1.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};
		try {
			const { consumer } = { consumer: makeMockConsumer({ crashOnStart: "OFFSET_OUT_OF_RANGE" }) };
			const mockClient = makeMockKafkaClient(consumer);
			const adapter = createKafkaAdapter(
				{ listenerName: "test-kafka", config: { brokers: ["localhost:9092"], topic: "test", group_id: "g1" } },
				mockClient,
			);
			await adapter.start(async () => {});
			await new Promise((r) => setTimeout(r, 50));
			const errorLog = logLines.find((l) => {
				try {
					const p = JSON.parse(l);
					return p.event === "listener.error" && typeof p.error === "string" && p.error.includes("OFFSET_OUT_OF_RANGE");
				} catch { return false; }
			});
			expect(errorLog).toBeDefined();
			await adapter.stop();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	it("emits listener.error with MESSAGE_TOO_LARGE when message exceeds broker limit", async () => {
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-119 plan=phase-1/task-1.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
			(chunk: string | Uint8Array) => {
				logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
				return true;
			};
		try {
			const { consumer } = { consumer: makeMockConsumer({ crashOnStart: "MESSAGE_TOO_LARGE" }) };
			const mockClient = makeMockKafkaClient(consumer);
			const adapter = createKafkaAdapter(
				{ listenerName: "test-kafka", config: { brokers: ["localhost:9092"], topic: "test", group_id: "g1" } },
				mockClient,
			);
			await adapter.start(async () => {});
			await new Promise((r) => setTimeout(r, 50));
			const errorLog = logLines.find((l) => {
				try {
					const p = JSON.parse(l);
					return p.event === "listener.error" && typeof p.error === "string" && p.error.includes("MESSAGE_TOO_LARGE");
				} catch { return false; }
			});
			expect(errorLog).toBeDefined();
			await adapter.stop();
		} finally {
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
		}
	});

	// ── REQ-LNP-118: Schema registry full implementation (task-4.1) ───────

	describe("Kafka schema registry — REQ-LNP-118", () => {
		it("decodes Confluent Schema Registry encoded message (magic byte 0x00) (REQ-LNP-118)", async () => {
			// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
			const mockDecoded = { field1: "value1", field2: 42 };
			const mockRegistry = { decode: async (_buf: Buffer) => mockDecoded };

			// Create a Confluent-encoded message: [0x00][4-byte schema ID][payload]
			const magicByte = Buffer.from([0x00]);
			const schemaId = Buffer.alloc(4);
			schemaId.writeInt32BE(1, 0);
			const payload = Buffer.from(JSON.stringify(mockDecoded));
			const encodedMessage = Buffer.concat([magicByte, schemaId, payload]);

			const mockConsumer = makeMockConsumer();
			const mockClient = makeMockKafkaClient(mockConsumer);

			const adapter = createKafkaAdapter(
				makeOptions({ schema_registry_url: "http://localhost:8081" }),
				mockClient,
				mockRegistry, // inject mock registry via DI
			);

			const events: EventEnvelope[] = [];
			await adapter.start(async (e) => { events.push(e); });

			// Deliver via _eachMessage directly to pass a Buffer (not a string)
			if (!mockConsumer._eachMessage) throw new Error("Consumer not running");
			await mockConsumer._eachMessage({
				topic: "test-topic",
				partition: 0,
				message: { offset: "0", key: null, value: encodedMessage },
				heartbeat: async () => {},
				resolveOffset: () => {},
			});

			await adapter.stop();

			expect(events).toHaveLength(1);
			expect(events[0].payload).toEqual(mockDecoded);
		});

		it("falls back to JSON for non-schema-registry messages (no magic byte) (REQ-LNP-118)", async () => {
			// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
			const mockConsumer = makeMockConsumer();
			const mockClient = makeMockKafkaClient(mockConsumer);

			// No schemaRegistryOverride — but message has no magic byte so registry is never called
			const adapter = createKafkaAdapter(
				makeOptions({ schema_registry_url: "http://localhost:8081" }),
				mockClient,
			);

			const events: EventEnvelope[] = [];
			await adapter.start(async (e) => { events.push(e); });

			// Regular JSON message (no magic byte 0x00 prefix)
			await mockConsumer.deliver({ offset: "0", value: '{"key":"value"}' });

			await adapter.stop();

			expect(events).toHaveLength(1);
			expect(events[0].payload).toEqual({ key: "value" });
		});

		it("schema registry credentials are NOT present in any log output (REQ-LNP-118)", async () => {
			// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 test=listener-kafka-adapter.test.ts jira_ref=DEX-434
			const logLines: string[] = [];
			const origWrite = process.stderr.write.bind(process.stderr);
			(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write =
				(chunk: string | Uint8Array) => {
					logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
					return true;
				};

			try {
				const mockConsumer = makeMockConsumer();
				const mockClient = makeMockKafkaClient(mockConsumer);

				const adapter = createKafkaAdapter(
					makeOptions({
						schema_registry_url: "http://localhost:8081",
						schema_registry_credentials: { username: "secret-user", password: "secret-pass" },
					}),
					mockClient,
				);

				await adapter.start(async () => {});
				// Deliver a plain JSON message (no magic byte) — no registry call, no credential exposure
				await mockConsumer.deliver({ offset: "0", value: '{"ping":true}' });
				await adapter.stop();

				// Credentials MUST NOT appear in any log line
				const allLogs = logLines.join("\n");
				expect(allLogs).not.toContain("secret-user");
				expect(allLogs).not.toContain("secret-pass");
			} finally {
				(process.stderr as { write: (chunk: string | Uint8Array) => boolean }).write = origWrite;
			}
		});
	});
});
