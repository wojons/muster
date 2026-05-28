/**
 * Session cost budget and alert contract tests — verifies per-session budget
 * config loading, conversational warning when the warning threshold is exceeded,
 * and advisory over-budget alerts that MUST NOT block the session.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/layers/metrics-budget
 * which does not exist yet. All tests are expected to fail at the module-
 * resolution level until the budget evaluator is implemented.
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
 * Out of scope for this slice:
 * - REQ-PLG-113: Per-repo cost budgets (daily/weekly/monthly) with session history aggregation
 * - REQ-PLG-106: Session persistence
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-110, specs/70-OpenCode-Plugin.md#REQ-PLG-111, specs/70-OpenCode-Plugin.md#REQ-PLG-112
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-110 plan=phase-70-3/task-70-3-3/step-70-3-3-1
 */
import { describe, expect, it } from "bun:test";

// Import from the not-yet-existing metrics-budget module — this WILL fail at
// module resolution, which is the expected test-first signal.
import {
	type BudgetEvaluationResult,
	evaluateSessionBudget,
	formatBudgetAlert,
	formatBudgetWarning,
	loadSessionBudgetConfig,
	type SessionBudgetConfig,
} from "../src/layers/metrics-budget";

// ===========================================================================
// REQ-PLG-110: Session budget config loading from .axiom/plugin.yaml
// ===========================================================================

describe("REQ-PLG-110: loadSessionBudgetConfig", () => {
	it("parses the spec-defined plugin.yaml metrics section", () => {
		// Spec example from REQ-PLG-110:
		// metrics:
		//   session_budget_usd: 5.00
		//   warning_threshold_pct: 80
		//   alert_channel: "console"
		const pluginYaml = {
			metrics: {
				session_budget_usd: 5.0,
				warning_threshold_pct: 80,
				alert_channel: "console",
			},
		};

		const config: SessionBudgetConfig = loadSessionBudgetConfig(pluginYaml);

		expect(config.session_budget_usd).toBe(5.0);
		expect(config.warning_threshold_pct).toBe(80);
		expect(config.alert_channel).toBe("console");
	});

	it("returns null budget when metrics section is absent", () => {
		const pluginYaml = {};

		const config = loadSessionBudgetConfig(pluginYaml);

		// No budget configured — budget evaluation should be a no-op
		expect(config.session_budget_usd).toBeNull();
		expect(config.warning_threshold_pct).toBe(80); // default
		expect(config.alert_channel).toBe("console"); // default
	});

	it("returns null budget when session_budget_usd is missing", () => {
		const pluginYaml = {
			metrics: {
				warning_threshold_pct: 90,
			},
		};

		const config = loadSessionBudgetConfig(pluginYaml);

		expect(config.session_budget_usd).toBeNull();
		expect(config.warning_threshold_pct).toBe(90);
	});

	it("uses default warning_threshold_pct of 80 when not specified", () => {
		const pluginYaml = {
			metrics: {
				session_budget_usd: 10.0,
			},
		};

		const config = loadSessionBudgetConfig(pluginYaml);

		expect(config.session_budget_usd).toBe(10.0);
		expect(config.warning_threshold_pct).toBe(80);
	});

	it("supports webhook and slack alert channels", () => {
		const webhookYaml = {
			metrics: {
				session_budget_usd: 5.0,
				alert_channel: "webhook",
			},
		};
		const slackYaml = {
			metrics: {
				session_budget_usd: 5.0,
				alert_channel: "slack",
			},
		};

		expect(loadSessionBudgetConfig(webhookYaml).alert_channel).toBe("webhook");
		expect(loadSessionBudgetConfig(slackYaml).alert_channel).toBe("slack");
	});

	it("clamps warning_threshold_pct to 1-99 range", () => {
		const tooLow = {
			metrics: {
				session_budget_usd: 5.0,
				warning_threshold_pct: 0,
			},
		};
		const tooHigh = {
			metrics: {
				session_budget_usd: 5.0,
				warning_threshold_pct: 100,
			},
		};

		expect(loadSessionBudgetConfig(tooLow).warning_threshold_pct).toBe(1);
		expect(loadSessionBudgetConfig(tooHigh).warning_threshold_pct).toBe(99);
	});
});

// ===========================================================================
// REQ-PLG-111: Warning when session cost exceeds warning threshold
// ===========================================================================

describe("REQ-PLG-111: evaluateSessionBudget warning behavior", () => {
	it("returns 'ok' when cost is below warning threshold", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 5.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		const result: BudgetEvaluationResult = evaluateSessionBudget(3.0, config);

		expect(result.status).toBe("ok");
		expect(result.warning).toBeNull();
		expect(result.alert).toBeNull();
	});

	it("returns 'warning' when cost exceeds warning threshold but not budget", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 5.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		// $4.12 is 82.4% of $5.00 — above 80% threshold
		const result = evaluateSessionBudget(4.12, config);

		expect(result.status).toBe("warning");
		expect(result.warning).not.toBeNull();
		expect(result.alert).toBeNull();
		expect(result.percentage).toBeCloseTo(82.4, 0);
	});

	it("returns 'ok' when no budget is configured (null budget)", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: null,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		const result = evaluateSessionBudget(100.0, config);

		// No budget → no evaluation → always ok
		expect(result.status).toBe("ok");
		expect(result.warning).toBeNull();
		expect(result.alert).toBeNull();
	});

	it("returns 'ok' when cost is exactly at the warning threshold boundary", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 10.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		// $8.00 is exactly 80% — at the boundary, not exceeding
		const result = evaluateSessionBudget(8.0, config);

		expect(result.status).toBe("ok");
	});

	it("returns 'warning' when cost is just above the warning threshold", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 10.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		// $8.01 is 80.1% — just above 80% threshold
		const result = evaluateSessionBudget(8.01, config);

		expect(result.status).toBe("warning");
		expect(result.warning).not.toBeNull();
	});
});

// ===========================================================================
// REQ-PLG-111: Conversational warning copy
// ===========================================================================

describe("REQ-PLG-111: formatBudgetWarning conversational copy", () => {
	it("produces the spec-defined warning format", () => {
		// Spec example: "Heads up — this session is at $4.12 of your $5.00 budget (82%). Want to wrap up or continue?"
		const warning = formatBudgetWarning({
			currentCostUsd: 4.12,
			budgetUsd: 5.0,
			percentage: 82.4,
		});

		expect(warning).toContain("Heads up");
		expect(warning).toContain("$4.12");
		expect(warning).toContain("$5.00");
		expect(warning).toContain("82%");
		expect(warning).toContain("wrap up or continue");
	});

	it("rounds percentage to nearest integer", () => {
		const warning = formatBudgetWarning({
			currentCostUsd: 4.567,
			budgetUsd: 5.0,
			percentage: 91.34,
		});

		expect(warning).toContain("91%");
		expect(warning).not.toContain("91.34");
	});

	it("formats cost with two decimal places", () => {
		const warning = formatBudgetWarning({
			currentCostUsd: 4.1,
			budgetUsd: 5.0,
			percentage: 82,
		});

		expect(warning).toContain("$4.10");
		expect(warning).toContain("$5.00");
	});
});

// ===========================================================================
// REQ-PLG-112: Over-budget alert (advisory, non-blocking)
// ===========================================================================

describe("REQ-PLG-112: evaluateSessionBudget over-budget alert", () => {
	it("returns 'over_budget' when cost exceeds the budget", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 5.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		const result = evaluateSessionBudget(5.5, config);

		expect(result.status).toBe("over_budget");
		expect(result.alert).not.toBeNull();
		// REQ-PLG-112: MUST NOT block the session
		expect(result.blocked).toBe(false);
	});

	it("returns 'over_budget' when cost is exactly at the budget", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 5.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		// $5.00 is exactly 100% — at the budget boundary
		const result = evaluateSessionBudget(5.0, config);

		// At-budget is over-budget (>= 100%)
		expect(result.status).toBe("over_budget");
		expect(result.alert).not.toBeNull();
		expect(result.blocked).toBe(false);
	});

	it("never sets blocked=true regardless of how far over budget", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 5.0,
			warning_threshold_pct: 80,
			alert_channel: "console",
		};

		// 10x over budget — still advisory
		const result = evaluateSessionBudget(50.0, config);

		expect(result.status).toBe("over_budget");
		expect(result.blocked).toBe(false);
	});

	it("includes the alert_channel in the evaluation result", () => {
		const config: SessionBudgetConfig = {
			session_budget_usd: 5.0,
			warning_threshold_pct: 80,
			alert_channel: "webhook",
		};

		const result = evaluateSessionBudget(6.0, config);

		expect(result.alert_channel).toBe("webhook");
	});
});

// ===========================================================================
// REQ-PLG-112: Conversational over-budget alert copy
// ===========================================================================

describe("REQ-PLG-112: formatBudgetAlert conversational copy", () => {
	it("produces a clear advisory alert that does not imply blocking", () => {
		const alert = formatBudgetAlert({
			currentCostUsd: 6.5,
			budgetUsd: 5.0,
			percentage: 130,
		});

		// Must be clear about the overage
		expect(alert).toContain("$6.50");
		expect(alert).toContain("$5.00");
		// Must NOT imply the session is blocked or stopped
		expect(alert.toLowerCase()).not.toContain("blocked");
		expect(alert.toLowerCase()).not.toContain("stopped");
		expect(alert.toLowerCase()).not.toContain("terminated");
		// Should indicate advisory nature
		expect(alert.toLowerCase()).toContain("budget");
	});

	it("formats cost with two decimal places", () => {
		const alert = formatBudgetAlert({
			currentCostUsd: 5.1,
			budgetUsd: 5.0,
			percentage: 102,
		});

		expect(alert).toContain("$5.10");
		expect(alert).toContain("$5.00");
	});

	it("includes percentage in the alert", () => {
		const alert = formatBudgetAlert({
			currentCostUsd: 7.5,
			budgetUsd: 5.0,
			percentage: 150,
		});

		expect(alert).toContain("150%");
	});
});
