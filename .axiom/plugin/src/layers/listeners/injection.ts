/**
 * Context injection engine — pushes event data into an active agent's
 * context window via the OpenCode plugin SDK hooks.
 *
 * REQ-LNP-040: Inject via experimental.chat.messages.transform hook.
 * REQ-LNP-041: Injected message format with clear delimiter.
 * REQ-LNP-042: Sufficient context post-compaction.
 * REQ-LNP-043: Fallback injection via HTTP API POST /message.
 * REQ-LNP-044: Rubix sanitization before injection.
 * REQ-LNP-045: Payload truncation to 64KB.
 * REQ-LNP-046: Metadata fields in injected event.
 * REQ-LNP-047: Skip paused/suspended sessions.
 * REQ-LNP-048: Self-routing for agent-built listeners.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#5
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-2/task-2.2
 */

import type { EventEnvelope } from "./types";
import { emitListenerLog, truncatePayload } from "./types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating an injection engine. */
export interface InjectionEngineOptions {
	/** OpenCode HTTP API base URL for fallback injection (REQ-LNP-043). */
	openCodeBaseUrl?: string;
	/** Rubix API endpoint (REQ-LNP-065). */
	rubixEndpoint?: string;
	/** Rubix timeout in ms (REQ-LNP-065). */
	rubixTimeoutMs?: number;
	/** Whether Rubix filtering is enabled (REQ-LNP-065). */
	rubixEnabled?: boolean;
}

/** Result of an injection attempt. */
export interface InjectionResult {
	success: boolean;
	method: "transform_hook" | "http_api" | "queued";
	error?: string;
	rubix_processed?: boolean;
	rubix_flagged?: boolean;
}

/** Pending injection entry in the queue. */
export interface PendingInjection {
	event: EventEnvelope;
	session_id: string;
	queued_at: string;
	delivery_mode: "interruptive" | "non-interruptive";
}

/** The injection engine interface. */
export interface InjectionEngine {
	/**
	 * Format an event as an injected message string.
	 * REQ-LNP-041: Clear delimiter format.
	 */
	formatEventMessage(event: EventEnvelope): string;

	/**
	 * Sanitize an event payload through Rubix.
	 * REQ-LNP-044: All external payloads MUST pass through Rubix.
	 * REQ-LNP-061: Fail-open if Rubix unavailable.
	 */
	sanitizePayload(event: EventEnvelope): Promise<EventEnvelope>;

	/**
	 * Inject an event into a session via the transform hook queue.
	 * The actual injection happens when the hook fires.
	 */
	queueForInjection(
		event: EventEnvelope,
		sessionId: string,
		deliveryMode: "interruptive" | "non-interruptive",
	): void;

	/**
	 * Inject an event via the OpenCode HTTP API (fallback).
	 * REQ-LNP-043: Fallback when transform hook unavailable.
	 */
	injectViaHttpApi(
		event: EventEnvelope,
		sessionId: string,
	): Promise<InjectionResult>;

	/**
	 * Drain the pending injection queue for a session.
	 * Called by the transform hook when it fires.
	 */
	drainQueue(sessionId: string): PendingInjection[];

	/** Get all pending injections (for inspection). */
	getPendingInjections(): PendingInjection[];
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create an injection engine.
 */
export function createInjectionEngine(
	options: InjectionEngineOptions = {},
): InjectionEngine {
	const pendingQueue: PendingInjection[] = [];
	const {
		openCodeBaseUrl,
		rubixEndpoint,
		rubixTimeoutMs = 5000,
		rubixEnabled = false,
	} = options;

	return {
		formatEventMessage(event: EventEnvelope): string {
			// REQ-LNP-041: Clear delimiter format
			// REQ-LNP-042: Sufficient context post-compaction
			const { payload, truncated } = truncatePayload(event.payload);
			const payloadStr = JSON.stringify(payload, null, 2);
			const truncatedMarker = truncated ? "\n[TRUNCATED — payload exceeded 64KB]" : "";

			return [
				`[Event: ${event.listener_name}] Received at ${event.received_at}`,
				`Source: ${event.source_type} | Event ID: ${event.id}`,
				`Payload:`,
				payloadStr,
				truncatedMarker,
			]
				.filter(Boolean)
				.join("\n");
		},

		async sanitizePayload(event: EventEnvelope): Promise<EventEnvelope> {
			// REQ-LNP-044: All external payloads MUST pass through Rubix
			// REQ-LNP-061: Fail-open if Rubix unavailable
			if (!rubixEnabled || !rubixEndpoint) {
				return {
					...event,
					rubix: { processed: false, modified: false, flagged: false },
				};
			}

			try {
				const controller = new AbortController();
				const timer = setTimeout(() => controller.abort(), rubixTimeoutMs);

				const response = await fetch(rubixEndpoint, {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ payload: event.payload }),
					signal: controller.signal,
				});
				clearTimeout(timer);

				if (!response.ok) {
					// REQ-LNP-061: Fail-open — log warning and proceed
					emitListenerLog({
						event: "listener.error",
						listener_name: event.listener_name,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						error: `Rubix returned ${response.status} — proceeding without sanitization`,
					});
					return {
						...event,
						rubix: { processed: false, modified: false, flagged: false },
					};
				}

				const result = (await response.json()) as {
					payload: unknown;
					modified: boolean;
					flagged: boolean;
					risk_level?: string;
				};

				// REQ-LNP-064: High-risk payload handling
				if (result.flagged) {
					emitListenerLog({
						event: "listener.event_rubix_flagged",
						listener_name: event.listener_name,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						rubix_flagged: true,
					});
				}

				return {
					...event,
					payload: result.payload,
					rubix: {
						processed: true,
						modified: result.modified,
						flagged: result.flagged,
						risk_level: result.risk_level,
					},
				};
			} catch (err) {
				// REQ-LNP-061: Fail-open — log warning and proceed
				const msg = err instanceof Error ? err.message : String(err);
				emitListenerLog({
					event: "listener.error",
					listener_name: event.listener_name,
					timestamp: new Date().toISOString(),
					event_id: event.id,
					error: `Rubix unavailable: ${msg} — proceeding without sanitization`,
				});
				return {
					...event,
					rubix: { processed: false, modified: false, flagged: false },
				};
			}
		},

		queueForInjection(
			event: EventEnvelope,
			sessionId: string,
			deliveryMode: "interruptive" | "non-interruptive",
		): void {
			pendingQueue.push({
				event,
				session_id: sessionId,
				queued_at: new Date().toISOString(),
				delivery_mode: deliveryMode,
			});
		},

		async injectViaHttpApi(
			event: EventEnvelope,
			sessionId: string,
		): Promise<InjectionResult> {
			// REQ-LNP-043: Fallback injection via OpenCode HTTP API
			if (!openCodeBaseUrl) {
				return {
					success: false,
					method: "http_api",
					error: "No OpenCode base URL configured for fallback injection",
				};
			}

			try {
				const message = this.formatEventMessage(event);
				const url = `${openCodeBaseUrl}/session/${sessionId}/message`;
				const response = await fetch(url, {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ content: message, role: "system" }),
				});

				if (!response.ok) {
					return {
						success: false,
						method: "http_api",
						error: `HTTP API returned ${response.status}`,
					};
				}

				return { success: true, method: "http_api" };
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				return { success: false, method: "http_api", error: msg };
			}
		},

		drainQueue(sessionId: string): PendingInjection[] {
			const drained: PendingInjection[] = [];
			let i = 0;
			while (i < pendingQueue.length) {
				if (pendingQueue[i].session_id === sessionId) {
					drained.push(...pendingQueue.splice(i, 1));
				} else {
					i++;
				}
			}
			return drained;
		},

		getPendingInjections(): PendingInjection[] {
			return [...pendingQueue];
		},
	};
}
