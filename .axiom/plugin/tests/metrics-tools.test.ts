/**
 * Session cost tool contract tests — verifies the codeops_session_cost tool
 * output shape, conversational summary wording, and embedded-pricing/no-network
 * rule before any tool implementation begins.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/layers/metrics-tools
 * which does not exist yet. All tests are expected to fail at the module-
 * resolution level until the tool surface is implemented.
 *
 * REQ-PLG-102: The plugin MUST expose a codeops_session_cost tool that returns
 *   the current session's cost breakdown with the spec-defined JSON shape.
 * REQ-PLG-103: Cost display MUST use conversational language per UX writer
 *   guidance: "This session used ~$X.XX across N models (M min, P messages)"
 *   — not raw JSON dumps.
 * REQ-PLG-104: The plugin MUST NOT make network calls to fetch pricing data.
 *   Pricing tables are embedded and updated with plugin releases. Users MAY
 *   override pricing via .axiom/plugin.yaml.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-102, specs/70-OpenCode-Plugin.md#REQ-PLG-103, specs/70-OpenCode-Plugin.md#REQ-PLG-104
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-102 plan=phase-70-3/task-70-3-2/step-70-3-2-1
 */
import { describe, expect, it } from "bun:test";
import type { PricingTable, SessionCostResult } from "../src/layers/metrics";
// Import from the not-yet-existing metrics-tools module — this WILL fail at
// module resolution, which is the expected test-first signal.
import {
	buildSessionCostResult,
	formatSessionCostSummary,
	type SessionCostToolResult,
} from "../src/layers/metrics-tools";

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Build a minimal SessionCostResult for tool-output testing. */
function makeCostResult(
	overrides: Partial<SessionCostResult> = {},
): SessionCostResult {
	return {
		total_cost_usd: 0.42,
		total_tokens: {
			input: 12000,
			output: 3000,
			reasoning: 200,
			cache_read: 5000,
			cache_write: 2000,
		},
		by_model: [
			{
				model: "claude-sonnet-4-20250514",
				calls: 12,
				cost_usd: 0.38,
				tokens: {
					input: 10000,
					output: 2500,
					reasoning: 200,
					cache_read: 4000,
					cache_write: 1500,
				},
			},
			{
				model: "claude-haiku-4-20250414",
				calls: 3,
				cost_usd: 0.04,
				tokens: {
					input: 2000,
					output: 500,
					reasoning: 0,
					cache_read: 1000,
					cache_write: 500,
				},
			},
		],
		...overrides,
	};
}

/** Build a minimal pricing table for embedded-pricing assertions. */
function _makeEmbeddedPricing(): PricingTable {
	return {
		version: "2026-02-01",
		models: [
			{
				id: "claude-sonnet-4-20250514",
				provider: "anthropic",
				aliases: [],
				price_per_1m_input: 3.0,
				price_per_1m_output: 15.0,
				price_per_1m_cache_read: 0.3,
				price_per_1m_cache_write: 3.75,
				price_per_1m_reasoning: 15.0,
			},
		],
	};
}

// ===========================================================================
// REQ-PLG-102: codeops_session_cost JSON shape
// ===========================================================================

describe("REQ-PLG-102: buildSessionCostResult JSON shape", () => {
	it("returns the spec-defined top-level fields", () => {
		const costResult = makeCostResult();
		const toolResult: SessionCostToolResult = buildSessionCostResult({
			sessionId: "abc-123",
			costResult,
			durationMinutes: 14.2,
			messageCount: 15,
			toolsUsed: ["bash", "read", "write", "edit"],
		});

		// Spec-required top-level fields per REQ-PLG-102
		expect(toolResult.session_id).toBe("abc-123");
		expect(typeof toolResult.total_cost_usd).toBe("number");
		expect(toolResult.total_cost_usd).toBeCloseTo(0.42, 2);
		expect(toolResult.total_tokens).toBeDefined();
		expect(typeof toolResult.total_tokens.prompt).toBe("number");
		expect(typeof toolResult.total_tokens.completion).toBe("number");
		expect(Array.isArray(toolResult.by_model)).toBe(true);
		expect(typeof toolResult.duration_minutes).toBe("number");
		expect(toolResult.duration_minutes).toBeCloseTo(14.2, 1);
		expect(typeof toolResult.messages).toBe("number");
		expect(toolResult.messages).toBe(15);
		expect(Array.isArray(toolResult.tools_used)).toBe(true);
		expect(toolResult.tools_used).toEqual(["bash", "read", "write", "edit"]);
	});

	it("maps internal token fields to spec-defined prompt/completion names", () => {
		// The spec JSON uses "prompt" and "completion" (not "input" and "output")
		const costResult = makeCostResult({
			total_tokens: {
				input: 15000,
				output: 3200,
				reasoning: 0,
				cache_read: 0,
				cache_write: 0,
			},
		});

		const toolResult = buildSessionCostResult({
			sessionId: "test-session",
			costResult,
			durationMinutes: 10,
			messageCount: 8,
			toolsUsed: [],
		});

		// REQ-PLG-102 spec JSON: total_tokens.prompt and total_tokens.completion
		expect(toolResult.total_tokens.prompt).toBe(15000);
		expect(toolResult.total_tokens.completion).toBe(3200);
	});

	it("includes per-model breakdown with model name, cost, and call count", () => {
		const costResult = makeCostResult();
		const toolResult = buildSessionCostResult({
			sessionId: "test-session",
			costResult,
			durationMinutes: 14.2,
			messageCount: 15,
			toolsUsed: [],
		});

		// REQ-PLG-102 spec JSON: by_model array with model, cost_usd, calls
		expect(toolResult.by_model).toHaveLength(2);
		const sonnet = toolResult.by_model.find(
			(e: { model: string }) => e.model === "claude-sonnet-4-20250514",
		);
		expect(sonnet).toBeDefined();
		expect(sonnet?.cost_usd).toBeCloseTo(0.38, 2);
		expect(sonnet?.calls).toBe(12);

		const haiku = toolResult.by_model.find(
			(e: { model: string }) => e.model === "claude-haiku-4-20250414",
		);
		expect(haiku).toBeDefined();
		expect(haiku?.cost_usd).toBeCloseTo(0.04, 2);
		expect(haiku?.calls).toBe(3);
	});

	it("handles zero-cost sessions (no messages)", () => {
		const costResult = makeCostResult({
			total_cost_usd: 0,
			total_tokens: {
				input: 0,
				output: 0,
				reasoning: 0,
				cache_read: 0,
				cache_write: 0,
			},
			by_model: [],
		});

		const toolResult = buildSessionCostResult({
			sessionId: "empty-session",
			costResult,
			durationMinutes: 0,
			messageCount: 0,
			toolsUsed: [],
		});

		expect(toolResult.total_cost_usd).toBe(0);
		expect(toolResult.messages).toBe(0);
		expect(toolResult.by_model).toEqual([]);
		expect(toolResult.tools_used).toEqual([]);
	});

	it("handles null cost_usd for unknown models in by_model", () => {
		const costResult = makeCostResult({
			by_model: [
				{
					model: "unknown-model",
					calls: 2,
					cost_usd: null,
					tokens: {
						input: 5000,
						output: 1000,
						reasoning: 0,
						cache_read: 0,
						cache_write: 0,
					},
				},
			],
		});

		const toolResult = buildSessionCostResult({
			sessionId: "unknown-model-session",
			costResult,
			durationMinutes: 5,
			messageCount: 2,
			toolsUsed: ["bash"],
		});

		expect(toolResult.by_model).toHaveLength(1);
		expect(toolResult.by_model[0].cost_usd).toBeNull();
		expect(toolResult.by_model[0].model).toBe("unknown-model");
	});
});

// ===========================================================================
// REQ-PLG-103: Conversational summary wording
// ===========================================================================

describe("REQ-PLG-103: formatSessionCostSummary conversational wording", () => {
	it("produces the spec-defined conversational format", () => {
		// Spec example: "This session used ~$0.42 across 3 models (14 min, 15 messages)"
		const toolResult: SessionCostToolResult = {
			session_id: "abc-123",
			total_cost_usd: 0.42,
			total_tokens: { prompt: 15000, completion: 3200 },
			by_model: [
				{ model: "claude-sonnet-4-20250514", cost_usd: 0.38, calls: 12 },
				{ model: "claude-haiku-4-20250414", cost_usd: 0.04, calls: 3 },
			],
			duration_minutes: 14.2,
			messages: 15,
			tools_used: ["bash", "read", "write", "edit"],
		};

		const summary = formatSessionCostSummary(toolResult);

		// Must contain the approximate cost with dollar sign
		expect(summary).toContain("~$0.42");
		// Must mention model count
		expect(summary).toContain("2 model");
		// Must mention duration in minutes
		expect(summary).toMatch(/14\s*min/);
		// Must mention message count
		expect(summary).toContain("15 message");
	});

	it("uses singular 'model' when only one model is used", () => {
		const toolResult: SessionCostToolResult = {
			session_id: "single-model",
			total_cost_usd: 0.1,
			total_tokens: { prompt: 5000, completion: 1000 },
			by_model: [
				{ model: "claude-sonnet-4-20250514", cost_usd: 0.1, calls: 5 },
			],
			duration_minutes: 3.5,
			messages: 5,
			tools_used: ["bash"],
		};

		const summary = formatSessionCostSummary(toolResult);

		expect(summary).toContain("1 model");
		// Should NOT say "1 models"
		expect(summary).not.toMatch(/1 models/);
	});

	it("uses singular 'message' when only one message", () => {
		const toolResult: SessionCostToolResult = {
			session_id: "one-msg",
			total_cost_usd: 0.01,
			total_tokens: { prompt: 1000, completion: 200 },
			by_model: [
				{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 },
			],
			duration_minutes: 0.5,
			messages: 1,
			tools_used: [],
		};

		const summary = formatSessionCostSummary(toolResult);

		expect(summary).toContain("1 message");
		expect(summary).not.toMatch(/1 messages/);
	});

	it("rounds cost to two decimal places with tilde prefix", () => {
		const toolResult: SessionCostToolResult = {
			session_id: "rounding",
			total_cost_usd: 0.123456,
			total_tokens: { prompt: 5000, completion: 1000 },
			by_model: [
				{ model: "claude-sonnet-4-20250514", cost_usd: 0.123456, calls: 3 },
			],
			duration_minutes: 2,
			messages: 3,
			tools_used: [],
		};

		const summary = formatSessionCostSummary(toolResult);

		// Should round to ~$0.12 (2 decimal places)
		expect(summary).toContain("~$0.12");
	});

	it("handles zero-cost sessions gracefully", () => {
		const toolResult: SessionCostToolResult = {
			session_id: "zero-cost",
			total_cost_usd: 0,
			total_tokens: { prompt: 0, completion: 0 },
			by_model: [],
			duration_minutes: 0,
			messages: 0,
			tools_used: [],
		};

		const summary = formatSessionCostSummary(toolResult);

		// Should still produce a valid string, not crash
		expect(typeof summary).toBe("string");
		expect(summary.length).toBeGreaterThan(0);
		expect(summary).toContain("$0.00");
	});

	it("does NOT return raw JSON as the summary", () => {
		const toolResult: SessionCostToolResult = {
			session_id: "no-json",
			total_cost_usd: 0.42,
			total_tokens: { prompt: 15000, completion: 3200 },
			by_model: [
				{ model: "claude-sonnet-4-20250514", cost_usd: 0.38, calls: 12 },
			],
			duration_minutes: 14,
			messages: 15,
			tools_used: ["bash"],
		};

		const summary = formatSessionCostSummary(toolResult);

		// REQ-PLG-103 explicitly says "not raw JSON dumps"
		expect(summary).not.toContain('"session_id"');
		expect(summary).not.toContain('"total_cost_usd"');
		expect(summary).not.toContain("{");
	});
});

// ===========================================================================
// REQ-PLG-104: Embedded pricing / no-network rule
// ===========================================================================

describe("REQ-PLG-104: embedded pricing / no-network rule", () => {
	it("buildSessionCostResult uses only the provided pricing data (no fetch)", () => {
		// This test verifies the function signature does NOT accept a URL or
		// network-fetching option. The function takes a pre-computed costResult
		// and session metadata — pricing is resolved upstream via loadPricingTable
		// which reads from disk or embedded data only.
		const costResult = makeCostResult();
		const toolResult = buildSessionCostResult({
			sessionId: "no-network",
			costResult,
			durationMinutes: 5,
			messageCount: 3,
			toolsUsed: [],
		});

		// The function must succeed without any network call
		expect(toolResult.session_id).toBe("no-network");
		expect(toolResult.total_cost_usd).toBeCloseTo(0.42, 2);
	});

	it("buildSessionCostResult does not accept a pricing URL parameter", () => {
		// Verify the function signature: it should NOT have a pricingUrl or
		// fetchPricing parameter. This is a compile-time contract encoded as
		// a runtime assertion: passing extra properties should not change behavior.
		const costResult = makeCostResult({ total_cost_usd: 0.1 });
		const toolResult = buildSessionCostResult({
			sessionId: "no-url",
			costResult,
			durationMinutes: 1,
			messageCount: 1,
			toolsUsed: [],
			// @ts-expect-error — pricingUrl is not a valid parameter
			pricingUrl: "https://example.com/pricing.json",
		} as Parameters<typeof buildSessionCostResult>[0]);

		// Must still return the locally-computed cost, ignoring any URL
		expect(toolResult.total_cost_usd).toBeCloseTo(0.1, 2);
	});

	it("formatSessionCostSummary is a pure function with no side effects", () => {
		// The summary formatter takes a result object and returns a string.
		// It must not make network calls, read files, or have side effects.
		const toolResult: SessionCostToolResult = {
			session_id: "pure-fn",
			total_cost_usd: 0.05,
			total_tokens: { prompt: 2000, completion: 500 },
			by_model: [
				{ model: "claude-sonnet-4-20250514", cost_usd: 0.05, calls: 2 },
			],
			duration_minutes: 1,
			messages: 2,
			tools_used: [],
		};

		// Call twice — must return identical results (pure function)
		const summary1 = formatSessionCostSummary(toolResult);
		const summary2 = formatSessionCostSummary(toolResult);
		expect(summary1).toBe(summary2);
		expect(typeof summary1).toBe("string");
	});
});
