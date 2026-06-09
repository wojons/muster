/**
 * Delivery queue and rate limiter for the Listen/Notify Plugin.
 *
 * REQ-LNP-050: Two delivery modes: interruptive and non-interruptive.
 * REQ-LNP-051: Interruptive delivery — immediate injection.
 * REQ-LNP-052: Non-interruptive delivery — queued until natural pause.
 * REQ-LNP-053: Interruptive rate limit: 1 event/5s per session; queue max 100.
 * REQ-LNP-054: Non-interruptive rate limit: 10 events/min per session.
 * REQ-LNP-055: Injection during tool execution — queue behind current tool.
 * REQ-LNP-057: Dynamic delivery mode switching.
 * REQ-LNP-058: Delivery mode in log events.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#6
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-2/task-2.3
 */

import type { EventEnvelope } from "./types";
import {
	emitListenerLog,
	INTERRUPTIVE_QUEUE_MAX,
	INTERRUPTIVE_RATE_LIMIT_MS,
	NON_INTERRUPTIVE_QUEUE_MAX,
	NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE,
} from "./types";

// ── Types ────────────────────────────────────────────────────────────────

/** A queued delivery item. */
export interface QueuedDelivery {
	event: EventEnvelope;
	session_id: string;
	mode: "interruptive" | "non-interruptive";
	queued_at: number; // ms timestamp
}

/** Per-session rate limit state. */
interface SessionRateState {
	/** Timestamp of last interruptive delivery (ms). */
	last_interruptive_at: number;
	/** Interruptive overflow queue. */
	interruptive_queue: QueuedDelivery[];
	/** Non-interruptive delivery timestamps in the last minute. */
	non_interruptive_timestamps: number[];
}

/** Options for creating a delivery queue. */
export interface DeliveryQueueOptions {
	/** Override interruptive rate limit in ms (default: 5000). */
	interruptiveRateLimitMs?: number;
	/** Override non-interruptive rate limit per minute (default: 10). */
	nonInterruptiveRateLimitPerMinute?: number;
	/** Override interruptive queue max (default: 100). */
	interruptiveQueueMax?: number;
	/** Maximum non-interruptive queue depth per session (default: 200). */
	nonInterruptiveQueueMax?: number;
}

/** Result of attempting to deliver an event. */
export interface DeliveryAttemptResult {
	/** Whether the event should be delivered now. */
	deliver_now: boolean;
	/** Whether the event was queued for later. */
	queued: boolean;
	/** Whether the event was dropped due to rate limiting. */
	dropped: boolean;
	/** Reason for dropping. */
	drop_reason?: string;
}

/** The delivery queue interface. */
export interface DeliveryQueue {
	/**
	 * Attempt to deliver an event to a session.
	 * Returns whether to deliver immediately, queue, or drop.
	 */
	attempt(
		event: EventEnvelope,
		sessionId: string,
		mode: "interruptive" | "non-interruptive",
	): DeliveryAttemptResult;

	/**
	 * Drain the interruptive queue for a session (called at rate limit cadence).
	 * REQ-LNP-053: Excess events delivered at rate limit cadence.
	 */
	drainInterruptiveQueue(sessionId: string): QueuedDelivery[];

	/**
	 * Get all queued non-interruptive events for a session.
	 * REQ-LNP-052: Delivered at next natural pause.
	 */
	drainNonInterruptiveQueue(sessionId: string): QueuedDelivery[];

	/** Get current queue depth for a session. */
	getQueueDepth(sessionId: string): { interruptive: number; non_interruptive: number };

	/** Clear all state for a session (on session end). */
	clearSession(sessionId: string): void;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create a delivery queue with rate limiting.
 */
export function createDeliveryQueue(
	options: DeliveryQueueOptions = {},
): DeliveryQueue {
	const {
		interruptiveRateLimitMs = INTERRUPTIVE_RATE_LIMIT_MS,
		nonInterruptiveRateLimitPerMinute = NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE,
		interruptiveQueueMax = INTERRUPTIVE_QUEUE_MAX,
		nonInterruptiveQueueMax = NON_INTERRUPTIVE_QUEUE_MAX,
	} = options;

	const sessionState = new Map<string, SessionRateState>();
	const nonInterruptiveQueues = new Map<string, QueuedDelivery[]>();

	function getOrCreateState(sessionId: string): SessionRateState {
		if (!sessionState.has(sessionId)) {
			sessionState.set(sessionId, {
				last_interruptive_at: 0,
				interruptive_queue: [],
				non_interruptive_timestamps: [],
			});
		}
		return sessionState.get(sessionId)!;
	}

	function getNonInterruptiveQueue(sessionId: string): QueuedDelivery[] {
		if (!nonInterruptiveQueues.has(sessionId)) {
			nonInterruptiveQueues.set(sessionId, []);
		}
		return nonInterruptiveQueues.get(sessionId)!;
	}

	return {
		attempt(
			event: EventEnvelope,
			sessionId: string,
			mode: "interruptive" | "non-interruptive",
		): DeliveryAttemptResult {
			const now = Date.now();
			const state = getOrCreateState(sessionId);

			if (mode === "interruptive") {
				// REQ-LNP-053: Max 1 interruptive event per 5 seconds per session
				const timeSinceLast = now - state.last_interruptive_at;
				if (timeSinceLast >= interruptiveRateLimitMs) {
					// Deliver immediately
					state.last_interruptive_at = now;
					emitListenerLog({
						event: "listener.event_delivered",
						listener_name: event.listener_name,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						session_id: sessionId,
						delivery_mode: "interruptive",
					});
					return { deliver_now: true, queued: false, dropped: false };
				}

				// Rate limited — queue or drop
				if (state.interruptive_queue.length >= interruptiveQueueMax) {
					// REQ-LNP-053: Drop oldest events when queue overflows
					const dropped = state.interruptive_queue.shift();
					if (dropped) {
						emitListenerLog({
							event: "listener.event_dropped",
							listener_name: dropped.event.listener_name,
							timestamp: new Date().toISOString(),
							event_id: dropped.event.id,
							session_id: sessionId,
							reason: "rate_limited",
						});
					}
				}

				state.interruptive_queue.push({
					event,
					session_id: sessionId,
					mode: "interruptive",
					queued_at: now,
				});
				return { deliver_now: false, queued: true, dropped: false };
			}

			// Non-interruptive mode
			// REQ-LNP-054: Max 10 events per minute per session
			const oneMinuteAgo = now - 60_000;
			state.non_interruptive_timestamps = state.non_interruptive_timestamps.filter(
				(ts) => ts > oneMinuteAgo,
			);

			if (state.non_interruptive_timestamps.length >= nonInterruptiveRateLimitPerMinute) {
				// Rate limited — drop
				emitListenerLog({
					event: "listener.event_dropped",
					listener_name: event.listener_name,
					timestamp: new Date().toISOString(),
					event_id: event.id,
					session_id: sessionId,
					reason: "rate_limited",
				});
				return {
					deliver_now: false,
					queued: false,
					dropped: true,
					drop_reason: "rate_limited",
				};
			}

			// Queue for delivery at next natural pause
			state.non_interruptive_timestamps.push(now);
			const queue = getNonInterruptiveQueue(sessionId);

			// REQ-LNP-054: Enforce queue depth cap — drop oldest if at max
			// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-054 plan=step-fix-005
			if (queue.length >= nonInterruptiveQueueMax) {
				const dropped = queue.shift();
				if (dropped) {
					emitListenerLog({
						event: "listener.event_dropped",
						listener_name: dropped.event.listener_name,
						timestamp: new Date().toISOString(),
						event_id: dropped.event.id,
						session_id: sessionId,
						reason: "rate_limited",
					});
				}
			}

			queue.push({
				event,
				session_id: sessionId,
				mode: "non-interruptive",
				queued_at: now,
			});
			return { deliver_now: false, queued: true, dropped: false };
		},

		drainInterruptiveQueue(sessionId: string): QueuedDelivery[] {
			const state = sessionState.get(sessionId);
			if (!state || state.interruptive_queue.length === 0) return [];

			const now = Date.now();
			const timeSinceLast = now - state.last_interruptive_at;
			if (timeSinceLast < interruptiveRateLimitMs) return [];

			// Deliver one item from the queue
			const item = state.interruptive_queue.shift();
			if (!item) return [];

			state.last_interruptive_at = now;
			emitListenerLog({
				event: "listener.event_delivered",
				listener_name: item.event.listener_name,
				timestamp: new Date().toISOString(),
				event_id: item.event.id,
				session_id: sessionId,
				delivery_mode: "interruptive",
			});
			return [item];
		},

		drainNonInterruptiveQueue(sessionId: string): QueuedDelivery[] {
			const queue = nonInterruptiveQueues.get(sessionId);
			if (!queue || queue.length === 0) return [];

			const drained = [...queue];
			nonInterruptiveQueues.set(sessionId, []);

			for (const item of drained) {
				emitListenerLog({
					event: "listener.event_delivered",
					listener_name: item.event.listener_name,
					timestamp: new Date().toISOString(),
					event_id: item.event.id,
					session_id: sessionId,
					delivery_mode: "non-interruptive",
				});
			}
			return drained;
		},

		getQueueDepth(sessionId: string): { interruptive: number; non_interruptive: number } {
			const state = sessionState.get(sessionId);
			const niQueue = nonInterruptiveQueues.get(sessionId);
			return {
				interruptive: state?.interruptive_queue.length ?? 0,
				non_interruptive: niQueue?.length ?? 0,
			};
		},

		clearSession(sessionId: string): void {
			sessionState.delete(sessionId);
			nonInterruptiveQueues.delete(sessionId);
		},
	};
}
