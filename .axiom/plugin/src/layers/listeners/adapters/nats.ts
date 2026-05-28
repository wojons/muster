/**
 * NATS source adapter for the Listen/Notify Plugin.
 *
 * REQ-LNP-100: NATS listener support — connect to NATS server using `servers` URL list.
 *              Supports both core NATS and JetStream (OD-LNP-06 resolved).
 * REQ-LNP-101: Configuration parameters (servers, subject, queue_group, jetstream, etc.).
 * REQ-LNP-102: Connection lifecycle — reconnect with exponential backoff (max 5 attempts).
 * REQ-LNP-103: JetStream explicit acknowledgment — ack on success, NAK on failure.
 * REQ-LNP-104: Core NATS is at-most-once delivery — no ack/nak semantics.
 * REQ-LNP-105: Subject wildcard support — * (single token) and > (one or more tokens).
 * REQ-LNP-106: Credentials MUST NOT be logged.
 * REQ-LNP-107: Emit same structured log events as other adapters.
 * REQ-LNP-108: Dynamic import of `nats` package — clear error if not installed.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#13.1
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-1/task-1.2
 */

import type { NatsListenerConfig } from "../../../config-loader";
import type { EventCallback, ListenerAdapter } from "../types";
import {
	emitListenerLog,
	generateEventId,
	truncatePayload,
} from "../types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating a NATS adapter. */
export interface NatsAdapterOptions {
	listenerName: string;
	config: NatsListenerConfig;
}

/**
 * Minimal interface for the dynamically-imported `nats` npm module.
 * REQ-LNP-106: credsAuthenticator() is the correct v2 API for credentials files.
 * REQ-LNP-108: Dynamic import — this interface allows test injection without
 *              requiring the real `nats` package to be installed.
 */
export interface NatsModule {
	connect(opts: Record<string, unknown>): Promise<NatsConnection>;
	/**
	 * REQ-LNP-106: nats.js v2 API — reads credentials content (Uint8Array) and
	 * returns an Authenticator object for use in ConnectOptions.authenticator.
	 */
	credsAuthenticator(creds: Uint8Array): unknown; // Authenticator type from nats.js
}

/**
 * A NATS connection status event (subset of nats.js v2 StatusEvent).
 * REQ-LNP-102: Used to detect post-start disconnect and reconnect exhaustion.
 */
export interface NatsStatusEvent {
	type: string;
	data?: unknown;
}

/**
 * Minimal NATS connection interface (allows mocking in tests).
 * Covers both core NATS subscribe and JetStream subscribe paths.
 */
export interface NatsConnection {
	/** Subscribe to a core NATS subject (REQ-LNP-104: at-most-once). */
	subscribe(
		subject: string,
		opts?: { queue?: string; callback?: NatsMsgCallback },
	): NatsSubscription;
	/** Get a JetStream client for at-least-once delivery (REQ-LNP-103). */
	jetstream(): JetStreamClient;
	/** Drain and close the connection. */
	drain(): Promise<void>;
	/** Whether the connection is closed. */
	isClosed(): boolean;
	/**
	 * Async iterator of connection status events.
	 * REQ-LNP-102: Used to detect post-start disconnect / reconnect exhaustion.
	 * The nats.js v2 package exposes this as `nc.status()`.
	 */
	status(): AsyncIterable<NatsStatusEvent>;
}

/** Callback type for core NATS message delivery. */
export type NatsMsgCallback = (err: Error | null, msg: NatsMsg) => void;

/** A single NATS message (core or JetStream). */
export interface NatsMsg {
	/** Raw message data as Uint8Array. */
	data: Uint8Array;
	/** Message subject. */
	subject: string;
	/** Acknowledge the message (JetStream only — REQ-LNP-103). */
	ack?(): void;
	/** Negative-acknowledge the message for redelivery (JetStream only — REQ-LNP-103). */
	nak?(): void;
}

/** A NATS subscription handle. */
export interface NatsSubscription {
	/** Async iterator over messages. */
	[Symbol.asyncIterator](): AsyncIterator<NatsMsg>;
	/** Unsubscribe / drain the subscription. */
	unsubscribe(): void;
}

/** Minimal JetStream client interface. */
export interface JetStreamClient {
	subscribe(
		subject: string,
		opts?: JetStreamSubscribeOptions,
	): Promise<NatsSubscription>;
}

/** JetStream subscribe options. */
export interface JetStreamSubscribeOptions {
	config?: {
		durable_name?: string;
		deliver_policy?: string;
		ack_wait?: number;
		filter_subject?: string;
	};
	stream?: string;
	queue?: string;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create a NATS listener adapter.
 *
 * REQ-LNP-100: Connects to NATS server(s) using the `servers` URL list.
 * REQ-LNP-108: Dynamically imports the `nats` npm package; emits clear error if missing.
 */
export function createNatsAdapter(
	options: NatsAdapterOptions,
	/** Optional NATS connection override for testing (REQ-LNP-108 bypass). */
	natsConnectionOverride?: NatsConnection,
	/** Optional nats module override for testing (REQ-LNP-106 / REQ-LNP-108 DI). */
	natsModuleOverride?: NatsModule,
): ListenerAdapter {
	const { listenerName, config } = options;
	const {
		servers,
		subject,
		queue_group,
		jetstream: useJetStream = false,
		stream,
		consumer,
		deliver_policy = "new",
		ack_wait_seconds = 30,
		// REQ-LNP-106: credentials_file path is used but NEVER logged
		credentials_file,
		tls_cert,
		tls_key,
		tls_ca,
	} = config;

	let running = false;
	let stopping = false; // REQ-LNP-102: Set before running=false in stop() to prevent spurious degraded log
	let healthy = false;
	let statusDesc = "stopped";
	let nc: NatsConnection | null = natsConnectionOverride ?? null;
	let activeSub: NatsSubscription | null = null;

	/**
	 * Dynamically import the `nats` package and connect.
	 * REQ-LNP-108: If `nats` is not installed, emit a clear error message.
	 * REQ-LNP-102: Uses NATS client built-in reconnect (maxReconnectAttempts: 5).
	 * REQ-LNP-106: credentials_file is read at startup only; content is NEVER logged.
	 */
	async function connect(): Promise<NatsConnection> {
		if (nc) return nc;

		let natsModule: NatsModule;

		if (natsModuleOverride) {
			natsModule = natsModuleOverride;
		} else {
			try {
				// REQ-LNP-108: Dynamic import — avoids hard dependency on `nats` package
				natsModule = await import("nats" as string) as NatsModule;
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				throw new Error(
					`nats package not available: ${msg}. Install it with: npm install nats (or bun add nats). Required for NATS listeners.`,
				);
			}
		}

		// Build connection options
		// REQ-LNP-106: credentials_file is passed as a path — the value is NOT logged anywhere
		const connectOpts: Record<string, unknown> = {
			servers,
			// REQ-LNP-102: Built-in reconnect with max 5 attempts
			reconnect: true,
			maxReconnectAttempts: 5,
			reconnectTimeWait: 1000, // base backoff 1s; NATS client applies exponential internally
		};

		// REQ-LNP-106: Use credsAuthenticator() per nats.js v2 API.
		// The plain-object form { creds: path } is NOT the correct API shape.
		// credsAuthenticator(content) takes the file content as a Uint8Array and
		// returns an Authenticator object that the NATS client uses for the handshake.
		// The file content is NEVER logged — only the path appears in error messages.
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-106 plan=step-fix-nats-002
		if (credentials_file) {
			try {
				const { readFileSync } = await import("node:fs");
				const credsContent = readFileSync(credentials_file, "utf-8");
				connectOpts.authenticator = natsModule.credsAuthenticator(
					new TextEncoder().encode(credsContent),
				);
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				throw new Error(
					`Failed to load NATS credentials from ${credentials_file}: ${msg}`,
				);
			}
		}

		// TLS configuration (REQ-LNP-101)
		if (tls_cert || tls_key || tls_ca) {
			connectOpts.tls = {
				...(tls_cert ? { certFile: tls_cert } : {}),
				...(tls_key ? { keyFile: tls_key } : {}),
				...(tls_ca ? { caFile: tls_ca } : {}),
			};
		}

		nc = await natsModule.connect(connectOpts);
		return nc;
	}

	/**
	 * Process a single NATS message and deliver it as an EventEnvelope.
	 * REQ-LNP-103: For JetStream, ack on success, nak on failure.
	 * REQ-LNP-104: For core NATS, no ack/nak (at-most-once).
	 */
	async function processMessage(
		msg: NatsMsg,
		onEvent: EventCallback,
		isJetStream: boolean,
	): Promise<void> {
		// Decode message data
		const rawText = new TextDecoder().decode(msg.data);

		// Parse payload — JSON if possible, else { raw: string }
		let payload: unknown;
		try {
			payload = JSON.parse(rawText);
		} catch {
			payload = { raw: rawText };
		}

		// REQ-LNP-045 (via truncatePayload): Truncate to 64KB
		const { payload: truncated, truncated: wasTruncated } = truncatePayload(payload);

		const event = {
			id: generateEventId(),
			listener_name: listenerName,
			source_type: "nats" as const,
			// REQ-LNP-105: subject includes wildcard tokens as-is for traceability
			source_ref: subject,
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

		try {
			await onEvent(event);

			// REQ-LNP-103: JetStream — ack only after successful processing
			if (isJetStream && msg.ack) {
				msg.ack();
			}
			// REQ-LNP-104: Core NATS — at-most-once, no ack needed
		} catch (err) {
			const errMsg = err instanceof Error ? err.message : String(err);

			// REQ-LNP-103: JetStream — NAK on failure so message is redelivered
			if (isJetStream && msg.nak) {
				msg.nak();
			}

			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				event_id: event.id,
				error: isJetStream
					? `Event processing failed: ${errMsg} — message NAK'd for redelivery`
					: `Event processing failed: ${errMsg} — core NATS at-most-once, message lost`,
			});
		}
	}

	/**
	 * Subscribe using core NATS (at-most-once delivery).
	 * REQ-LNP-104: Core NATS has no ack semantics — document explicitly.
	 * REQ-LNP-105: Subject wildcards (* and >) are passed through to NATS directly.
	 */
	async function subscribeCoreNats(
		connection: NatsConnection,
		onEvent: EventCallback,
	): Promise<NatsSubscription> {
		// REQ-LNP-104: Core NATS is at-most-once delivery.
		// Messages are delivered once; if processing fails, the message is NOT redelivered.
		// Use JetStream (jetstream: true) for at-least-once delivery guarantees.
		const sub = connection.subscribe(subject, {
			queue: queue_group,
		});

		// Consume messages asynchronously
		void (async () => {
			for await (const msg of sub) {
				if (!running) break;
				await processMessage(msg, onEvent, false);
			}
		})();

		return sub;
	}

	/**
	 * Subscribe using JetStream (at-least-once delivery).
	 * REQ-LNP-103: Explicit ack/nak per message.
	 * REQ-LNP-102: JetStream consumer resumes from last ack'd message after reconnect.
	 */
	async function subscribeJetStream(
		connection: NatsConnection,
		onEvent: EventCallback,
	): Promise<NatsSubscription> {
		if (!stream) {
			throw new Error(
				`NATS listener "${listenerName}": "stream" is required when jetstream: true (REQ-LNP-101)`,
			);
		}

		const js = connection.jetstream();

		// REQ-LNP-102: Durable consumer name ensures resume from last ack'd message after reconnect
		const sub = await js.subscribe(subject, {
			stream,
			queue: queue_group,
			config: {
				durable_name: consumer,
				deliver_policy,
				// BL-007: Cap ack_wait_seconds to prevent Number.MAX_SAFE_INTEGER overflow
				// ack_wait_seconds * 1_000_000_000 overflows for values > ~9,007,199 seconds (~104 days)
				// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-101 plan=phase-1/task-1.4 jira_ref=DEX-434
				ack_wait: Math.min(ack_wait_seconds ?? 30, 9_000_000) * 1_000_000_000,
				filter_subject: subject,
			},
		});

		// Consume messages asynchronously
		void (async () => {
			for await (const msg of sub) {
				if (!running) break;
				await processMessage(msg, onEvent, true);
			}
		})();

		return sub;
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
			const connection = await connect();
			if (useJetStream) {
				activeSub = await subscribeJetStream(connection, onEvent);
			} else {
				activeSub = await subscribeCoreNats(connection, onEvent);
			}

			healthy = true;
			statusDesc = "running";

			// REQ-LNP-102: Monitor connection status for post-start degraded detection.
			// The nats.js v2 `status()` async iterator yields events including
			// `disconnect`, `reconnecting`, `reconnect`, `error`, and `close`.
			// When the iterator completes (connection permanently closed after reconnect
			// exhaustion), we transition to degraded state so the lifecycle manager
			// can detect the silent failure.
			// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=step-fix-nats-001
			void (async () => {
				try {
					for await (const s of connection.status()) {
						if (s.type === "disconnect" || s.type === "error") {
							emitListenerLog({
								event: "listener.error",
								listener_name: listenerName,
								timestamp: new Date().toISOString(),
								error: `NATS connection status: ${s.type} — ${String(s.data ?? "")}`,
							});
						} else if (s.type === "reconnect") {
							// Recovered — restore healthy state
							healthy = true;
							statusDesc = "running";
						}
					}
				} catch {
					// Status iterator closed — connection is permanently closed
				}
				// If we reach here, the status iterator has completed, meaning the
				// connection is permanently closed (reconnect exhaustion or explicit close).
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=step-da-001
				if (running && !stopping) {
					// REQ-LNP-102: Only emit degraded state if we're still running (not stopping cleanly).
					// The `stopping` flag is set before `running=false` in stop(), so if drain()
					// causes the status iterator to complete before running=false is observed,
					// the !stopping guard prevents a spurious "degraded" log on clean shutdown.
					healthy = false;
					statusDesc = "degraded: connection closed after reconnect exhaustion";
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						error: "NATS connection permanently closed — entering degraded state",
					});
				}
			})();
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			healthy = false;
			running = false;
			statusDesc = `degraded: ${msg}`;

			// REQ-LNP-107: Emit structured error log
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: `Failed to start NATS listener: ${msg}`,
			});

			// Re-throw so lifecycle manager can handle degraded state + backoff (REQ-LNP-102)
			throw err;
		}
		},

		async stop(): Promise<void> {
			stopping = true;  // REQ-LNP-102: Set before running=false to prevent spurious degraded log
			// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=step-da-001
			running = false;
			healthy = false;

			// Unsubscribe first
			if (activeSub) {
				try {
					activeSub.unsubscribe();
				} catch {
					// Ignore unsubscribe errors on shutdown
				}
				activeSub = null;
			}

			// Drain and close the connection
			if (nc && !nc.isClosed()) {
				try {
					await nc.drain();
				} catch {
					// Ignore drain errors on shutdown
				}
			}
			nc = null;

			statusDesc = "stopped";

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
