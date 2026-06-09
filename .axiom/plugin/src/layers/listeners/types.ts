/**
 * Core types for the Listen/Notify Plugin subsystem (L7).
 *
 * Defines the event envelope, listener state, and delivery contracts
 * per specs/88-Listen-Notify-Plugin.md Appendix A.
 *
 * REQ-LNP-046: Injected event MUST include listener_name, event_id,
 *              received_at, source_type metadata fields.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-1/task-1.2
 */

import type { ListenerEntry, ListenerSourceType } from "../../config-loader";

// ── Event Envelope ───────────────────────────────────────────────────────

/** Unique event ID — format: evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN> per specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability. */
export type EventId = string;

/** Delivery status for an event. */
export type DeliveryStatus = "pending" | "delivered" | "failed" | "expired" | "dropped";

/** Delivery mode for an event. */
export type DeliveryMode = "interruptive" | "non-interruptive";

/**
 * Canonical event envelope (Appendix A of spec 88).
 * REQ-LNP-046: Must include listener_name, event_id, received_at, source_type.
 */
export interface EventEnvelope {
	/** Unique event ID (evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN>). */
	id: EventId;
	/** Which listener captured this event. */
	listener_name: string;
	/** Adapter type: sqs, http, script. */
	source_type: ListenerSourceType;
	/** Source-specific reference (queue ARN, URL path, script path). */
	source_ref: string;
	/** When the plugin received the event (ISO 8601). */
	received_at: string;
	/** Normalized JSON payload (truncated to 64KB per REQ-LNP-045). */
	payload: unknown;
	/** Whether the payload was truncated (REQ-LNP-045). */
	truncated?: boolean;
	/** Routing information resolved by the router. */
	routing?: {
		target_session_id?: string;
		match_rule?: string;
	};
	/** Delivery tracking. */
	delivery: {
		mode: DeliveryMode;
		status: DeliveryStatus;
		attempts: number;
		last_attempt_at: string | null;
	};
	/** Rubix sanitization result (REQ-LNP-063). */
	rubix?: {
		processed: boolean;
		modified: boolean;
		flagged: boolean;
		risk_level?: string;
	};
}

// ── Listener State ───────────────────────────────────────────────────────

/** Operational state of a listener. */
export type ListenerStatus =
	| "starting"
	| "running"
	| "degraded"
	| "stopped"
	| "error";

/** Runtime state for a managed listener. */
export interface ListenerState {
	/** Unique name from config. */
	name: string;
	/** Current operational status. */
	status: ListenerStatus;
	/** When the listener was started. */
	started_at: string | null;
	/** When the listener was stopped. */
	stopped_at: string | null;
	/** Session ID that created this listener (for agent-built listeners). */
	creator_session_id?: string;
	/** Total events received. */
	events_received: number;
	/** Total events delivered. */
	events_delivered: number;
	/** Total events dropped. */
	events_dropped: number;
	/** Last health check timestamp. */
	last_health_check?: string;
	/** Last error message. */
	last_error?: string;
	/** The config entry for this listener. */
	config: ListenerEntry;
}

// ── Adapter Interface ────────────────────────────────────────────────────

/** Callback invoked when an adapter receives an event. */
export type EventCallback = (event: EventEnvelope) => Promise<void>;

/** Interface all source adapters must implement. */
export interface ListenerAdapter {
	/** Start the adapter (begin listening). */
	start(onEvent: EventCallback): Promise<void>;
	/** Stop the adapter (clean shutdown). */
	stop(): Promise<void>;
	/** Health check — returns true if healthy. */
	isHealthy(): boolean;
	/** Human-readable status description. */
	statusDescription(): string;
}

// ── Structured Log Events ────────────────────────────────────────────────

/** Structured log event types per REQ-LNP-005. */
export type ListenerLogEvent =
	| "listener.started"
	| "listener.stopped"
	| "listener.event_received"
	| "listener.event_delivered"
	| "listener.event_dropped"
	| "listener.event_routed"
	| "listener.event_rubix_flagged"
	| "listener.agent_script_rejected"
	| "listener.error"
	/** REQ-LNP-127: EventBridge rule lifecycle events. */
	| "listener.eventbridge_rule_created"
	| "listener.eventbridge_rule_deleted"
	/** REQ-LNP-116: Kafka-specific events. */
	| "listener.kafka_rebalance"
	| "listener.kafka_lag"
	/** REQ-LNP-145: Runner trigger event. */
	| "listener.runner_triggered";

/** Structured log entry for listener events. */
export interface ListenerLogEntry {
	event: ListenerLogEvent;
	listener_name: string;
	timestamp: string;
	event_id?: EventId;
	session_id?: string;
	/** REQ-LNP-145: Work item ID for runner_triggered events. */
	work_item?: string;
	/** REQ-LNP-145: Generated UUID for the runner execution. */
	runner_session_id?: string;
	reason?: string;
	error?: string;
	delivery_mode?: DeliveryMode;
	truncated?: boolean;
	rubix_modified?: boolean;
	rubix_flagged?: boolean;
}

// ── Utility ──────────────────────────────────────────────────────────────

/** Maximum event payload size in bytes (REQ-LNP-045, REQ-LNP-075). */
export const MAX_PAYLOAD_BYTES = 64 * 1024; // 64KB

/** Maximum concurrent listeners per session (REQ-LNP-073). */
export const MAX_LISTENERS_PER_SESSION = 10;

/** Maximum concurrent listeners per machine (REQ-LNP-073). */
export const MAX_LISTENERS_PER_MACHINE = 50;

/** Interruptive rate limit: 1 event per 5 seconds per session (REQ-LNP-053). */
export const INTERRUPTIVE_RATE_LIMIT_MS = 5_000;

/** Interruptive queue overflow limit (REQ-LNP-053). */
export const INTERRUPTIVE_QUEUE_MAX = 100;

/** Non-interruptive rate limit: 10 events per minute per session (REQ-LNP-054). */
export const NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE = 10;

/** Non-interruptive queue maximum depth per session (REQ-LNP-054). */
// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-054 plan=step-fix-005
export const NON_INTERRUPTIVE_QUEUE_MAX = 200;

/**
 * Maximum events injected per messages.transform call (REQ-LNP-040).
 * Prevents context-window bomb when the injection queue is large.
 * Remaining events are carried over to the next transform invocation.
 */
// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-040 plan=step-fix-012
export const MAX_EVENTS_PER_INJECT = 10;

/** Listener health check interval in ms (REQ-LNP-076). */
export const HEALTH_CHECK_INTERVAL_MS = 30_000;

/** Orphan detection interval in ms (REQ-LNP-077). */
export const ORPHAN_DETECTION_INTERVAL_MS = 60_000;

/**
 * Per-millisecond monotonic sequence counter for generateEventId.
 * Resets to 0 whenever the timestamp changes, ensuring uniqueness within
 * each millisecond. Hard limit: 999 unique IDs per millisecond (seq 001–999).
 *
 * axiom:trace work_item=sse-event-id-format-01 spec=specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability jira_ref=SWDE-17 plan=phase-1/task-1/step-INJ-CRIT-1
 */
let _eventIdLastTimestamp = "";
let _eventIdSequence = 0;

/**
 * Generate a unique event ID conforming to the spec contract.
 *
 * Format: evt_<YYYY-MM-DDTHH-MM-SS-mmmZ>_<NNN>
 * Example: evt_2026-02-06T10-31-00-123Z_001
 *
 * Per specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability:
 * - <timestamp> is the event time in YYYY-MM-DDTHH-MM-SS-mmmZ format
 *   (hyphens replacing colons/dots for URL safety).
 * - <NNN> is a zero-padded 3-digit per-millisecond sequence number (001–999).
 *   The counter resets to 1 whenever the millisecond timestamp changes.
 *   Hard limit: 999 unique IDs per millisecond.
 *
 * axiom:trace work_item=sse-event-id-format-01 spec=specs/30-External-API-And-Realtime.md#sse-event-id-and-resumability jira_ref=SWDE-17 plan=phase-1/task-1/step-INJ-CRIT-1
 */
export function generateEventId(): EventId {
	const now = new Date(Date.now());
	// Build YYYY-MM-DDTHH-MM-SS-mmmZ (hyphens replace colons and dots)
	const iso = now.toISOString(); // "2026-02-06T10:31:00.123Z"
	const safeTimestamp = iso
		.replace(/:/g, "-")   // colons → hyphens
		.replace(/\./g, "-"); // dot (milliseconds separator) → hyphen
	// Reset sequence when the millisecond timestamp changes (per-ms counter)
	if (safeTimestamp !== _eventIdLastTimestamp) {
		_eventIdLastTimestamp = safeTimestamp;
		_eventIdSequence = 0;
	}
	// Increment; wrap at 999 to stay 3 digits (hard limit: 999 IDs/ms)
	_eventIdSequence = (_eventIdSequence % 999) + 1;
	const seq = String(_eventIdSequence).padStart(3, "0");
	return `evt_${safeTimestamp}_${seq}`;
}

/**
 * Truncate a payload to MAX_PAYLOAD_BYTES.
 * REQ-LNP-045: Truncated payloads include a [TRUNCATED] marker.
 */
export function truncatePayload(
	payload: unknown,
): { payload: unknown; truncated: boolean } {
	const serialized = JSON.stringify(payload);
	if (serialized.length <= MAX_PAYLOAD_BYTES) {
		return { payload, truncated: false };
	}
	const truncated = serialized.slice(0, MAX_PAYLOAD_BYTES - 20) + "...[TRUNCATED]";
	try {
		return { payload: JSON.parse(truncated), truncated: true };
	} catch {
		return { payload: truncated, truncated: true };
	}
}

/**
 * Emit a structured log entry for listener events.
 * REQ-LNP-005: Structured log events per specs/25-Structured-Logging-Events.md.
 */
export function emitListenerLog(entry: ListenerLogEntry): void {
	// Use structured JSON logging compatible with specs/25-Structured-Logging-Events.md
	const logLine = JSON.stringify({
		...entry,
		service: "axiom-plugin",
		layer: "L7-listeners",
	});
	// Use stderr to avoid polluting stdout (which OpenCode may parse)
	process.stderr.write(logLine + "\n");
}
