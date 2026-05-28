/**
 * Kafka source adapter for the Listen/Notify Plugin.
 *
 * REQ-LNP-110: Kafka listener support — connect to Kafka cluster using bootstrap servers.
 * REQ-LNP-111: Configuration parameters (brokers, topic, group_id, sasl, ssl, etc.).
 * REQ-LNP-112: Manual offset commit — commit only after successful processing.
 * REQ-LNP-113: Consumer group rebalance — pause/resume, emit listener.kafka_rebalance.
 * REQ-LNP-114: Partition-level parallelism — sequential per partition to preserve ordering.
 * REQ-LNP-115: Credentials MUST NOT be logged.
 * REQ-LNP-116: Structured log events including listener.kafka_rebalance and listener.kafka_lag.
 * REQ-LNP-117: Dynamic import of kafkajs — clear error if not installed.
 * REQ-LNP-119: Edge cases — broker unavailable → degraded + backoff; topic not found → error; offset out of range → reset + log.
 *
 * OD-LNP-07 resolved: auto_commit: false (manual ack after successful processing).
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#13.2
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.2
 */

import type { KafkaListenerConfig } from "../../../config-loader";
import type { EventCallback, ListenerAdapter } from "../types";
import {
	emitListenerLog,
	generateEventId,
	truncatePayload,
} from "../types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating a Kafka adapter. */
export interface KafkaAdapterOptions {
	listenerName: string;
	config: KafkaListenerConfig;
}

// ── Kafka Client Interface (for testability) ─────────────────────────────

/** A single Kafka message as received by eachMessage. */
export interface KafkaMessage {
	offset: string;
	key: Buffer | null;
	value: Buffer | null;
	headers?: Record<string, Buffer | string | undefined>;
	timestamp?: string;
}

/** Batch context passed to eachMessage. */
export interface KafkaEachMessagePayload {
	topic: string;
	partition: number;
	message: KafkaMessage;
	heartbeat: () => Promise<void>;
	resolveOffset: (offset: string) => void;
}

/** Minimal Kafka consumer interface (allows mocking in tests). */
export interface KafkaConsumer {
	connect(): Promise<void>;
	disconnect(): Promise<void>;
	subscribe(opts: { topic: string; fromBeginning: boolean }): Promise<void>;
	run(opts: {
		autoCommit: boolean;
		eachMessage: (payload: KafkaEachMessagePayload) => Promise<void>;
	}): Promise<void>;
	/**
	 * Commit specific offsets (or flush pending auto-commit buffer when called with []).
	 * REQ-LNP-113: Used to flush in-flight offsets before rebalance guard takes effect.
	 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-113 plan=phase-1/task-1.2 jira_ref=DEX-434
	 */
	commitOffsets(offsets: { topic: string; partition: number; offset: string }[]): Promise<void>;
	on(event: string, listener: (event: unknown) => void | Promise<void>): void;
	events: {
		GROUP_JOIN: string;
		REBALANCING: string;
		CRASH: string;
		/** REQ-LNP-116: FETCH event for lag tracking. */
		FETCH: string;
	};
}

/** Minimal Kafka client interface (allows mocking in tests). */
export interface KafkaClient {
	consumer(opts: {
		groupId: string;
		sessionTimeout?: number;
		heartbeatInterval?: number;
		maxBytesPerPartition?: number;
	}): KafkaConsumer;
}

// ── Error Classification ─────────────────────────────────────────────────

/**
 * Classify a Kafka error into a known error class for operator diagnostics.
 * REQ-LNP-119: Detect specific Kafka error classes for operator diagnostics.
 *
 * VERSION NOTE: This function uses string matching on kafkajs error messages.
 * Error message strings are implementation details of kafkajs and may change
 * across versions. Verified against kafkajs v2.x. If upgrading kafkajs, re-verify
 * these strings or migrate to error.type field checks if kafkajs exposes them.
 * See package.json for the pinned kafkajs version range.
 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-119 plan=phase-1/task-1.1 jira_ref=DEX-434
 */
function classifyKafkaError(err: unknown): string {
	const msg = err instanceof Error ? err.message : String(err);
	const name = err instanceof Error ? err.name : "";
	if (
		msg.includes("UNKNOWN_TOPIC_OR_PARTITION") ||
		(name.includes("KafkaJSProtocolError") && msg.includes("UNKNOWN_TOPIC"))
	) {
		return "TOPIC_NOT_FOUND";
	}
	if (msg.includes("OFFSET_OUT_OF_RANGE") || name.includes("KafkaJSOffsetOutOfRange")) {
		return "OFFSET_OUT_OF_RANGE";
	}
	if (msg.includes("MESSAGE_TOO_LARGE") || msg.includes("RecordTooLargeException")) {
		return "MESSAGE_TOO_LARGE";
	}
	return "UNKNOWN";
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create a Kafka listener adapter.
 * REQ-LNP-110: Connects to Kafka cluster using bootstrap servers.
 * REQ-LNP-117: Dynamic import of kafkajs — clear error if not installed.
 */
export function createKafkaAdapter(
	options: KafkaAdapterOptions,
	/** Optional Kafka client override for testing. */
	kafkaClientOverride?: KafkaClient,
	/** Optional schema registry override for testing (REQ-LNP-118 DI).
	 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 jira_ref=DEX-434
	 */
	schemaRegistryOverride?: { decode: (buf: Buffer) => Promise<unknown> },
): ListenerAdapter {
	const { listenerName, config } = options;
	const {
		brokers,
		topic,
		group_id,
		client_id = "axiom-listener",
		from_beginning = false,
		auto_commit = false,
		session_timeout_ms = 30000,
		heartbeat_interval_ms = 3000,
		max_bytes_per_partition = 1048576,
	} = config;

	// REQ-LNP-116: Lag threshold for listener.kafka_lag emission (default: 1000).
	// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-116 plan=step-fix-kafka-001
	const lagThreshold = config.lag_threshold ?? 1000;
	let lastKnownLag = 0;

	let running = false;
	let healthy = true;
	let statusDesc = "stopped";
	let consumer: KafkaConsumer | null = null;
	let rebalancing = false;

	// REQ-LNP-118: Cache SchemaRegistry instance to avoid per-message instantiation.
	// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 jira_ref=DEX-434
	let schemaRegistryInstance: { decode: (buf: Buffer) => Promise<unknown> } | null = null;

	/**
	 * Lazily create (and cache) the SchemaRegistry instance.
	 * REQ-LNP-118: Credentials MUST NOT be logged.
	 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 jira_ref=DEX-434
	 */
	async function getSchemaRegistry(): Promise<{ decode: (buf: Buffer) => Promise<unknown> }> {
		// DI override for tests
		if (schemaRegistryOverride) return schemaRegistryOverride;
		// Return cached instance if already created
		if (schemaRegistryInstance) return schemaRegistryInstance;

		// Use a variable to prevent bun build from statically resolving the import.
		// This package is optional — only needed when schema_registry_url is configured.
		const schemaRegistryPkg = "@kafkajs/confluent-schema-registry";
		const { SchemaRegistry } = await import(schemaRegistryPkg);
		const registryConfig: Record<string, unknown> = { host: config.schema_registry_url };
		// REQ-LNP-118: Credentials MUST NOT be logged — passed to constructor only
		if (config.schema_registry_credentials) {
			registryConfig.auth = {
				username: config.schema_registry_credentials.username,
				password: config.schema_registry_credentials.password,
			};
		}
		schemaRegistryInstance = new SchemaRegistry(registryConfig) as { decode: (buf: Buffer) => Promise<unknown> };
		return schemaRegistryInstance;
	}

	/**
	 * Deserialize a Kafka message value using the schema registry if configured.
	 * REQ-LNP-118: Magic byte (0x00) detection — only attempt schema registry decode
	 * for Confluent-encoded messages. Falls back to JSON/raw for non-encoded messages.
	 * Fail-open if schema registry package is not installed or decode fails.
	 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=phase-4/task-4.1 jira_ref=DEX-434
	 */
	async function deserializeMessage(value: Buffer | null): Promise<unknown> {
		if (!value) return { raw: null };

		if (!config.schema_registry_url) {
			// No schema registry — parse as JSON or raw
			try {
				return JSON.parse(value.toString("utf-8"));
			} catch {
				return { raw: value.toString("utf-8") };
			}
		}

		// REQ-LNP-118: Check for Confluent Schema Registry magic byte (0x00).
		// Confluent-encoded messages start with: [0x00][4-byte schema ID][payload].
		// Non-encoded messages (plain JSON, etc.) will not have this prefix.
		//
		// KNOWN RISK: Binary payloads whose first byte is 0x00 (protobuf, CBOR, MessagePack,
		// or null-prefixed custom formats) will be incorrectly routed to the schema registry.
		// The fail-open path handles this safely (logs error, falls back to JSON/raw), but
		// the misrouting is silent — no dedicated metric is emitted. Operators should ensure
		// non-Confluent topics do not use 0x00-prefixed encodings when schema_registry_url
		// is configured. See specs/88-Listen-Notify-Plugin.md#REQ-LNP-118.
		if (value[0] !== 0x00) {
			// Not a schema-registry-encoded message — fall back to JSON/raw
			try {
				return JSON.parse(value.toString("utf-8"));
			} catch {
				return { raw: value.toString("utf-8") };
			}
		}

		// Schema registry encoded — attempt deserialization via cached registry
		try {
			const registry = await getSchemaRegistry();
			return await registry.decode(value);
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			// Fail-open: log error and pass raw bytes through
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: `Schema registry deserialization failed: ${msg} — passing raw payload`,
			});
			try {
				return JSON.parse(value.toString("utf-8"));
			} catch {
				return { raw: value.toString("utf-8") };
			}
		}
	}

	/**
	 * Dynamically import kafkajs and create a Kafka client.
	 * REQ-LNP-117: If kafkajs is not installed, emit a clear error.
	 * REQ-LNP-115: SASL credentials MUST NOT be logged.
	 */
	async function getKafkaClient(): Promise<KafkaClient> {
		if (kafkaClientOverride) return kafkaClientOverride;

		try {
			// REQ-LNP-117: Dynamic import — avoids hard dependency.
			// Use variable to prevent bun build from statically resolving.
			const kafkaPkg = "kafkajs";
			const { Kafka } = await import(kafkaPkg);

			// Build Kafka constructor options
			// REQ-LNP-115: SASL credentials are passed to the constructor but NEVER logged
			const kafkaOpts: Record<string, unknown> = {
				clientId: client_id,
				brokers,
			};

			// REQ-LNP-111: SASL authentication
			if (config.sasl) {
				kafkaOpts.sasl = {
					mechanism: config.sasl.mechanism,
					username: config.sasl.username,
					password: config.sasl.password,
					// REQ-LNP-115: password is passed to kafkajs but never emitted in logs
				};
			}

			// REQ-LNP-111: SSL/TLS configuration
			if (config.ssl) {
				kafkaOpts.ssl = {
					ca: config.ssl.ca_cert ? [config.ssl.ca_cert] : undefined,
					cert: config.ssl.client_cert,
					key: config.ssl.client_key,
				};
			}

			return new Kafka(kafkaOpts) as KafkaClient;
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new Error(
				`kafkajs not available: ${msg}. Install it to use Kafka listeners.`,
			);
		}
	}

	/**
	 * Wire up consumer group rebalance event handlers.
	 * REQ-LNP-113: Pause consumption during rebalance, resume after.
	 * REQ-LNP-116: Emit listener.kafka_rebalance on rebalance events.
	 */
	function wireRebalanceEvents(c: KafkaConsumer): void {
		// REBALANCING: consumer group is rebalancing — commit in-flight offsets then pause consumption.
		// REQ-LNP-113: Commit in-flight offsets before rebalancing guard takes effect.
		// This ensures messages processed but not yet committed are acknowledged before
		// partition reassignment. At-least-once delivery still applies — duplicates possible.
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-113 plan=phase-1/task-1.2 jira_ref=DEX-434
		c.on(c.events.REBALANCING, async (_evt: unknown) => {
			try {
				// commitOffsets([]) flushes the pending auto-commit buffer in kafkajs
				await c.commitOffsets([]);
			} catch {
				// Non-fatal — best-effort commit before rebalance
			}
			rebalancing = true;
			emitListenerLog({
				event: "listener.kafka_rebalance",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				reason: "rebalance",
			});
			// Also emit listener.error as informational per REQ-LNP-113
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: "Consumer group rebalancing — consumption paused",
				reason: "rebalance",
			});
		});

		// GROUP_JOIN: rebalance complete — resume consumption
		c.on(c.events.GROUP_JOIN, (_evt: unknown) => {
			rebalancing = false;
			emitListenerLog({
				event: "listener.kafka_rebalance",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				reason: "rebalance_complete",
			});
		});

		// CRASH: consumer crashed — classify error and mark unhealthy.
		// REQ-LNP-119: Detect specific Kafka error classes for operator diagnostics.
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-119 plan=phase-1/task-1.1 jira_ref=DEX-434
		c.on(c.events.CRASH, (evt: unknown) => {
			const rawErr =
				evt && typeof evt === "object" && "payload" in evt
					? (evt as { payload: unknown }).payload
					: evt;
			const errMsg = rawErr instanceof Error ? rawErr.message : String(rawErr ?? "unknown crash");
			const errorClass = classifyKafkaError(rawErr ?? new Error(errMsg));
			healthy = false;
			statusDesc = `degraded: consumer crashed — ${errMsg}`;

			// REQ-LNP-119: OFFSET_OUT_OF_RANGE — log reset intent (kafkajs handles actual reset via autoOffsetReset config)
			if (errorClass === "OFFSET_OUT_OF_RANGE") {
				emitListenerLog({
					event: "listener.error",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					error: `Kafka offset out of range — resetting to ${from_beginning ? "earliest" : "latest"} offset`,
				});
			}

			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: `Kafka consumer error [${errorClass}]: ${errMsg}`,
			});
		});
	}

	/**
	 * Wire up FETCH event handler for consumer lag tracking.
	 * REQ-LNP-116: Emit listener.kafka_lag when numberOfBatches exceeds lagThreshold.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-116 plan=step-fix-kafka-001
	 */
	function wireLagTracking(c: KafkaConsumer): void {
		c.on(c.events.FETCH, (evt: unknown) => {
			const batchCount =
				evt &&
				typeof evt === "object" &&
				"payload" in evt &&
				evt.payload !== null &&
				typeof evt.payload === "object" &&
				"numberOfBatches" in (evt.payload as object)
					? Number((evt.payload as { numberOfBatches: unknown }).numberOfBatches)
					: 0;

			if (batchCount > lagThreshold) {
				lastKnownLag = batchCount;
				// REQ-LNP-116: emit listener.kafka_lag when lag exceeds threshold
				emitListenerLog({
					event: "listener.kafka_lag",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					reason: `Consumer lag detected: ${batchCount} batches pending (threshold: ${lagThreshold})`,
				});
			} else if (batchCount === 0 && lastKnownLag > lagThreshold) {
				// Caught up — reset lag tracking
				lastKnownLag = 0;
			}
		});
	}

	return {
		async start(onEvent: EventCallback): Promise<void> {
			if (running) return;
			running = true;
			statusDesc = "starting";

			emitListenerLog({
				event: "listener.started",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
			});

			try {
				const kafkaClient = await getKafkaClient();

				// REQ-LNP-111: Create consumer with config parameters
				consumer = kafkaClient.consumer({
					groupId: group_id,
					sessionTimeout: session_timeout_ms,
					heartbeatInterval: heartbeat_interval_ms,
					maxBytesPerPartition: max_bytes_per_partition,
				});

				// REQ-LNP-113: Wire rebalance event handlers before connect
				wireRebalanceEvents(consumer);

				// REQ-LNP-116: Wire FETCH event handler for lag tracking
				wireLagTracking(consumer);

				await consumer.connect();

				// REQ-LNP-111: Subscribe to topic
				await consumer.subscribe({ topic, fromBeginning: from_beginning });

				// REQ-LNP-112: Manual offset commit — autoCommit: false
				// REQ-LNP-114: eachMessage processes partitions sequentially (kafkajs default)
				await consumer.run({
					autoCommit: auto_commit,
					eachMessage: async ({ message, heartbeat, resolveOffset }: KafkaEachMessagePayload) => {
					// REQ-LNP-113: Skip processing during rebalance
					if (rebalancing) return;

					// REQ-LNP-118: Deserialize message via schema registry if configured; else JSON/raw.
					// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=step-fix-kafka-002
					const payload = await deserializeMessage(message.value as Buffer | null);

						// REQ-LNP-045 (via types): Truncate payload to 64KB
						const { payload: truncated, truncated: wasTruncated } =
							truncatePayload(payload);

						const event = {
							id: generateEventId(),
							listener_name: listenerName,
							source_type: "kafka" as const,
							source_ref: topic,
							received_at: new Date().toISOString(),
							payload: truncated,
							truncated: wasTruncated,
							delivery: {
								mode: "non-interruptive" as const,
								status: "pending" as const,
								attempts: 0,
								last_attempt_at: null,
							},
						};

						emitListenerLog({
							event: "listener.event_received",
							listener_name: listenerName,
							timestamp: new Date().toISOString(),
							event_id: event.id,
							truncated: wasTruncated,
						});

// REQ-LNP-112: Heartbeat pump — keeps the kafkajs consumer session alive during
					// slow onEvent() processing. Without this, if onEvent() takes longer than
					// session_timeout_ms (default: 30s), kafkajs marks the session as timed out and
					// resolveOffset() may silently not commit.
					//
					// Note on double-heartbeat: kafkajs's internal eachMessageJob runner may also call
					// heartbeat() during message processing (see kafkajs source:
					// consumer/runner/eachMessageJob.ts). If so, this pump fires an additional heartbeat
					// at the same interval — the broker receives heartbeats at ~2x the configured rate.
					// This is safe (more heartbeats = more session keepalive) but slightly wasteful.
					// The pump is retained as a defensive safety net for slow handlers regardless of
					// kafkajs internal behavior, since kafkajs version behavior may change.
					//
					// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-112 plan=step-da-003
					const heartbeatIntervalMs = Math.floor(config.heartbeat_interval_ms ?? 3000);
					let heartbeatTimer: ReturnType<typeof setInterval> | null = null;

					try {
						// Start heartbeat pump
						heartbeatTimer = setInterval(() => {
								void heartbeat().catch(() => {
									// Non-fatal — heartbeat failure will eventually cause session timeout
								});
							}, heartbeatIntervalMs);

							await onEvent(event);

							// REQ-LNP-112: Manual offset commit — only after successful processing
							resolveOffset(message.offset);
						} catch (err) {
							// REQ-LNP-112: Do NOT commit offset on processing failure
							const errMsg = err instanceof Error ? err.message : String(err);
							emitListenerLog({
								event: "listener.error",
								listener_name: listenerName,
								timestamp: new Date().toISOString(),
								event_id: event.id,
								error: `Event processing failed: ${errMsg} — offset not committed`,
							});
						} finally {
							// Always clear heartbeat pump
							if (heartbeatTimer) {
								clearInterval(heartbeatTimer);
								heartbeatTimer = null;
							}
						}
					},
				});

				healthy = true;
				statusDesc = "running";
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				healthy = false;
				statusDesc = `degraded: ${msg}`;

				// REQ-LNP-119: Broker unavailable → degraded state
				emitListenerLog({
					event: "listener.error",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					error: `Failed to start Kafka listener: ${msg}`,
				});
				// REQ-LNP-070: Listener enters degraded state; retries handled by lifecycle manager
			}
		},

		async stop(): Promise<void> {
			running = false;
			statusDesc = "stopped";

			if (consumer) {
				try {
					await consumer.disconnect();
				} catch {
					// Non-fatal — disconnect errors should not crash the plugin
				}
				consumer = null;
			}

			emitListenerLog({
				event: "listener.stopped",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
			});
		},

		isHealthy(): boolean {
			return healthy && running;
		},

		statusDescription(): string {
			return statusDesc;
		},
	};
}
