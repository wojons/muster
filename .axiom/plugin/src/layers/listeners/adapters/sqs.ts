/**
 * SQS source adapter for the Listen/Notify Plugin.
 *
 * REQ-LNP-001: SQS listener support with queue URL and region.
 * REQ-LNP-002: SQS configuration parameters.
 * REQ-LNP-003: SQS message lifecycle (receive, process, delete on success).
 * REQ-LNP-004: AWS credentials from ambient environment; explicit creds not logged.
 * REQ-LNP-005: Structured log events for listener lifecycle.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#3.1
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-3/task-3.1
 */

import type { SqsListenerConfig } from "../../../config-loader";
import type { EventCallback, ListenerAdapter } from "../types";
import {
	emitListenerLog,
	generateEventId,
	truncatePayload,
} from "../types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating an SQS adapter. */
export interface SqsAdapterOptions {
	listenerName: string;
	config: SqsListenerConfig;
}

// ── SQS Client Interface (for testability) ───────────────────────────────

/** Minimal SQS client interface (allows mocking in tests). */
export interface SqsClient {
	receiveMessages(params: {
		QueueUrl: string;
		MaxNumberOfMessages: number;
		VisibilityTimeout: number;
		WaitTimeSeconds: number;
	}): Promise<SqsMessage[]>;
	deleteMessage(params: {
		QueueUrl: string;
		ReceiptHandle: string;
	}): Promise<void>;
}

/** A single SQS message. */
export interface SqsMessage {
	MessageId?: string;
	ReceiptHandle?: string;
	Body?: string;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create an SQS listener adapter.
 * REQ-LNP-001: Connects to SQS queue using queue_url and region.
 */
export function createSqsAdapter(
	options: SqsAdapterOptions,
	/** Optional SQS client override for testing. */
	sqsClientOverride?: SqsClient,
): ListenerAdapter {
	const { listenerName, config } = options;
	const {
		queue_url,
		region,
		poll_interval_seconds = 10,
		max_messages_per_poll = 10,
		visibility_timeout_seconds = 30,
	} = config;

	let running = false;
	let healthy = true;
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let statusDesc = "stopped";
	let sqsClient: SqsClient | null = sqsClientOverride ?? null;

	async function getSqsClient(): Promise<SqsClient> {
		if (sqsClient) return sqsClient;

		// REQ-LNP-004: Inherit AWS credentials from ambient environment
		// Dynamically import AWS SDK to avoid hard dependency
		try {
			const { SQSClient, ReceiveMessageCommand, DeleteMessageCommand } =
				await import("@aws-sdk/client-sqs" as string);

			const clientConfig: Record<string, unknown> = { region };

			// REQ-LNP-004: Explicit credentials — MUST NOT be logged
			if (config.access_key_id && config.secret_access_key) {
				clientConfig.credentials = {
					accessKeyId: config.access_key_id,
					secretAccessKey: config.secret_access_key,
				};
			}

			const client = new SQSClient(clientConfig);

			sqsClient = {
				async receiveMessages(params) {
					const cmd = new ReceiveMessageCommand({
						QueueUrl: params.QueueUrl,
						MaxNumberOfMessages: params.MaxNumberOfMessages,
						VisibilityTimeout: params.VisibilityTimeout,
						WaitTimeSeconds: params.WaitTimeSeconds,
					});
					const result = await client.send(cmd);
					return (result.Messages ?? []) as SqsMessage[];
				},
				async deleteMessage(params) {
					const cmd = new DeleteMessageCommand({
						QueueUrl: params.QueueUrl,
						ReceiptHandle: params.ReceiptHandle,
					});
					await client.send(cmd);
				},
			};
			return sqsClient;
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new Error(
				`@aws-sdk/client-sqs not available: ${msg}. Install it to use SQS listeners.`,
			);
		}
	}

	async function pollOnce(onEvent: EventCallback): Promise<void> {
		try {
			const client = await getSqsClient();
			const messages = await client.receiveMessages({
				QueueUrl: queue_url,
				MaxNumberOfMessages: max_messages_per_poll,
				VisibilityTimeout: visibility_timeout_seconds,
				WaitTimeSeconds: Math.min(poll_interval_seconds, 20), // SQS max long-poll is 20s
			});

			for (const msg of messages) {
				if (!msg.Body) continue;

				// Parse the message body
				let payload: unknown;
				try {
					payload = JSON.parse(msg.Body);
				} catch {
					payload = { raw: msg.Body };
				}

				// REQ-LNP-045: Truncate payload to 64KB
				const { payload: truncated, truncated: wasTruncated } =
					truncatePayload(payload);

				const event = {
					id: generateEventId(),
					listener_name: listenerName,
					source_type: "sqs" as const,
					source_ref: queue_url,
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

					// REQ-LNP-003: Delete message on successful processing
					if (msg.ReceiptHandle) {
						await client.deleteMessage({
							QueueUrl: queue_url,
							ReceiptHandle: msg.ReceiptHandle,
						});
					}
				} catch (err) {
					// REQ-LNP-003: Failed processing — message returns to queue after visibility timeout
					const errMsg = err instanceof Error ? err.message : String(err);
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						error: `Event processing failed: ${errMsg} — message will return to queue`,
					});
				}
			}

			healthy = true;
			statusDesc = "running";
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			healthy = false;
			statusDesc = `degraded: ${msg}`;
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: msg,
			});
		}
	}

	function schedulePoll(onEvent: EventCallback): void {
		if (!running) return;
		pollTimer = setTimeout(async () => {
			await pollOnce(onEvent);
			schedulePoll(onEvent);
		}, poll_interval_seconds * 1000);
		// unref() allows the process to exit even if polling is still scheduled
		if (pollTimer && typeof pollTimer.unref === "function") pollTimer.unref();
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

			// Start polling immediately, then on interval
			try {
				await pollOnce(onEvent);
				schedulePoll(onEvent);
				statusDesc = "running";
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				healthy = false;
				statusDesc = `degraded: ${msg}`;
				emitListenerLog({
					event: "listener.error",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					error: `Failed to start SQS listener: ${msg}`,
				});
				// REQ-LNP-070: Listener enters degraded state, retries with exponential backoff
				// (handled by lifecycle manager)
			}
		},

		async stop(): Promise<void> {
			running = false;
			if (pollTimer) {
				clearTimeout(pollTimer);
				pollTimer = null;
			}
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
