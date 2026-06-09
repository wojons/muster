/**
 * L2 Metrics Layer — session message parsing, pricing table loading, and cost
 * computation for the Axiom OpenCode plugin.
 *
 * REQ-PLG-100: Read OpenCode session message files to extract token usage and
 *   cost data (data source per specs/47 Section 2).
 * REQ-PLG-101: Compute running session cost using pricing table from specs/47
 *   Section 3. Read .axiom/cost-pricing.yaml if present, fall back to
 *   embedded default pricing. pricing_overrides in .axiom/plugin.yaml
 *   supports all 5 price fields.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-100, specs/70-OpenCode-Plugin.md#REQ-PLG-101
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-100 plan=phase-70-3/task-70-3-1/step-70-3-1-2
 */

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Token counts extracted from a single message file. */
export interface MessageTokens {
	input: number;
	output: number;
	reasoning: number;
	cache_read: number;
	cache_write: number;
}

/** Parsed assistant message with token data and model info. */
export interface ParsedMessage {
	id: string;
	role: string;
	modelID: string;
	providerID: string;
	tokens: MessageTokens;
}

/** A single model entry in the pricing table. */
export interface PricingEntry {
	id: string;
	provider: string;
	aliases: string[];
	price_per_1m_input: number;
	price_per_1m_output: number;
	price_per_1m_cache_read: number;
	price_per_1m_cache_write: number;
	price_per_1m_reasoning: number;
}

/** The full pricing table (from .axiom/cost-pricing.yaml or embedded). */
export interface PricingTable {
	version: string;
	models: PricingEntry[];
}

/** Per-model cost breakdown entry. */
export interface ModelCostEntry {
	model: string;
	calls: number;
	cost_usd: number | null;
	tokens: MessageTokens;
}

/** Result of computing session cost across all messages. */
export interface SessionCostResult {
	total_cost_usd: number;
	total_tokens: MessageTokens;
	by_model: ModelCostEntry[];
}

// ---------------------------------------------------------------------------
// REQ-PLG-100: Session message file parsing
// ---------------------------------------------------------------------------

/**
 * Parse OpenCode session message files for a given session ID.
 *
 * Reads all JSON files from `<messageDir>/<sessionId>/`, filters to assistant
 * role messages, excludes compaction summaries (summary: true), and extracts
 * the 5 token fields.
 *
 * Returns an empty array if the session directory does not exist.
 * Skips malformed JSON files without crashing.
 */
export function parseSessionMessages(
	messageDir: string,
	sessionId: string,
): ParsedMessage[] {
	const sessionDir = join(messageDir, sessionId);

	if (!existsSync(sessionDir)) {
		return [];
	}

	const entries = readdirSync(sessionDir).filter((f) => f.endsWith(".json"));
	const messages: ParsedMessage[] = [];

	for (const entry of entries) {
		try {
			const raw = readFileSync(join(sessionDir, entry), "utf-8");
			const data = JSON.parse(raw) as Record<string, unknown>;

			// Only assistant messages carry cost-relevant token data
			if (data.role !== "assistant") continue;

			// Exclude compaction summary messages per REQ-COST-003
			if (data.summary === true) continue;

			const tokens = data.tokens as Record<string, unknown> | undefined;
			const cache = (tokens?.cache ?? {}) as Record<string, unknown>;

			messages.push({
				id: String(data.id ?? ""),
				role: "assistant",
				modelID: String(data.modelID ?? ""),
				providerID: String(data.providerID ?? ""),
				tokens: {
					input: Number(tokens?.input ?? 0),
					output: Number(tokens?.output ?? 0),
					reasoning: Number(tokens?.reasoning ?? 0),
					cache_read: Number(cache.read ?? 0),
					cache_write: Number(cache.write ?? 0),
				},
			});
		} catch {}
	}

	return messages;
}

// ---------------------------------------------------------------------------
// REQ-PLG-101: Pricing table loading
// ---------------------------------------------------------------------------

/**
 * Embedded default pricing table — used when .axiom/cost-pricing.yaml is
 * absent. Matches the spec 47 Section 9.1 reference pricing.
 */
const EMBEDDED_PRICING: PricingTable = {
	version: "2026-02-01",
	models: [
		{
			id: "claude-3-5-sonnet-20241022",
			provider: "anthropic",
			aliases: ["claude-sonnet-4-20250514", "claude-3-5-sonnet"],
			price_per_1m_input: 3.0,
			price_per_1m_output: 15.0,
			price_per_1m_cache_read: 0.3,
			price_per_1m_cache_write: 3.75,
			price_per_1m_reasoning: 0.0,
		},
		{
			id: "claude-3-5-haiku-20241022",
			provider: "anthropic",
			aliases: ["claude-haiku-4-20250414"],
			price_per_1m_input: 0.8,
			price_per_1m_output: 4.0,
			price_per_1m_cache_read: 0.08,
			price_per_1m_cache_write: 1.0,
			price_per_1m_reasoning: 0.0,
		},
		{
			id: "amazon-bedrock/anthropic.claude-sonnet-4-6",
			provider: "amazon-bedrock",
			aliases: [],
			price_per_1m_input: 3.0,
			price_per_1m_output: 15.0,
			price_per_1m_cache_read: 0.3,
			price_per_1m_cache_write: 3.75,
			price_per_1m_reasoning: 0.0,
		},
		{
			id: "opencode/big-pickle",
			provider: "opencode",
			aliases: [],
			price_per_1m_input: 0.0,
			price_per_1m_output: 0.0,
			price_per_1m_cache_read: 0.0,
			price_per_1m_cache_write: 0.0,
			price_per_1m_reasoning: 0.0,
		},
	],
};

/** Return the embedded default pricing table (no file I/O). */
export function loadEmbeddedPricing(): PricingTable {
	// Deep-clone to prevent mutation of the constant
	return {
		version: EMBEDDED_PRICING.version,
		models: EMBEDDED_PRICING.models.map((m) => ({
			...m,
			aliases: [...m.aliases],
		})),
	};
}

/**
 * Load the pricing table for a repo.
 *
 * Reads `.axiom/cost-pricing.yaml` if it exists; otherwise falls back to
 * the embedded default pricing table.
 */
export function loadPricingTable(repoRoot: string): PricingTable {
	const pricingPath = join(repoRoot, ".axiom", "cost-pricing.yaml");

	if (!existsSync(pricingPath)) {
		return loadEmbeddedPricing();
	}

	try {
		const content = readFileSync(pricingPath, "utf-8");
		return parseCostPricingYaml(content);
	} catch {
		// If parsing fails, fall back to embedded pricing
		return loadEmbeddedPricing();
	}
}

// ---------------------------------------------------------------------------
// REQ-PLG-101: Pricing overrides from plugin.yaml
// ---------------------------------------------------------------------------

/**
 * Apply pricing_overrides from `.axiom/plugin.yaml` over the base pricing
 * table. Returns a new table with overridden prices; non-overridden models
 * are unchanged.
 *
 * Override format in plugin.yaml:
 * ```yaml
 * pricing_overrides:
 *   "claude-3-5-sonnet-20241022":
 *     input_per_1m: 5.00
 *     output_per_1m: 20.00
 *     cache_read_per_1m: 0.50
 *     cache_write_per_1m: 5.00
 *     reasoning_per_1m: 20.00
 * ```
 */
export function applyPricingOverrides(
	baseTable: PricingTable,
	repoRoot: string,
): PricingTable {
	const pluginYamlPath = join(repoRoot, ".axiom", "plugin.yaml");

	if (!existsSync(pluginYamlPath)) {
		return baseTable;
	}

	try {
		const content = readFileSync(pluginYamlPath, "utf-8");
		const overrides = parsePricingOverrides(content);

		if (Object.keys(overrides).length === 0) {
			return baseTable;
		}

		const newModels = baseTable.models.map((model) => {
			const override = overrides[model.id];
			if (!override) return model;

			return {
				...model,
				aliases: [...model.aliases],
				price_per_1m_input: override.input_per_1m ?? model.price_per_1m_input,
				price_per_1m_output:
					override.output_per_1m ?? model.price_per_1m_output,
				price_per_1m_cache_read:
					override.cache_read_per_1m ?? model.price_per_1m_cache_read,
				price_per_1m_cache_write:
					override.cache_write_per_1m ?? model.price_per_1m_cache_write,
				price_per_1m_reasoning:
					override.reasoning_per_1m ?? model.price_per_1m_reasoning,
			};
		});

		return { version: baseTable.version, models: newModels };
	} catch {
		return baseTable;
	}
}

// ---------------------------------------------------------------------------
// REQ-PLG-101: Cost computation (spec 47 Section 3.2 formula)
// ---------------------------------------------------------------------------

/**
 * Compute session cost from parsed messages and a pricing table.
 *
 * Formula per message (spec 47 Section 3.2):
 * ```
 * cost_usd = (price_per_1m_cache_write / 1_000_000 * tokens.cache_write)
 *          + (price_per_1m_cache_read  / 1_000_000 * tokens.cache_read)
 *          + (price_per_1m_input       / 1_000_000 * tokens.input)
 *          + (price_per_1m_output      / 1_000_000 * tokens.output)
 *          + (price_per_1m_reasoning   / 1_000_000 * tokens.reasoning)
 * ```
 *
 * Unknown models (not in pricing table, not an alias) get `cost_usd: null`
 * per REQ-COST-005.
 */
export function computeSessionCost(
	messages: Array<{ id: string; modelID: string; tokens: MessageTokens }>,
	pricing: PricingTable,
): SessionCostResult {
	if (messages.length === 0) {
		return {
			total_cost_usd: 0,
			total_tokens: {
				input: 0,
				output: 0,
				reasoning: 0,
				cache_read: 0,
				cache_write: 0,
			},
			by_model: [],
		};
	}

	// Build alias lookup: alias -> canonical model id
	const aliasMap = new Map<string, string>();
	for (const model of pricing.models) {
		for (const alias of model.aliases) {
			aliasMap.set(alias, model.id);
		}
	}

	// Build model pricing lookup: canonical id -> PricingEntry
	const pricingMap = new Map<string, PricingEntry>();
	for (const model of pricing.models) {
		pricingMap.set(model.id, model);
	}

	// Accumulate per-model stats
	const modelStats = new Map<
		string,
		{
			calls: number;
			cost_usd: number | null;
			tokens: MessageTokens;
		}
	>();

	const totalTokens: MessageTokens = {
		input: 0,
		output: 0,
		reasoning: 0,
		cache_read: 0,
		cache_write: 0,
	};

	for (const msg of messages) {
		// Resolve alias to canonical model ID
		const canonicalId = aliasMap.get(msg.modelID) ?? msg.modelID;
		const entry = pricingMap.get(canonicalId);

		// Accumulate total tokens
		totalTokens.input += msg.tokens.input;
		totalTokens.output += msg.tokens.output;
		totalTokens.reasoning += msg.tokens.reasoning;
		totalTokens.cache_read += msg.tokens.cache_read;
		totalTokens.cache_write += msg.tokens.cache_write;

		// Get or create per-model accumulator
		const displayModel = canonicalId;
		let stat = modelStats.get(displayModel);
		if (!stat) {
			stat = {
				calls: 0,
				cost_usd: entry ? 0 : null,
				tokens: {
					input: 0,
					output: 0,
					reasoning: 0,
					cache_read: 0,
					cache_write: 0,
				},
			};
			modelStats.set(displayModel, stat);
		}

		stat.calls += 1;
		stat.tokens.input += msg.tokens.input;
		stat.tokens.output += msg.tokens.output;
		stat.tokens.reasoning += msg.tokens.reasoning;
		stat.tokens.cache_read += msg.tokens.cache_read;
		stat.tokens.cache_write += msg.tokens.cache_write;

		if (entry && stat.cost_usd !== null) {
			// Spec 47 Section 3.2 formula
			const msgCost =
				(entry.price_per_1m_input / 1_000_000) * msg.tokens.input +
				(entry.price_per_1m_output / 1_000_000) * msg.tokens.output +
				(entry.price_per_1m_cache_read / 1_000_000) * msg.tokens.cache_read +
				(entry.price_per_1m_cache_write / 1_000_000) * msg.tokens.cache_write +
				(entry.price_per_1m_reasoning / 1_000_000) * msg.tokens.reasoning;

			stat.cost_usd += msgCost;
		}
	}

	// Build result
	let totalCost = 0;
	const byModel: ModelCostEntry[] = [];

	for (const [model, stat] of modelStats) {
		byModel.push({
			model,
			calls: stat.calls,
			cost_usd: stat.cost_usd,
			tokens: stat.tokens,
		});
		if (stat.cost_usd !== null) {
			totalCost += stat.cost_usd;
		}
	}

	return {
		total_cost_usd: totalCost,
		total_tokens: totalTokens,
		by_model: byModel,
	};
}

// ---------------------------------------------------------------------------
// Minimal YAML parsers (no external dependency)
// ---------------------------------------------------------------------------

/**
 * Parse .axiom/cost-pricing.yaml content into a PricingTable.
 *
 * Handles the specific YAML structure defined in spec 47 Section 9.1:
 * ```yaml
 * version: "2026-02-01"
 * models:
 *   - id: "model-name"
 *     provider: "provider"
 *     aliases: ["alias1", "alias2"]
 *     price_per_1m_input: 3.00
 *     ...
 * ```
 */
function parseCostPricingYaml(content: string): PricingTable {
	const lines = content.split("\n");
	let version = "";
	const models: PricingEntry[] = [];
	let current: Partial<PricingEntry> | null = null;

	for (const line of lines) {
		const trimmed = line.trimEnd();

		// Skip comments and empty lines
		if (trimmed === "" || trimmed.startsWith("#")) continue;

		// Top-level version
		const versionMatch = trimmed.match(/^version:\s*"?([^"]*)"?\s*$/);
		if (versionMatch) {
			version = versionMatch[1];
			continue;
		}

		// models: header
		if (/^models:\s*$/.test(trimmed)) continue;

		// New model entry (starts with "  - ")
		const newItemMatch = trimmed.match(/^\s+-\s+(\w[\w_]*):\s*"?([^"]*)"?\s*$/);
		if (newItemMatch && /^\s+-\s/.test(trimmed)) {
			// Save previous model
			if (current?.id) {
				models.push(finalizePricingEntry(current));
			}
			current = {};
			setEntryField(current, newItemMatch[1], newItemMatch[2]);
			continue;
		}

		// Continuation property (indented, no dash)
		const propMatch = trimmed.match(/^\s+(\w[\w_]*):\s*(.*)$/);
		if (propMatch && current && !/^\s+-\s/.test(trimmed)) {
			setEntryField(current, propMatch[1], propMatch[2].trim());
		}
	}

	// Save last model
	if (current?.id) {
		models.push(finalizePricingEntry(current));
	}

	return { version, models };
}

/** Set a field on a partial PricingEntry from parsed YAML key/value. */
function setEntryField(
	entry: Partial<PricingEntry>,
	key: string,
	rawValue: string,
): void {
	const value = rawValue.replace(/^"(.*)"$/, "$1").trim();

	switch (key) {
		case "id":
			entry.id = value;
			break;
		case "provider":
			entry.provider = value;
			break;
		case "aliases":
			entry.aliases = parseYamlStringArray(rawValue);
			break;
		case "price_per_1m_input":
			entry.price_per_1m_input = parseFloat(value) || 0;
			break;
		case "price_per_1m_output":
			entry.price_per_1m_output = parseFloat(value) || 0;
			break;
		case "price_per_1m_cache_read":
			entry.price_per_1m_cache_read = parseFloat(value) || 0;
			break;
		case "price_per_1m_cache_write":
			entry.price_per_1m_cache_write = parseFloat(value) || 0;
			break;
		case "price_per_1m_reasoning":
			entry.price_per_1m_reasoning = parseFloat(value) || 0;
			break;
	}
}

/** Parse a YAML inline array like ["alias1", "alias2"]. */
function parseYamlStringArray(raw: string): string[] {
	const trimmed = raw.trim();
	// Handle inline array: ["a", "b"]
	const match = trimmed.match(/^\[(.*)]\s*$/);
	if (match) {
		return match[1]
			.split(",")
			.map((s) => s.trim().replace(/^"(.*)"$/, "$1"))
			.filter((s) => s.length > 0);
	}
	// Handle empty array: []
	if (trimmed === "[]") return [];
	return [];
}

/** Fill in defaults for any missing fields on a partial PricingEntry. */
function finalizePricingEntry(partial: Partial<PricingEntry>): PricingEntry {
	return {
		id: partial.id ?? "",
		provider: partial.provider ?? "",
		aliases: partial.aliases ?? [],
		price_per_1m_input: partial.price_per_1m_input ?? 0,
		price_per_1m_output: partial.price_per_1m_output ?? 0,
		price_per_1m_cache_read: partial.price_per_1m_cache_read ?? 0,
		price_per_1m_cache_write: partial.price_per_1m_cache_write ?? 0,
		price_per_1m_reasoning: partial.price_per_1m_reasoning ?? 0,
	};
}

/** Override fields for a single model from plugin.yaml pricing_overrides. */
interface PricingOverrideFields {
	input_per_1m?: number;
	output_per_1m?: number;
	cache_read_per_1m?: number;
	cache_write_per_1m?: number;
	reasoning_per_1m?: number;
}

/**
 * Parse pricing_overrides section from plugin.yaml content.
 *
 * Expected format:
 * ```yaml
 * pricing_overrides:
 *   "model-id":
 *     input_per_1m: 5.00
 *     output_per_1m: 20.00
 *     cache_read_per_1m: 0.50
 *     cache_write_per_1m: 5.00
 *     reasoning_per_1m: 20.00
 * ```
 */
function parsePricingOverrides(
	content: string,
): Record<string, PricingOverrideFields> {
	const result: Record<string, PricingOverrideFields> = {};
	const lines = content.split("\n");

	let inOverrides = false;
	let currentModelId: string | null = null;

	for (const line of lines) {
		const trimmed = line.trimEnd();

		// Detect pricing_overrides: section
		if (/^pricing_overrides:\s*$/.test(trimmed)) {
			inOverrides = true;
			continue;
		}

		if (!inOverrides) continue;

		// Detect end of section (non-indented line that isn't empty/comment)
		if (
			trimmed !== "" &&
			!trimmed.startsWith("#") &&
			!trimmed.startsWith(" ")
		) {
			break;
		}

		// Detect model ID line (2-space indent): "  "model-id":"
		const modelMatch = trimmed.match(/^\s{2}"?([^":]+)"?:\s*$/);
		if (modelMatch && !trimmed.match(/^\s{4}/)) {
			currentModelId = modelMatch[1].trim();
			result[currentModelId] = {};
			continue;
		}

		// Detect price field (4-space indent): "    input_per_1m: 5.00"
		const fieldMatch = trimmed.match(/^\s{4,}(\w+):\s*([\d.]+)\s*$/);
		if (fieldMatch && currentModelId) {
			const key = fieldMatch[1];
			const value = parseFloat(fieldMatch[2]);
			const fields = result[currentModelId];
			if (fields) {
				switch (key) {
					case "input_per_1m":
						fields.input_per_1m = value;
						break;
					case "output_per_1m":
						fields.output_per_1m = value;
						break;
					case "cache_read_per_1m":
						fields.cache_read_per_1m = value;
						break;
					case "cache_write_per_1m":
						fields.cache_write_per_1m = value;
						break;
					case "reasoning_per_1m":
						fields.reasoning_per_1m = value;
						break;
				}
			}
		}
	}

	return result;
}
