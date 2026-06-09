/**
 * Metrics extraction and pricing tests — verifies the L2 metrics layer for
 * reading OpenCode session message files, computing costs from token counts,
 * and honoring pricing fallback/override behavior.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/layers/metrics
 * which does not exist yet. All tests are expected to fail at the module-
 * resolution level until the metrics layer is implemented.
 *
 * REQ-PLG-100: Plugin reads OpenCode session message files to extract token
 *   usage and cost data (data source per specs/47 §2).
 * REQ-PLG-101: Plugin computes running session cost using pricing table from
 *   specs/47 §3. Reads .axiom/cost-pricing.yaml if present, falls back to
 *   embedded default pricing. pricing_overrides in .axiom/plugin.yaml
 *   supports all 5 price fields (input, output, cache_read, cache_write,
 *   reasoning per 1M tokens).
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-100, specs/70-OpenCode-Plugin.md#REQ-PLG-101
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-100 plan=phase-70-3/task-70-3-1/step-70-3-1-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Import from the not-yet-existing metrics module — this WILL fail at module
// resolution, which is the expected test-first signal.
import {
	applyPricingOverrides,
	computeSessionCost,
	loadEmbeddedPricing,
	loadPricingTable,
	type MessageTokens,
	type PricingEntry,
	type PricingTable,
	parseSessionMessages,
	type SessionCostResult,
} from "../src/layers/metrics";

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Create a minimal OpenCode message file (assistant role with token data). */
function makeMessageFile(overrides: Record<string, unknown> = {}): object {
	return {
		id: "msg_test001",
		sessionID: "ses_test001",
		role: "assistant",
		parentID: "msg_test000",
		time: { created: 1735689601000, completed: 1735689615000 },
		modelID: "claude-3-5-sonnet-20241022",
		providerID: "anthropic",
		agent: "dev-axiom",
		mode: "primary",
		cost: 0,
		tokens: {
			input: 12500,
			output: 850,
			reasoning: 0,
			cache: { read: 8000, write: 4500 },
		},
		finish: "stop",
		error: null,
		summary: false,
		...overrides,
	};
}

/** Create a compaction summary message (should be excluded from aggregation). */
function makeCompactionMessage(
	overrides: Record<string, unknown> = {},
): object {
	return makeMessageFile({
		id: "msg_compact001",
		mode: "compaction",
		summary: true,
		tokens: {
			input: 50000,
			output: 2000,
			reasoning: 0,
			cache: { read: 0, write: 0 },
		},
		...overrides,
	});
}

/** Create a user-role message (no cost data). */
function makeUserMessage(overrides: Record<string, unknown> = {}): object {
	return {
		id: "msg_user001",
		sessionID: "ses_test001",
		role: "user",
		parentID: "",
		time: { created: 1735689600000, completed: 1735689600000 },
		modelID: "",
		providerID: "",
		agent: "",
		mode: "",
		cost: 0,
		tokens: { input: 0, output: 0, reasoning: 0, cache: { read: 0, write: 0 } },
		finish: "",
		error: null,
		summary: false,
		...overrides,
	};
}

/** Minimal cost-pricing.yaml content matching spec 47 §9.1 format. */
function makeCostPricingYaml(): string {
	return `version: "2026-02-01"
models:
  - id: "claude-3-5-sonnet-20241022"
    provider: "anthropic"
    aliases: ["claude-sonnet-4-20250514"]
    price_per_1m_input: 3.00
    price_per_1m_output: 15.00
    price_per_1m_cache_read: 0.30
    price_per_1m_cache_write: 3.75
    price_per_1m_reasoning: 0.00
  - id: "claude-3-5-haiku-20241022"
    provider: "anthropic"
    aliases: ["claude-haiku-4-20250414"]
    price_per_1m_input: 0.80
    price_per_1m_output: 4.00
    price_per_1m_cache_read: 0.08
    price_per_1m_cache_write: 1.00
    price_per_1m_reasoning: 0.00
`;
}

/** plugin.yaml with pricing_overrides section per REQ-PLG-101 format. */
function makePluginYamlWithOverrides(): string {
	return `pricing_overrides:
  "claude-3-5-sonnet-20241022":
    input_per_1m: 5.00
    output_per_1m: 20.00
    cache_read_per_1m: 0.50
    cache_write_per_1m: 5.00
    reasoning_per_1m: 20.00
`;
}

// ---------------------------------------------------------------------------
// Test setup / teardown
// ---------------------------------------------------------------------------

let testDir: string;

beforeEach(() => {
	testDir = join(
		tmpdir(),
		`axiom-metrics-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(testDir, { recursive: true });
});

afterEach(() => {
	try {
		rmSync(testDir, { recursive: true, force: true });
	} catch {
		// best-effort cleanup
	}
});

// ===========================================================================
// REQ-PLG-100: Session message file parsing
// ===========================================================================

describe("REQ-PLG-100: parseSessionMessages", () => {
	it("reads assistant messages from a session message directory", () => {
		// Arrange: create message directory with 2 assistant messages + 1 user message
		const msgDir = join(testDir, "storage", "message", "ses_test001");
		mkdirSync(msgDir, { recursive: true });
		writeFileSync(
			join(msgDir, "msg_001.json"),
			JSON.stringify(makeMessageFile({ id: "msg_001" })),
		);
		writeFileSync(
			join(msgDir, "msg_002.json"),
			JSON.stringify(
				makeMessageFile({
					id: "msg_002",
					modelID: "claude-3-5-haiku-20241022",
					tokens: {
						input: 5000,
						output: 300,
						reasoning: 0,
						cache: { read: 2000, write: 1000 },
					},
				}),
			),
		);
		writeFileSync(
			join(msgDir, "msg_user.json"),
			JSON.stringify(makeUserMessage()),
		);

		// Act
		const messages = parseSessionMessages(
			join(testDir, "storage", "message"),
			"ses_test001",
		);

		// Assert: only assistant messages returned
		expect(messages).toHaveLength(2);
		expect(
			messages.every((m: { role: string }) => m.role === "assistant"),
		).toBe(true);
	});

	it("excludes compaction summary messages from token aggregation", () => {
		const msgDir = join(testDir, "storage", "message", "ses_test001");
		mkdirSync(msgDir, { recursive: true });
		writeFileSync(
			join(msgDir, "msg_001.json"),
			JSON.stringify(makeMessageFile({ id: "msg_001" })),
		);
		writeFileSync(
			join(msgDir, "msg_compact.json"),
			JSON.stringify(makeCompactionMessage()),
		);

		const messages = parseSessionMessages(
			join(testDir, "storage", "message"),
			"ses_test001",
		);

		// Compaction messages (summary: true) must be excluded per REQ-COST-003
		expect(messages).toHaveLength(1);
		expect(messages[0].id).toBe("msg_001");
	});

	it("returns empty array when session directory does not exist", () => {
		const messages = parseSessionMessages(
			join(testDir, "storage", "message"),
			"ses_nonexistent",
		);
		expect(messages).toEqual([]);
	});

	it("extracts all 5 token fields from message files", () => {
		const msgDir = join(testDir, "storage", "message", "ses_test001");
		mkdirSync(msgDir, { recursive: true });
		writeFileSync(
			join(msgDir, "msg_001.json"),
			JSON.stringify(
				makeMessageFile({
					id: "msg_001",
					tokens: {
						input: 10000,
						output: 2000,
						reasoning: 500,
						cache: { read: 3000, write: 1500 },
					},
				}),
			),
		);

		const messages = parseSessionMessages(
			join(testDir, "storage", "message"),
			"ses_test001",
		);

		expect(messages).toHaveLength(1);
		const tokens: MessageTokens = messages[0].tokens;
		expect(tokens.input).toBe(10000);
		expect(tokens.output).toBe(2000);
		expect(tokens.reasoning).toBe(500);
		expect(tokens.cache_read).toBe(3000);
		expect(tokens.cache_write).toBe(1500);
	});

	it("skips malformed JSON files without crashing", () => {
		const msgDir = join(testDir, "storage", "message", "ses_test001");
		mkdirSync(msgDir, { recursive: true });
		writeFileSync(
			join(msgDir, "msg_001.json"),
			JSON.stringify(makeMessageFile({ id: "msg_001" })),
		);
		writeFileSync(join(msgDir, "msg_bad.json"), "{ not valid json !!!");

		const messages = parseSessionMessages(
			join(testDir, "storage", "message"),
			"ses_test001",
		);

		// Should still return the valid message
		expect(messages).toHaveLength(1);
		expect(messages[0].id).toBe("msg_001");
	});

	it("preserves modelID and providerID from message files", () => {
		const msgDir = join(testDir, "storage", "message", "ses_test001");
		mkdirSync(msgDir, { recursive: true });
		writeFileSync(
			join(msgDir, "msg_001.json"),
			JSON.stringify(
				makeMessageFile({
					id: "msg_001",
					modelID: "amazon-bedrock/anthropic.claude-sonnet-4-6",
					providerID: "amazon-bedrock",
				}),
			),
		);

		const messages = parseSessionMessages(
			join(testDir, "storage", "message"),
			"ses_test001",
		);

		expect(messages[0].modelID).toBe(
			"amazon-bedrock/anthropic.claude-sonnet-4-6",
		);
		expect(messages[0].providerID).toBe("amazon-bedrock");
	});
});

// ===========================================================================
// REQ-PLG-101: Pricing table loading and cost computation
// ===========================================================================

describe("REQ-PLG-101: loadPricingTable", () => {
	it("loads pricing from .axiom/cost-pricing.yaml when present", () => {
		const repoRoot = join(testDir, "repo");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);

		const table = loadPricingTable(repoRoot);

		expect(table.version).toBe("2026-02-01");
		expect(table.models).toHaveLength(2);
		expect(table.models[0].id).toBe("claude-3-5-sonnet-20241022");
		expect(table.models[0].price_per_1m_input).toBe(3.0);
		expect(table.models[0].price_per_1m_output).toBe(15.0);
		expect(table.models[0].price_per_1m_cache_read).toBe(0.3);
		expect(table.models[0].price_per_1m_cache_write).toBe(3.75);
		expect(table.models[0].price_per_1m_reasoning).toBe(0.0);
	});

	it("falls back to embedded default pricing when cost-pricing.yaml is absent", () => {
		const repoRoot = join(testDir, "repo-no-pricing");
		mkdirSync(repoRoot, { recursive: true });

		const table = loadPricingTable(repoRoot);

		// Embedded pricing must exist and contain at least one model
		expect(table.version).toBeTruthy();
		expect(table.models.length).toBeGreaterThan(0);
		// Embedded pricing must include the 5 price fields
		const firstModel = table.models[0];
		expect(typeof firstModel.price_per_1m_input).toBe("number");
		expect(typeof firstModel.price_per_1m_output).toBe("number");
		expect(typeof firstModel.price_per_1m_cache_read).toBe("number");
		expect(typeof firstModel.price_per_1m_cache_write).toBe("number");
		expect(typeof firstModel.price_per_1m_reasoning).toBe("number");
	});

	it("resolves model aliases from the pricing table", () => {
		const repoRoot = join(testDir, "repo-alias");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);

		const table = loadPricingTable(repoRoot);

		// "claude-sonnet-4-20250514" is an alias for "claude-3-5-sonnet-20241022"
		const entry = table.models.find(
			(m: PricingEntry) => m.id === "claude-3-5-sonnet-20241022",
		);
		expect(entry).toBeDefined();
		expect(entry?.aliases).toContain("claude-sonnet-4-20250514");
	});
});

describe("REQ-PLG-101: loadEmbeddedPricing", () => {
	it("returns a valid pricing table without any file on disk", () => {
		const table = loadEmbeddedPricing();

		expect(table.version).toBeTruthy();
		expect(table.models.length).toBeGreaterThan(0);
		// Must include at least one Anthropic model
		const hasAnthropic = table.models.some(
			(m: PricingEntry) => m.provider === "anthropic",
		);
		expect(hasAnthropic).toBe(true);
	});

	it("embedded pricing includes all 5 price fields per model", () => {
		const table = loadEmbeddedPricing();

		for (const model of table.models) {
			expect(typeof model.price_per_1m_input).toBe("number");
			expect(typeof model.price_per_1m_output).toBe("number");
			expect(typeof model.price_per_1m_cache_read).toBe("number");
			expect(typeof model.price_per_1m_cache_write).toBe("number");
			expect(typeof model.price_per_1m_reasoning).toBe("number");
		}
	});
});

describe("REQ-PLG-101: applyPricingOverrides", () => {
	it("applies pricing_overrides from plugin.yaml over the base table", () => {
		const repoRoot = join(testDir, "repo-override");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);
		writeFileSync(
			join(repoRoot, ".axiom", "plugin.yaml"),
			makePluginYamlWithOverrides(),
		);

		const baseTable = loadPricingTable(repoRoot);
		const overridden = applyPricingOverrides(baseTable, repoRoot);

		// The override should replace the sonnet pricing
		const sonnet = overridden.models.find(
			(m: PricingEntry) => m.id === "claude-3-5-sonnet-20241022",
		);
		expect(sonnet).toBeDefined();
		expect(sonnet?.price_per_1m_input).toBe(5.0);
		expect(sonnet?.price_per_1m_output).toBe(20.0);
		expect(sonnet?.price_per_1m_cache_read).toBe(0.5);
		expect(sonnet?.price_per_1m_cache_write).toBe(5.0);
		expect(sonnet?.price_per_1m_reasoning).toBe(20.0);
	});

	it("leaves non-overridden models unchanged", () => {
		const repoRoot = join(testDir, "repo-partial-override");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);
		// Override only sonnet, not haiku
		writeFileSync(
			join(repoRoot, ".axiom", "plugin.yaml"),
			makePluginYamlWithOverrides(),
		);

		const baseTable = loadPricingTable(repoRoot);
		const overridden = applyPricingOverrides(baseTable, repoRoot);

		const haiku = overridden.models.find(
			(m: PricingEntry) => m.id === "claude-3-5-haiku-20241022",
		);
		expect(haiku).toBeDefined();
		// Haiku prices should remain at base values
		expect(haiku?.price_per_1m_input).toBe(0.8);
		expect(haiku?.price_per_1m_output).toBe(4.0);
	});

	it("returns base table unchanged when plugin.yaml has no pricing_overrides", () => {
		const repoRoot = join(testDir, "repo-no-override");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);
		writeFileSync(
			join(repoRoot, ".axiom", "plugin.yaml"),
			"some_other_key: true\n",
		);

		const baseTable = loadPricingTable(repoRoot);
		const overridden = applyPricingOverrides(baseTable, repoRoot);

		expect(overridden.models).toEqual(baseTable.models);
	});

	it("returns base table unchanged when plugin.yaml does not exist", () => {
		const repoRoot = join(testDir, "repo-no-plugin-yaml");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);

		const baseTable = loadPricingTable(repoRoot);
		const overridden = applyPricingOverrides(baseTable, repoRoot);

		expect(overridden.models).toEqual(baseTable.models);
	});

	it("supports all 5 price fields in pricing_overrides per REQ-PLG-101", () => {
		const repoRoot = join(testDir, "repo-5fields");
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(
			join(repoRoot, ".axiom", "cost-pricing.yaml"),
			makeCostPricingYaml(),
		);
		writeFileSync(
			join(repoRoot, ".axiom", "plugin.yaml"),
			`pricing_overrides:
  "claude-3-5-sonnet-20241022":
    input_per_1m: 1.11
    output_per_1m: 2.22
    cache_read_per_1m: 3.33
    cache_write_per_1m: 4.44
    reasoning_per_1m: 5.55
`,
		);

		const baseTable = loadPricingTable(repoRoot);
		const overridden = applyPricingOverrides(baseTable, repoRoot);

		const sonnet = overridden.models.find(
			(m: PricingEntry) => m.id === "claude-3-5-sonnet-20241022",
		);
		expect(sonnet?.price_per_1m_input).toBe(1.11);
		expect(sonnet?.price_per_1m_output).toBe(2.22);
		expect(sonnet?.price_per_1m_cache_read).toBe(3.33);
		expect(sonnet?.price_per_1m_cache_write).toBe(4.44);
		expect(sonnet?.price_per_1m_reasoning).toBe(5.55);
	});
});

// ===========================================================================
// REQ-PLG-101: Cost computation from tokens + pricing
// ===========================================================================

describe("REQ-PLG-101: computeSessionCost", () => {
	it("computes cost from token counts using the spec 47 §3.2 formula", () => {
		// Formula: cost = sum of (price_per_1m_X / 1_000_000 * tokens.X) for all 5 fields
		const messages = [
			{
				id: "msg_001",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 12500,
					output: 850,
					reasoning: 0,
					cache_read: 8000,
					cache_write: 4500,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [
				{
					id: "claude-3-5-sonnet-20241022",
					provider: "anthropic",
					aliases: [],
					price_per_1m_input: 3.0,
					price_per_1m_output: 15.0,
					price_per_1m_cache_read: 0.3,
					price_per_1m_cache_write: 3.75,
					price_per_1m_reasoning: 0.0,
				},
			],
		};

		const result: SessionCostResult = computeSessionCost(messages, pricing);

		// Expected cost:
		// input:       3.00 / 1M * 12500 = 0.0375
		// output:     15.00 / 1M * 850   = 0.01275
		// cache_read:  0.30 / 1M * 8000  = 0.0024
		// cache_write: 3.75 / 1M * 4500  = 0.016875
		// reasoning:   0.00 / 1M * 0     = 0.0
		// Total: 0.069525
		expect(result.total_cost_usd).toBeCloseTo(0.069525, 5);
	});

	it("aggregates cost across multiple messages", () => {
		const messages = [
			{
				id: "msg_001",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 10000,
					output: 1000,
					reasoning: 0,
					cache_read: 5000,
					cache_write: 2000,
				},
			},
			{
				id: "msg_002",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 8000,
					output: 500,
					reasoning: 0,
					cache_read: 3000,
					cache_write: 1000,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [
				{
					id: "claude-3-5-sonnet-20241022",
					provider: "anthropic",
					aliases: [],
					price_per_1m_input: 3.0,
					price_per_1m_output: 15.0,
					price_per_1m_cache_read: 0.3,
					price_per_1m_cache_write: 3.75,
					price_per_1m_reasoning: 0.0,
				},
			],
		};

		const result = computeSessionCost(messages, pricing);

		// msg_001: (3*10000 + 15*1000 + 0.3*5000 + 3.75*2000 + 0*0) / 1M = 0.054
		// msg_002: (3*8000 + 15*500 + 0.3*3000 + 3.75*1000 + 0*0) / 1M = 0.036150
		// Total: 0.090150
		expect(result.total_cost_usd).toBeCloseTo(0.09015, 4);
		expect(result.by_model).toHaveLength(1);
		expect(result.by_model[0].calls).toBe(2);
	});

	it("groups costs by model in the by_model breakdown", () => {
		const messages = [
			{
				id: "msg_001",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 10000,
					output: 1000,
					reasoning: 0,
					cache_read: 0,
					cache_write: 0,
				},
			},
			{
				id: "msg_002",
				modelID: "claude-3-5-haiku-20241022",
				tokens: {
					input: 5000,
					output: 500,
					reasoning: 0,
					cache_read: 0,
					cache_write: 0,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [
				{
					id: "claude-3-5-sonnet-20241022",
					provider: "anthropic",
					aliases: [],
					price_per_1m_input: 3.0,
					price_per_1m_output: 15.0,
					price_per_1m_cache_read: 0.3,
					price_per_1m_cache_write: 3.75,
					price_per_1m_reasoning: 0.0,
				},
				{
					id: "claude-3-5-haiku-20241022",
					provider: "anthropic",
					aliases: [],
					price_per_1m_input: 0.8,
					price_per_1m_output: 4.0,
					price_per_1m_cache_read: 0.08,
					price_per_1m_cache_write: 1.0,
					price_per_1m_reasoning: 0.0,
				},
			],
		};

		const result = computeSessionCost(messages, pricing);

		expect(result.by_model).toHaveLength(2);
		const sonnetEntry = result.by_model.find(
			(e: { model: string }) => e.model === "claude-3-5-sonnet-20241022",
		);
		const haikuEntry = result.by_model.find(
			(e: { model: string }) => e.model === "claude-3-5-haiku-20241022",
		);
		expect(sonnetEntry).toBeDefined();
		expect(haikuEntry).toBeDefined();
		expect(sonnetEntry?.calls).toBe(1);
		expect(haikuEntry?.calls).toBe(1);
	});

	it("resolves model aliases when computing cost", () => {
		const messages = [
			{
				id: "msg_001",
				modelID: "claude-sonnet-4-20250514", // alias for claude-3-5-sonnet-20241022
				tokens: {
					input: 10000,
					output: 1000,
					reasoning: 0,
					cache_read: 0,
					cache_write: 0,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [
				{
					id: "claude-3-5-sonnet-20241022",
					provider: "anthropic",
					aliases: ["claude-sonnet-4-20250514"],
					price_per_1m_input: 3.0,
					price_per_1m_output: 15.0,
					price_per_1m_cache_read: 0.3,
					price_per_1m_cache_write: 3.75,
					price_per_1m_reasoning: 0.0,
				},
			],
		};

		const result = computeSessionCost(messages, pricing);

		// Should resolve alias and compute: (3*10000 + 15*1000) / 1M = 0.045
		expect(result.total_cost_usd).toBeCloseTo(0.045, 5);
	});

	it("records null cost for unknown models without crashing", () => {
		const messages = [
			{
				id: "msg_001",
				modelID: "unknown-model-xyz",
				tokens: {
					input: 10000,
					output: 1000,
					reasoning: 0,
					cache_read: 0,
					cache_write: 0,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [],
		};

		const result = computeSessionCost(messages, pricing);

		// Unknown model: cost should be null per REQ-COST-005
		expect(result.by_model).toHaveLength(1);
		expect(result.by_model[0].cost_usd).toBeNull();
	});

	it("includes reasoning tokens in cost calculation", () => {
		const messages = [
			{
				id: "msg_001",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 0,
					output: 0,
					reasoning: 10000,
					cache_read: 0,
					cache_write: 0,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [
				{
					id: "claude-3-5-sonnet-20241022",
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

		const result = computeSessionCost(messages, pricing);

		// reasoning: 15.00 / 1M * 10000 = 0.15
		expect(result.total_cost_usd).toBeCloseTo(0.15, 5);
	});

	it("returns zero cost for empty message list", () => {
		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [],
		};

		const result = computeSessionCost([], pricing);

		expect(result.total_cost_usd).toBe(0);
		expect(result.by_model).toEqual([]);
	});

	it("aggregates total token counts across all messages", () => {
		const messages = [
			{
				id: "msg_001",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 10000,
					output: 1000,
					reasoning: 200,
					cache_read: 5000,
					cache_write: 2000,
				},
			},
			{
				id: "msg_002",
				modelID: "claude-3-5-sonnet-20241022",
				tokens: {
					input: 8000,
					output: 500,
					reasoning: 100,
					cache_read: 3000,
					cache_write: 1000,
				},
			},
		];

		const pricing: PricingTable = {
			version: "2026-02-01",
			models: [
				{
					id: "claude-3-5-sonnet-20241022",
					provider: "anthropic",
					aliases: [],
					price_per_1m_input: 3.0,
					price_per_1m_output: 15.0,
					price_per_1m_cache_read: 0.3,
					price_per_1m_cache_write: 3.75,
					price_per_1m_reasoning: 0.0,
				},
			],
		};

		const result = computeSessionCost(messages, pricing);

		expect(result.total_tokens.input).toBe(18000);
		expect(result.total_tokens.output).toBe(1500);
		expect(result.total_tokens.reasoning).toBe(300);
		expect(result.total_tokens.cache_read).toBe(8000);
		expect(result.total_tokens.cache_write).toBe(3000);
	});
});
