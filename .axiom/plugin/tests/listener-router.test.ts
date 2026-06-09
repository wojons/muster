/**
 * Tests for the session router.
 *
 * Covers: filter evaluation, variable substitution, routing strategies,
 * fan-out, fallback session, match_all vs match_any.
 *
 * REQ-LNP-030 through REQ-LNP-038.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#4
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.2
 */
import { describe, expect, it } from "bun:test";
import type { ListenerEntry } from "../src/config-loader";
import {
	createSessionRouter,
	evaluateFilter,
	getNestedField,
	matchesRoutingRule,
	resolveVariables,
} from "../src/layers/listeners/router";
import type { EventEnvelope } from "../src/layers/listeners/types";
import { generateEventId } from "../src/layers/listeners/types";

// ── Helpers ──────────────────────────────────────────────────────────────

function makeEvent(payload: unknown = {}): EventEnvelope {
	return {
		id: generateEventId(),
		listener_name: "test-listener",
		source_type: "sqs",
		source_ref: "arn:aws:sqs:us-east-1:123:test",
		received_at: new Date().toISOString(),
		payload,
		delivery: {
			mode: "non-interruptive",
			status: "pending",
			attempts: 0,
			last_attempt_at: null,
		},
	};
}

function makeListenerEntry(overrides: Partial<ListenerEntry> = {}): ListenerEntry {
	return {
		name: "test-listener",
		source: "sqs",
		config: { queue_url: "https://sqs.us-east-1.amazonaws.com/123/test", region: "us-east-1" },
		...overrides,
	};
}

// ── getNestedField ───────────────────────────────────────────────────────

describe("getNestedField", () => {
	it("returns top-level field", () => {
		expect(getNestedField({ foo: "bar" }, "foo")).toBe("bar");
	});

	it("returns nested field with dot notation", () => {
		expect(getNestedField({ issue: { key: "ABC-123" } }, "issue.key")).toBe("ABC-123");
	});

	it("returns undefined for missing field", () => {
		expect(getNestedField({ foo: "bar" }, "baz")).toBeUndefined();
	});

	it("returns undefined for null object", () => {
		expect(getNestedField(null, "foo")).toBeUndefined();
	});

	it("handles deeply nested paths", () => {
		const obj = { a: { b: { c: { d: 42 } } } };
		expect(getNestedField(obj, "a.b.c.d")).toBe(42);
	});
});

// ── evaluateFilter ───────────────────────────────────────────────────────

describe("evaluateFilter — REQ-LNP-037, REQ-LNP-038", () => {
	it("returns true for empty filter", () => {
		const event = makeEvent({ type: "test" });
		expect(evaluateFilter(event, {})).toBe(true);
	});

	it("matches exact string (match_any default)", () => {
		const event = makeEvent({ event_type: "jira:issue_updated" });
		expect(evaluateFilter(event, { event_type: "jira:issue_updated" })).toBe(true);
	});

	it("does not match wrong exact string", () => {
		const event = makeEvent({ event_type: "jira:issue_created" });
		expect(evaluateFilter(event, { event_type: "jira:issue_updated" })).toBe(false);
	});

	it("matches prefix operator", () => {
		const event = makeEvent({ event_type: "jira:issue_updated" });
		expect(
			evaluateFilter(event, {
				event_type: { operator: "prefix", value: "jira:" },
			}),
		).toBe(true);
	});

	it("matches contains operator", () => {
		const event = makeEvent({ message: "deployment succeeded" });
		expect(
			evaluateFilter(event, {
				message: { operator: "contains", value: "succeeded" },
			}),
		).toBe(true);
	});

	it("matches regex operator", () => {
		const event = makeEvent({ status: "DONE" });
		expect(
			evaluateFilter(event, {
				status: { operator: "regex", value: "^(DONE|CLOSED)$" },
			}),
		).toBe(true);
	});

	it("matches exists operator for present field", () => {
		const event = makeEvent({ issue_key: "ABC-123" });
		expect(
			evaluateFilter(event, {
				issue_key: { operator: "exists" },
			}),
		).toBe(true);
	});

	it("does not match exists operator for missing field", () => {
		const event = makeEvent({});
		expect(
			evaluateFilter(event, {
				issue_key: { operator: "exists" },
			}),
		).toBe(false);
	});

	it("match_any: returns true if ANY condition matches (REQ-LNP-038)", () => {
		const event = makeEvent({ type: "A" });
		expect(
			evaluateFilter(
				event,
				{ type: "A", status: "B" },
				false, // match_any
			),
		).toBe(true);
	});

	it("match_all: returns false if NOT ALL conditions match (REQ-LNP-038)", () => {
		const event = makeEvent({ type: "A" });
		expect(
			evaluateFilter(
				event,
				{ type: "A", status: "B" },
				true, // match_all
			),
		).toBe(false);
	});

	it("match_all: returns true if ALL conditions match (REQ-LNP-038)", () => {
		const event = makeEvent({ type: "A", status: "B" });
		expect(
			evaluateFilter(
				event,
				{ type: "A", status: "B" },
				true, // match_all
			),
		).toBe(true);
	});
});

// ── resolveVariables ─────────────────────────────────────────────────────

describe("resolveVariables — REQ-LNP-034", () => {
	it("resolves $event.field.path from payload", () => {
		const event = makeEvent({ issue: { key: "ABC-123" } });
		const resolved = resolveVariables({ work_item: "$event.issue.key" }, event);
		expect(resolved.work_item).toBe("ABC-123");
	});

	it("leaves non-variable values unchanged", () => {
		const event = makeEvent({});
		const resolved = resolveVariables({ work_item: "STATIC-VALUE" }, event);
		expect(resolved.work_item).toBe("STATIC-VALUE");
	});

	it("keeps original value when field not found", () => {
		const event = makeEvent({});
		const resolved = resolveVariables({ work_item: "$event.missing.field" }, event);
		expect(resolved.work_item).toBe("$event.missing.field");
	});
});

// ── fan-out bomb prevention ──────────────────────────────────────────────

/**
 * REQ-LNP-034: Fan-out bomb prevention.
 * When a routing rule uses $event.field.path and the field is absent from the
 * payload, resolveVariables() keeps the literal "$event.field.path" string.
 * The route() guard must detect an empty resolvedMatch and drop — not broadcast.
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-034 plan=step-fix-009
 */
describe("fan-out bomb prevention — REQ-LNP-034", () => {
	it("drops event when routing rule resolves to empty match (missing field)", () => {
		const sessions = new Map([
			["s1", { session_id: "s1", work_item: "ABC-123" }],
			["s2", { session_id: "s2", work_item: "XYZ-999" }],
		]);
		const router = createSessionRouter({
			currentSessionId: "s1",
			activeSessions: sessions,
		});

		// Event payload does NOT have the field referenced in routing rule
		const event = makeEvent({ other_field: "value" }); // no 'issue.key'
		const entry = makeListenerEntry({
			routing: { match: { work_item: "$event.issue.key" } },
		});

		// $event.issue.key resolves to the literal string "$event.issue.key"
		// which doesn't match any work_item — should drop, not fan-out to all sessions
		const result = router.route(event, entry);
		// Must NOT deliver to both sessions (fan-out bomb)
		expect(result.target_sessions).not.toHaveLength(2);
	});

	it("drops event (no_session) when routing rule resolves to empty match", () => {
		const sessions = new Map([
			["s1", { session_id: "s1", work_item: "ABC-123" }],
			["s2", { session_id: "s2", work_item: "XYZ-999" }],
		]);
		const router = createSessionRouter({
			currentSessionId: "s1",
			activeSessions: sessions,
		});

		// Routing match is explicitly empty — must never match all sessions
		const event = makeEvent({ some_field: "value" });
		const entry = makeListenerEntry({
			routing: { match: {} },
		});

		const result = router.route(event, entry);
		expect(result.dropped).toBe(true);
		expect(result.drop_reason).toBe("no_session");
		expect(result.target_sessions).toHaveLength(0);
	});

	it("matchesRoutingRule returns false for empty resolvedMatch (defense-in-depth)", () => {
		const info = { session_id: "s1", work_item: "ABC-123" };
		// Empty match must never match — prevents silent fan-out from direct callers
		expect(matchesRoutingRule({}, info, "s1")).toBe(false);
	});

	it("matchesRoutingRule returns false for empty resolvedMatch regardless of currentSessionId", () => {
		const info = { session_id: "s1", work_item: "ABC-123" };
		expect(matchesRoutingRule({}, info, "s1", "s1")).toBe(false);
	});
});

// ── matchesRoutingRule ───────────────────────────────────────────────────

describe("matchesRoutingRule — REQ-LNP-031", () => {
	it("matches by work_item", () => {
		const info = { session_id: "s1", work_item: "ABC-123" };
		expect(matchesRoutingRule({ work_item: "ABC-123" }, info, "s1")).toBe(true);
	});

	it("does not match wrong work_item", () => {
		const info = { session_id: "s1", work_item: "XYZ-999" };
		expect(matchesRoutingRule({ work_item: "ABC-123" }, info, "s1")).toBe(false);
	});

	it("matches by tag", () => {
		const info = { session_id: "s1", tags: ["infrastructure", "ops"] };
		expect(matchesRoutingRule({ tag: "infrastructure" }, info, "s1")).toBe(true);
	});

	it("matches by session_id", () => {
		const info = { session_id: "s1" };
		expect(matchesRoutingRule({ session_id: "s1" }, info, "s1")).toBe(true);
	});

	it("matches self routing", () => {
		const info = { session_id: "current-session" };
		expect(
			matchesRoutingRule({ self: "true" }, info, "current-session", "current-session"),
		).toBe(true);
	});

	it("does not match self routing for different session", () => {
		const info = { session_id: "other-session" };
		expect(
			matchesRoutingRule({ self: "true" }, info, "other-session", "current-session"),
		).toBe(false);
	});
});

// ── createSessionRouter ──────────────────────────────────────────────────

describe("createSessionRouter — REQ-LNP-030 through REQ-LNP-038", () => {
	it("routes to current session when no routing rules", () => {
		const router = createSessionRouter({ currentSessionId: "session-1" });
		const event = makeEvent({});
		const entry = makeListenerEntry();
		const result = router.route(event, entry);
		expect(result.dropped).toBe(false);
		expect(result.target_sessions).toContain("session-1");
	});

	it("drops event when no session matches and no fallback", () => {
		const router = createSessionRouter({ currentSessionId: "session-1" });
		const event = makeEvent({ issue: { key: "ABC-123" } });
		const entry = makeListenerEntry({
			routing: { match: { work_item: "ABC-123" } },
		});
		// No active sessions registered
		const result = router.route(event, entry);
		expect(result.dropped).toBe(true);
		expect(result.drop_reason).toBe("no_session");
	});

	it("routes to fallback session when no match (REQ-LNP-032)", () => {
		const router = createSessionRouter({ currentSessionId: "session-1" });
		const event = makeEvent({});
		const entry = makeListenerEntry({
			routing: {
				match: { work_item: "MISSING-999" },
				fallback_session: "fallback-session",
			},
		});
		const result = router.route(event, entry);
		expect(result.dropped).toBe(false);
		expect(result.target_sessions).toContain("fallback-session");
	});

	it("fan-out to multiple matching sessions (REQ-LNP-033)", () => {
		const sessions = new Map([
			["s1", { session_id: "s1", work_item: "ABC-123" }],
			["s2", { session_id: "s2", work_item: "ABC-123" }],
			["s3", { session_id: "s3", work_item: "XYZ-999" }],
		]);
		const router = createSessionRouter({
			currentSessionId: "s1",
			activeSessions: sessions,
		});
		const event = makeEvent({});
		const entry = makeListenerEntry({
			routing: { match: { work_item: "ABC-123" } },
		});
		const result = router.route(event, entry);
		expect(result.dropped).toBe(false);
		expect(result.target_sessions).toHaveLength(2);
		expect(result.target_sessions).toContain("s1");
		expect(result.target_sessions).toContain("s2");
	});

	it("drops event when filter does not match (REQ-LNP-036)", () => {
		const router = createSessionRouter({ currentSessionId: "session-1" });
		const event = makeEvent({ type: "wrong-type" });
		const entry = makeListenerEntry({
			filter: { type: "expected-type" },
		});
		const result = router.route(event, entry);
		expect(result.dropped).toBe(true);
		expect(result.drop_reason).toBe("filter_mismatch");
	});

	it("resolves $event variables in routing rules (REQ-LNP-034)", () => {
		const sessions = new Map([
			["s1", { session_id: "s1", work_item: "ABC-123" }],
		]);
		const router = createSessionRouter({
			currentSessionId: "s1",
			activeSessions: sessions,
		});
		const event = makeEvent({ issue: { key: "ABC-123" } });
		const entry = makeListenerEntry({
			routing: { match: { work_item: "$event.issue.key" } },
		});
		const result = router.route(event, entry);
		expect(result.dropped).toBe(false);
		expect(result.target_sessions).toContain("s1");
	});

	it("skips paused sessions (REQ-LNP-047)", () => {
		const sessions = new Map([
			["s1", { session_id: "s1", work_item: "ABC-123", paused: true }],
		]);
		const router = createSessionRouter({
			currentSessionId: "s1",
			activeSessions: sessions,
		});
		const event = makeEvent({});
		const entry = makeListenerEntry({
			routing: { match: { work_item: "ABC-123" } },
		});
		const result = router.route(event, entry);
		expect(result.dropped).toBe(true);
	});
});
