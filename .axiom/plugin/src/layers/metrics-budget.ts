/**
 * L2 Metrics Budget — per-session cost budget evaluation, conversational
 * warning messages, and advisory over-budget alerts.
 *
 * REQ-PLG-110: The plugin MUST support per-session cost budgets defined in
 *   `.axiom/plugin.yaml` under `metrics.session_budget_usd`,
 *   `metrics.warning_threshold_pct`, and `metrics.alert_channel`.
 * REQ-PLG-111: When session cost exceeds the warning threshold, the plugin
 *   MUST emit a conversational warning: "Heads up — this session is at $X.XX
 *   of your $Y.YY budget (ZZ%). Want to wrap up or continue?"
 * REQ-PLG-112: When session cost exceeds the budget, the plugin MUST emit a
 *   clear alert but MUST NOT block the session. Cost budgets are advisory,
 *   not enforcement gates.
 *
 * Out of scope:
 * - REQ-PLG-113: Per-repo cost budgets (daily/weekly/monthly) with session history aggregation
 * - REQ-PLG-106: Session persistence
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-110, specs/70-OpenCode-Plugin.md#REQ-PLG-111, specs/70-OpenCode-Plugin.md#REQ-PLG-112
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-110 plan=phase-70-3/task-70-3-3/step-70-3-3-2
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Session budget configuration loaded from .axiom/plugin.yaml metrics section. */
export interface SessionBudgetConfig {
	/** Budget in USD, or null if no budget is configured. */
	session_budget_usd: number | null;
	/** Percentage threshold (1-99) at which to emit a warning. Default: 80. */
	warning_threshold_pct: number;
	/** Channel for alerts: "console", "webhook", or "slack". Default: "console". */
	alert_channel: string;
}

/** Result of evaluating current session cost against the budget. */
export interface BudgetEvaluationResult {
	/** "ok" = under threshold, "warning" = above threshold but under budget, "over_budget" = at or above budget. */
	status: "ok" | "warning" | "over_budget";
	/** Conversational warning message, or null if not in warning state. */
	warning: string | null;
	/** Conversational alert message, or null if not over budget. */
	alert: string | null;
	/** Current cost as a percentage of the budget, or null if no budget. */
	percentage: number | null;
	/** REQ-PLG-112: Always false — budgets are advisory, never blocking. */
	blocked: false;
	/** The alert channel from config. */
	alert_channel: string;
}

/** Input for formatting a budget warning message. */
interface BudgetWarningInput {
	currentCostUsd: number;
	budgetUsd: number;
	percentage: number;
}

/** Input for formatting a budget alert message. */
interface BudgetAlertInput {
	currentCostUsd: number;
	budgetUsd: number;
	percentage: number;
}

// ---------------------------------------------------------------------------
// REQ-PLG-110: Load session budget config from plugin.yaml
// ---------------------------------------------------------------------------

/**
 * Parse the session budget configuration from a parsed plugin.yaml object.
 *
 * Extracts `metrics.session_budget_usd`, `metrics.warning_threshold_pct`,
 * and `metrics.alert_channel` with sensible defaults:
 * - session_budget_usd: null (no budget)
 * - warning_threshold_pct: 80 (clamped to 1-99)
 * - alert_channel: "console"
 */
export function loadSessionBudgetConfig(
	pluginYaml: Record<string, unknown>,
): SessionBudgetConfig {
	const metrics = pluginYaml.metrics as Record<string, unknown> | undefined;

	// Budget: null if not specified or metrics section absent
	const rawBudget = metrics?.session_budget_usd;
	const sessionBudgetUsd =
		typeof rawBudget === "number" && rawBudget > 0 ? rawBudget : null;

	// Warning threshold: default 80, clamped to 1-99
	const rawThreshold = metrics?.warning_threshold_pct;
	let warningThresholdPct = 80;
	if (typeof rawThreshold === "number") {
		warningThresholdPct = Math.max(1, Math.min(99, rawThreshold));
	}

	// Alert channel: default "console"
	const rawChannel = metrics?.alert_channel;
	const alertChannel =
		typeof rawChannel === "string" && rawChannel.length > 0
			? rawChannel
			: "console";

	return {
		session_budget_usd: sessionBudgetUsd,
		warning_threshold_pct: warningThresholdPct,
		alert_channel: alertChannel,
	};
}

// ---------------------------------------------------------------------------
// REQ-PLG-111 / REQ-PLG-112: Evaluate session cost against budget
// ---------------------------------------------------------------------------

/**
 * Evaluate the current session cost against the configured budget.
 *
 * Returns:
 * - "ok" when cost is at or below the warning threshold, or no budget is set
 * - "warning" when cost exceeds the warning threshold but is below the budget
 * - "over_budget" when cost is at or above the budget
 *
 * REQ-PLG-112: `blocked` is always `false` — budgets are advisory.
 */
export function evaluateSessionBudget(
	currentCostUsd: number,
	config: SessionBudgetConfig,
): BudgetEvaluationResult {
	// No budget configured → always ok
	if (config.session_budget_usd === null) {
		return {
			status: "ok",
			warning: null,
			alert: null,
			percentage: null,
			blocked: false,
			alert_channel: config.alert_channel,
		};
	}

	const budget = config.session_budget_usd;
	const percentage = (currentCostUsd / budget) * 100;
	const thresholdPct = config.warning_threshold_pct;

	// At or above budget → over_budget (REQ-PLG-112)
	if (currentCostUsd >= budget) {
		return {
			status: "over_budget",
			warning: null,
			alert: formatBudgetAlert({
				currentCostUsd,
				budgetUsd: budget,
				percentage,
			}),
			percentage,
			blocked: false,
			alert_channel: config.alert_channel,
		};
	}

	// Above warning threshold but below budget → warning (REQ-PLG-111)
	if (percentage > thresholdPct) {
		return {
			status: "warning",
			warning: formatBudgetWarning({
				currentCostUsd,
				budgetUsd: budget,
				percentage,
			}),
			alert: null,
			percentage,
			blocked: false,
			alert_channel: config.alert_channel,
		};
	}

	// Below warning threshold → ok
	return {
		status: "ok",
		warning: null,
		alert: null,
		percentage,
		blocked: false,
		alert_channel: config.alert_channel,
	};
}

// ---------------------------------------------------------------------------
// REQ-PLG-111: Conversational warning formatter
// ---------------------------------------------------------------------------

/**
 * Format a conversational budget warning per the spec-defined template:
 * "Heads up — this session is at $4.12 of your $5.00 budget (82%).
 *  Want to wrap up or continue?"
 *
 * - Cost formatted to 2 decimal places
 * - Percentage rounded to nearest integer
 */
export function formatBudgetWarning(input: BudgetWarningInput): string {
	const cost = input.currentCostUsd.toFixed(2);
	const budget = input.budgetUsd.toFixed(2);
	const pct = Math.round(input.percentage);

	return `Heads up — this session is at $${cost} of your $${budget} budget (${pct}%). Want to wrap up or continue?`;
}

// ---------------------------------------------------------------------------
// REQ-PLG-112: Conversational over-budget alert formatter
// ---------------------------------------------------------------------------

/**
 * Format a conversational over-budget alert.
 *
 * REQ-PLG-112: The alert MUST be clear about the overage but MUST NOT
 * imply the session is blocked, stopped, or terminated. Budgets are advisory.
 *
 * - Cost formatted to 2 decimal places
 * - Percentage rounded to nearest integer
 */
export function formatBudgetAlert(input: BudgetAlertInput): string {
	const cost = input.currentCostUsd.toFixed(2);
	const budget = input.budgetUsd.toFixed(2);
	const pct = Math.round(input.percentage);

	return `This session has exceeded its budget — $${cost} of $${budget} (${pct}%). This is an advisory notice; the session will continue normally.`;
}
