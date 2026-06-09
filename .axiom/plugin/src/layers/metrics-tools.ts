/**
 * L2 Metrics Tools — codeops_session_cost tool output builder and
 * conversational summary formatter.
 *
 * REQ-PLG-102: Expose a codeops_session_cost tool that returns the current
 *   session's cost breakdown with the spec-defined JSON shape.
 * REQ-PLG-103: Cost display uses conversational language per UX writer
 *   guidance: "This session used ~$X.XX across N models (M min, P messages)"
 *   — not raw JSON dumps.
 * REQ-PLG-104: No network calls for pricing data. Pricing tables are embedded
 *   and updated with plugin releases. Users MAY override via .axiom/plugin.yaml.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-102, specs/70-OpenCode-Plugin.md#REQ-PLG-103, specs/70-OpenCode-Plugin.md#REQ-PLG-104
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-102 plan=phase-70-3/task-70-3-2/step-70-3-2-2
 */

import type { SessionCostResult } from "./metrics";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Per-model entry in the tool output (REQ-PLG-102 JSON shape). */
export interface SessionCostModelEntry {
	model: string;
	cost_usd: number | null;
	calls: number;
}

/**
 * The spec-defined JSON shape returned by the codeops_session_cost tool.
 *
 * REQ-PLG-102 defines the exact field names:
 *   session_id, total_cost_usd, total_tokens.prompt, total_tokens.completion,
 *   by_model[].model, by_model[].cost_usd, by_model[].calls,
 *   duration_minutes, messages, tools_used
 */
export interface SessionCostToolResult {
	session_id: string;
	total_cost_usd: number;
	total_tokens: {
		prompt: number;
		completion: number;
	};
	by_model: SessionCostModelEntry[];
	duration_minutes: number;
	messages: number;
	tools_used: string[];
	/** REQ-PLG-104a: stable identity for cost attribution (git→hostname→UUID→custom). */
	session_identity?: string;
	/** REQ-PLG-104a: source of the resolved identity (git|hostname|uuid|custom). */
	session_identity_source?: string;
}

/** Input parameters for buildSessionCostResult. */
export interface BuildSessionCostInput {
	sessionId: string;
	costResult: SessionCostResult;
	durationMinutes: number;
	messageCount: number;
	toolsUsed: string[];
}

// ---------------------------------------------------------------------------
// REQ-PLG-102: Build the spec-defined JSON shape
// ---------------------------------------------------------------------------

/**
 * Transform an internal SessionCostResult into the spec-defined
 * codeops_session_cost tool output shape.
 *
 * Maps internal token field names (input/output) to the spec-defined
 * prompt/completion names. Passes through per-model breakdown with
 * model, cost_usd, and calls fields.
 *
 * REQ-PLG-104: This function is a pure data transform — no network calls,
 * no file I/O. Pricing is resolved upstream via loadPricingTable.
 */
export function buildSessionCostResult(
	input: BuildSessionCostInput,
): SessionCostToolResult {
	const { sessionId, costResult, durationMinutes, messageCount, toolsUsed } =
		input;

	return {
		session_id: sessionId,
		total_cost_usd: costResult.total_cost_usd,
		total_tokens: {
			// Map internal "input" → spec "prompt", "output" → spec "completion"
			prompt: costResult.total_tokens.input,
			completion: costResult.total_tokens.output,
		},
		by_model: costResult.by_model.map((entry) => ({
			model: entry.model,
			cost_usd: entry.cost_usd,
			calls: entry.calls,
		})),
		duration_minutes: durationMinutes,
		messages: messageCount,
		tools_used: toolsUsed,
	};
}

// ---------------------------------------------------------------------------
// REQ-PLG-103: Conversational summary formatter
// ---------------------------------------------------------------------------

/**
 * Format a SessionCostToolResult into a conversational summary string.
 *
 * REQ-PLG-103 spec example:
 *   "This session used ~$0.42 across 3 models (14 min, 15 messages)"
 *
 * Rules:
 * - Cost is rounded to 2 decimal places with tilde prefix (~$X.XX)
 * - Singular/plural for "model" and "message"
 * - Duration in whole minutes
 * - No raw JSON in the output
 *
 * REQ-PLG-104: Pure function — no network calls, no side effects.
 */
export function formatSessionCostSummary(
	result: SessionCostToolResult,
): string {
	const cost = result.total_cost_usd.toFixed(2);
	const modelCount = result.by_model.length;
	const modelWord = modelCount === 1 ? "model" : "models";
	const minutes = Math.round(result.duration_minutes);
	const messageCount = result.messages;
	const messageWord = messageCount === 1 ? "message" : "messages";

	return `This session used ~$${cost} across ${modelCount} ${modelWord} (${minutes} min, ${messageCount} ${messageWord})`;
}
