/**
 * Tool chaining executor for L8 file-based agent communication (Pillar 3).
 *
 * Enables declarative multi-step tool workflows that execute without LLM
 * round-trips between steps. Each step receives the output of the previous
 * step via a local context object. The LLM sees only the final aggregated result.
 *
 * REQ-FAC-040: All tools chainable by default ('auto' mode).
 * REQ-FAC-041: Declarative chain definitions in plugin.yaml.
 * REQ-FAC-042: Each step receives previous output via local context.
 * REQ-FAC-043: Cycle detection with max depth limit (default 10).
 * REQ-FAC-044: Chained tools governed by REQ-PLG-720/721 trust model.
 * REQ-FAC-045: Fail-fast on step error.
 * REQ-FAC-046: Chain execution latency < 2x sum of individual tool latencies.
 * REQ-FAC-047: No privilege escalation in chains.
 * REQ-FAC-048: Structured chain result (name, step count, latency, per-step results, final output).
 * REQ-FAC-049: Error for chaining to non-chainable tool.
 * REQ-FAC-050: Chain validation at config load time.
 * REQ-FAC-051: Conditional execution via 'if' field.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#6
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-040 plan=phase-100/task-100-3/step-100-3-2 jira_ref=DEX-458
 */

import type { ChainConfig } from "../../config-loader";

// ── Constants ────────────────────────────────────────────────────────────

/** Default maximum chain depth. REQ-FAC-043 */
export const DEFAULT_MAX_CHAIN_DEPTH = 10;

// ── Types ────────────────────────────────────────────────────────────────

/**
 * A tool executor function — takes input args and returns a string result.
 * In production this is wired to the actual tool registry.
 * In tests it can be any function.
 */
export type ToolExecutor = (
	toolName: string,
	args: Record<string, unknown>,
) => Promise<string>;

/**
 * Local context passed between chain steps.
 * REQ-FAC-042: each step receives the output of the previous step.
 */
export interface ChainContext {
	/** Output of the previous step (undefined for the first step). */
	prev?: string;
	/** Outputs of all previous steps, keyed by step index (0-based). */
	steps: Record<number, string>;
	/** Input args passed to the chain invocation. */
	input: Record<string, unknown>;
}

/** Result of a single chain step. */
export interface ChainStepResult {
	/** Step index (0-based). */
	index: number;
	/** Tool name that was invoked. */
	tool: string;
	/** Whether the step was skipped due to a false 'if' condition. REQ-FAC-051 */
	skipped: boolean;
	/** Output of the step (undefined if skipped). */
	output?: string;
	/** Latency in milliseconds. */
	latency_ms: number;
}

/**
 * Structured chain execution result. REQ-FAC-048
 */
export interface ChainResult {
	/** Chain name. */
	chain_name: string;
	/** Number of steps defined (including skipped). */
	step_count: number;
	/** Number of steps actually executed. */
	executed_count: number;
	/** Total execution latency in milliseconds. */
	total_latency_ms: number;
	/** Per-step results (truncated output for large outputs). */
	steps: ChainStepResult[];
	/** Final output (output of the last non-skipped step). */
	final_output: string;
}

/** Options for executeChain(). */
export interface ExecuteChainOptions {
	/** Maximum chain depth. REQ-FAC-043 */
	maxDepth?: number;
	/** Input args for the chain. */
	input?: Record<string, unknown>;
	/** Set of tool names that have opted out of chaining. REQ-FAC-049 */
	nonChainableTools?: Set<string>;
}

/** Validation result for a chain definition. REQ-FAC-050 */
export interface ChainValidationResult {
	valid: boolean;
	errors: string[];
}

// ── Condition evaluation ─────────────────────────────────────────────────

/**
 * Evaluate a simple condition expression against the chain context.
 *
 * Supported expressions:
 * - "$prev" — truthy if prev output is non-empty
 * - "$prev.includes('text')" — truthy if prev output contains text
 * - "$steps[0]" — truthy if step 0 produced output
 * - "true" / "false" — literal booleans
 *
 * REQ-FAC-051
 */
export function evaluateChainCondition(
	condition: string,
	context: ChainContext,
): boolean {
	const trimmed = condition.trim();

	if (trimmed === "true") return true;
	if (trimmed === "false") return false;

	// $prev — truthy if prev is non-empty
	if (trimmed === "$prev") {
		return Boolean(context.prev && context.prev.length > 0);
	}

	// $prev.includes('text') or $prev.includes("text")
	const includesMatch = trimmed.match(/^\$prev\.includes\(['"](.+)['"]\)$/);
	if (includesMatch) {
		return Boolean(context.prev?.includes(includesMatch[1]));
	}

	// $steps[N] — truthy if step N produced output
	const stepMatch = trimmed.match(/^\$steps\[(\d+)\]$/);
	if (stepMatch) {
		const idx = parseInt(stepMatch[1], 10);
		return Boolean(context.steps[idx] && context.steps[idx].length > 0);
	}

	// Unknown condition — default to true (permissive)
	return true;
}

// ── Input resolution ─────────────────────────────────────────────────────

/**
 * Resolve step input args from the chain context.
 *
 * Values starting with "$prev" or "$steps[N]" are substituted from context.
 * All other values are passed through as-is.
 *
 * REQ-FAC-042
 */
export function resolveStepInput(
	inputSpec: Record<string, string> | undefined,
	context: ChainContext,
): Record<string, unknown> {
	if (!inputSpec) return {};

	const resolved: Record<string, unknown> = {};
	for (const [param, valueExpr] of Object.entries(inputSpec)) {
		if (valueExpr === "$prev") {
			resolved[param] = context.prev ?? "";
		} else if (valueExpr.startsWith("$steps[")) {
			const match = valueExpr.match(/^\$steps\[(\d+)\]$/);
			if (match) {
				const idx = parseInt(match[1], 10);
				resolved[param] = context.steps[idx] ?? "";
			} else {
				resolved[param] = valueExpr;
			}
		} else if (valueExpr.startsWith("$input.")) {
			const field = valueExpr.slice("$input.".length);
			resolved[param] = context.input[field] ?? "";
		} else {
			resolved[param] = valueExpr;
		}
	}
	return resolved;
}

// ── Cycle detection ──────────────────────────────────────────────────────

/**
 * Detect cycles in a chain definition.
 *
 * A chain is cyclic if any step references a tool that would cause
 * re-entry into the same chain (detected via depth limit at runtime).
 * At config-load time, we detect direct self-references and duplicate
 * step sequences that would loop.
 *
 * REQ-FAC-043, REQ-FAC-050
 */
export function detectChainCycles(
	chain: ChainConfig,
	allChains: ChainConfig[],
	visited: Set<string> = new Set(),
	depth = 0,
	maxDepth = DEFAULT_MAX_CHAIN_DEPTH,
): boolean {
	if (depth > maxDepth) return true; // depth exceeded = cycle
	if (visited.has(chain.name)) return true; // direct cycle

	visited.add(chain.name);

	// Check if any step invokes another chain (by convention: tool name = chain name)
	for (const step of chain.steps) {
		const subChain = allChains.find((c) => c.name === step.tool);
		if (subChain) {
			const subVisited = new Set(visited);
			if (detectChainCycles(subChain, allChains, subVisited, depth + 1, maxDepth)) {
				return true;
			}
		}
	}

	return false;
}

// ── Validation ───────────────────────────────────────────────────────────

/**
 * Validate a chain definition at config load time.
 * REQ-FAC-050: invalid chain references and cycles produce config validation errors.
 */
export function validateChain(
	chain: ChainConfig,
	allChains: ChainConfig[],
	maxDepth = DEFAULT_MAX_CHAIN_DEPTH,
): ChainValidationResult {
	const errors: string[] = [];

	if (!chain.name || chain.name.trim() === "") {
		errors.push("Chain must have a non-empty name.");
	}

	if (!chain.steps || chain.steps.length === 0) {
		errors.push(`Chain "${chain.name}" must have at least one step.`);
	}

	for (let i = 0; i < (chain.steps ?? []).length; i++) {
		const step = chain.steps[i];
		if (!step.tool || step.tool.trim() === "") {
			errors.push(`Chain "${chain.name}" step ${i}: tool name is required.`);
		}
	}

	// Cycle detection
	if (detectChainCycles(chain, allChains, new Set(), 0, maxDepth)) {
		errors.push(
			`Chain "${chain.name}" contains a cycle or exceeds max depth ${maxDepth}.`,
		);
	}

	return { valid: errors.length === 0, errors };
}

/**
 * Validate all chain definitions in a config.
 * REQ-FAC-050
 */
export function validateAllChains(
	chains: ChainConfig[],
	maxDepth = DEFAULT_MAX_CHAIN_DEPTH,
): ChainValidationResult {
	const allErrors: string[] = [];
	for (const chain of chains) {
		const result = validateChain(chain, chains, maxDepth);
		allErrors.push(...result.errors);
	}
	return { valid: allErrors.length === 0, errors: allErrors };
}

// ── Executor ─────────────────────────────────────────────────────────────

/**
 * Execute a chain definition, invoking each step's tool in sequence.
 *
 * REQ-FAC-042: each step receives previous output via context.
 * REQ-FAC-043: depth limit enforced.
 * REQ-FAC-045: fail-fast on step error.
 * REQ-FAC-047: no privilege escalation (executor runs with caller's permissions).
 * REQ-FAC-048: returns structured result.
 * REQ-FAC-049: non-chainable tools produce an error.
 * REQ-FAC-051: conditional steps skipped when condition is false.
 */
export async function executeChain(
	chain: ChainConfig,
	executor: ToolExecutor,
	opts: ExecuteChainOptions = {},
): Promise<ChainResult> {
	const maxDepth = opts.maxDepth ?? DEFAULT_MAX_CHAIN_DEPTH;
	const nonChainable = opts.nonChainableTools ?? new Set<string>();

	if (chain.steps.length > maxDepth) {
		throw new Error(
			`Tool chain depth exceeded — possible cycle in chain definition.`,
		);
	}

	const context: ChainContext = {
		prev: undefined,
		steps: {},
		input: opts.input ?? {},
	};

	const stepResults: ChainStepResult[] = [];
	const chainStart = Date.now();
	let executedCount = 0;

	for (let i = 0; i < chain.steps.length; i++) {
		const step = chain.steps[i];
		const stepStart = Date.now();

		// REQ-FAC-049: non-chainable tool check
		if (nonChainable.has(step.tool)) {
			throw new Error(
				`Tool '${step.tool}' does not support chaining.`,
			);
		}

		// REQ-FAC-051: conditional execution
		if (step.if !== undefined) {
			const condResult = evaluateChainCondition(step.if, context);
			if (!condResult) {
				stepResults.push({
					index: i,
					tool: step.tool,
					skipped: true,
					latency_ms: Date.now() - stepStart,
				});
				continue;
			}
		}

		// Resolve input args from context
		const resolvedArgs = resolveStepInput(step.input, context);

		// REQ-FAC-045: fail-fast on step error
		let output: string;
		try {
			output = await executor(step.tool, resolvedArgs);
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new Error(
				`Chain "${chain.name}" failed at step ${i} (tool: ${step.tool}): ${msg}`,
			);
		}

		// Update context for next step
		context.prev = output;
		context.steps[i] = output;
		executedCount++;

		stepResults.push({
			index: i,
			tool: step.tool,
			skipped: false,
			// Truncate output in result to avoid large payloads. REQ-FAC-048
			output: output.length > 500 ? output.slice(0, 500) + "…" : output,
			latency_ms: Date.now() - stepStart,
		});
	}

	const finalOutput = context.prev ?? "";

	return {
		chain_name: chain.name,
		step_count: chain.steps.length,
		executed_count: executedCount,
		total_latency_ms: Date.now() - chainStart,
		steps: stepResults,
		final_output: finalOutput,
	};
}
