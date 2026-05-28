/**
 * Session router — evaluates incoming events against declarative rules
 * and dispatches them to the correct Axiom session(s).
 *
 * REQ-LNP-030: Session routing engine.
 * REQ-LNP-031: Routing matching strategies (work_item, tag, session_id, self).
 * REQ-LNP-032: Fallback session configuration.
 * REQ-LNP-033: Fan-out to multiple matching sessions.
 * REQ-LNP-034: Variable substitution in routing rules ($event.field.path).
 * REQ-LNP-035: Cross-session routing via file-based coordination.
 * REQ-LNP-036: Filter evaluation before routing.
 * REQ-LNP-037: Event pattern matching operators.
 * REQ-LNP-038: match_all vs match_any logic.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#4
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-2/task-2.1
 */

import type { ListenerEntry, ListenerFilter, ListenerRouting } from "../../config-loader";
import type { EventEnvelope } from "./types";
import { emitListenerLog } from "./types";

// ── Types ────────────────────────────────────────────────────────────────

/** Result of routing an event. */
export interface RoutingResult {
	/** Session IDs that should receive this event. */
	target_sessions: string[];
	/** Which routing rule matched (for logging). */
	match_rule?: string;
	/** Whether the event was dropped (no session matched). */
	dropped: boolean;
	/** Reason for dropping (if dropped). */
	drop_reason?: string;
}

/** Options for creating a session router. */
export interface SessionRouterOptions {
	/** The session ID of the current session (for "self" routing). */
	currentSessionId?: string;
	/** Known active sessions (session_id -> work_item/tags). */
	activeSessions?: Map<string, SessionInfo>;
}

/** Information about an active session. */
export interface SessionInfo {
	session_id: string;
	work_item?: string;
	tags?: string[];
	paused?: boolean;
}

/** The session router interface. */
export interface SessionRouter {
	/**
	 * Route an event to the appropriate session(s).
	 * REQ-LNP-036: Filters are evaluated BEFORE routing.
	 */
	route(event: EventEnvelope, listenerConfig: ListenerEntry): RoutingResult;

	/** Update the active sessions map. */
	updateSessions(sessions: Map<string, SessionInfo>): void;

	/** Get the current session ID. */
	getCurrentSessionId(): string | undefined;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create a session router.
 * REQ-LNP-030: Routing engine that evaluates events against declarative rules.
 */
export function createSessionRouter(
	options: SessionRouterOptions = {},
): SessionRouter {
	let currentSessionId = options.currentSessionId;
	let activeSessions: Map<string, SessionInfo> =
		options.activeSessions ?? new Map();

	return {
		route(event: EventEnvelope, listenerConfig: ListenerEntry): RoutingResult {
			// REQ-LNP-036: Evaluate filters BEFORE routing
			if (listenerConfig.filter) {
				const filterPassed = evaluateFilter(event, listenerConfig.filter, listenerConfig.routing?.match_all);
				if (!filterPassed) {
					emitListenerLog({
						event: "listener.event_dropped",
						listener_name: listenerConfig.name,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						reason: "filter_mismatch",
					});
					return { target_sessions: [], dropped: true, drop_reason: "filter_mismatch" };
				}
			}

			const routing = listenerConfig.routing;
			if (!routing?.match) {
				// No routing rules — deliver to current session (self) or fallback
				const target = currentSessionId ?? routing?.fallback_session;
				if (target) {
					return {
						target_sessions: [target],
						match_rule: "default_self",
						dropped: false,
					};
				}
				return _dropNoSession(event, listenerConfig.name, routing);
			}

			// REQ-LNP-034: Variable substitution in routing rules
			const resolvedMatch = resolveVariables(routing.match, event);

			// Guard: if resolvedMatch is empty after variable substitution, treat as no-match.
			// This prevents fan-out to all sessions when routing rules resolve to empty.
			// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-034 plan=step-fix-009 test=listener-router.test.ts#fan-out-bomb-prevention
			if (Object.keys(resolvedMatch).length === 0) {
				return _dropNoSession(event, listenerConfig.name, routing);
			}

			// Find matching sessions
			const matched: string[] = [];
			for (const [sessionId, info] of activeSessions) {
				if (info.paused) continue; // REQ-LNP-047: skip paused sessions
				if (matchesRoutingRule(resolvedMatch, info, sessionId, currentSessionId)) {
					matched.push(sessionId);
				}
			}

			// REQ-LNP-033: Fan-out to all matching sessions
			if (matched.length > 0) {
				for (const sid of matched) {
					emitListenerLog({
						event: "listener.event_routed",
						listener_name: listenerConfig.name,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						session_id: sid,
					});
				}
				return {
					target_sessions: matched,
					match_rule: JSON.stringify(routing.match),
					dropped: false,
				};
			}

			// REQ-LNP-032: Fallback session
			if (routing.fallback_session) {
				return {
					target_sessions: [routing.fallback_session],
					match_rule: "fallback",
					dropped: false,
				};
			}

			return _dropNoSession(event, listenerConfig.name, routing);
		},

		updateSessions(sessions: Map<string, SessionInfo>): void {
			activeSessions = sessions;
		},

		getCurrentSessionId(): string | undefined {
			return currentSessionId;
		},
	};
}

// ── Filter Evaluation ────────────────────────────────────────────────────

/**
 * Evaluate a filter against an event payload.
 * REQ-LNP-037: Supports exact, prefix, contains, regex, exists operators.
 * REQ-LNP-038: match_all (AND) vs match_any (OR) logic.
 */
export function evaluateFilter(
	event: EventEnvelope,
	filter: ListenerFilter,
	matchAll = false,
): boolean {
	const entries = Object.entries(filter);
	if (entries.length === 0) return true;

	const results = entries.map(([field, condition]) => {
		const value = getNestedField(event.payload, field);
		if (typeof condition === "string") {
			// Simple exact match
			return String(value) === condition;
		}
		return evaluateOperator(value, condition.operator, condition.value);
	});

	return matchAll ? results.every(Boolean) : results.some(Boolean);
}

/** Evaluate a single operator condition. */
function evaluateOperator(
	value: unknown,
	operator: "exact" | "prefix" | "contains" | "regex" | "exists",
	expected?: string,
): boolean {
	switch (operator) {
		case "exists":
			return value !== undefined && value !== null;
		case "exact":
			return String(value) === (expected ?? "");
		case "prefix":
			return String(value).startsWith(expected ?? "");
		case "contains":
			return String(value).includes(expected ?? "");
		case "regex": {
			try {
				return new RegExp(expected ?? "").test(String(value));
			} catch {
				return false;
			}
		}
		default:
			return false;
	}
}

// ── Variable Substitution ────────────────────────────────────────────────

/**
 * Resolve $event.field.path variables in routing rules.
 * REQ-LNP-034: $event.field.path resolved against event payload.
 */
export function resolveVariables(
	match: Record<string, string>,
	event: EventEnvelope,
): Record<string, string> {
	const resolved: Record<string, string> = {};
	for (const [key, value] of Object.entries(match)) {
		if (typeof value === "string" && value.startsWith("$event.")) {
			const path = value.slice("$event.".length);
			const fieldValue = getNestedField(event.payload, path);
			resolved[key] = fieldValue !== undefined ? String(fieldValue) : value;
		} else {
			resolved[key] = value;
		}
	}
	return resolved;
}

// ── Routing Rule Matching ────────────────────────────────────────────────

/**
 * Check if a session matches the resolved routing rules.
 * REQ-LNP-031: work_item, tag, session_id, self matching strategies.
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-034 plan=step-fix-009 test=listener-router.test.ts#matchesRoutingRule-empty-guard
 */
export function matchesRoutingRule(
	resolvedMatch: Record<string, string>,
	info: SessionInfo,
	sessionId: string,
	currentSessionId?: string,
): boolean {
	// Defense-in-depth guard: empty match object must never match any session.
	// Primary guard lives in route() above; this catches any direct callers.
	if (Object.keys(resolvedMatch).length === 0) return false;

	for (const [key, value] of Object.entries(resolvedMatch)) {
		switch (key) {
			case "work_item":
				if (info.work_item !== value) return false;
				break;
			case "tag":
				if (!info.tags?.includes(value)) return false;
				break;
			case "session_id":
				if (sessionId !== value) return false;
				break;
			case "self":
				if (sessionId !== currentSessionId) return false;
				break;
			default:
				// Unknown key — skip
				break;
		}
	}
	return true;
}

// ── Helpers ──────────────────────────────────────────────────────────────

/** Get a nested field from an object using dot notation. */
export function getNestedField(obj: unknown, path: string): unknown {
	if (obj === null || obj === undefined) return undefined;
	const parts = path.split(".");
	let current: unknown = obj;
	for (const part of parts) {
		if (current === null || current === undefined) return undefined;
		if (typeof current !== "object") return undefined;
		current = (current as Record<string, unknown>)[part];
	}
	return current;
}

/** Drop an event with no_session reason. */
function _dropNoSession(
	event: EventEnvelope,
	listenerName: string,
	_routing?: ListenerRouting,
): RoutingResult {
	emitListenerLog({
		event: "listener.event_dropped",
		listener_name: listenerName,
		timestamp: new Date().toISOString(),
		event_id: event.id,
		reason: "no_session",
	});
	return {
		target_sessions: [],
		dropped: true,
		drop_reason: "no_session",
	};
}
