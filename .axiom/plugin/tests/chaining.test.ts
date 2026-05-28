/**
 * Tests for L8 chaining layer (Pillar 3).
 *
 * Covers:
 * - Chain execution: steps run in sequence, context passed (REQ-FAC-042)
 * - Cycle detection: max depth exceeded (REQ-FAC-043)
 * - Fail-fast on step error (REQ-FAC-045)
 * - Structured result: name, step count, latency, per-step, final output (REQ-FAC-048)
 * - Non-chainable tool error (REQ-FAC-049)
 * - Config validation: empty name, empty steps, cycles (REQ-FAC-050)
 * - Conditional execution: step skipped when 'if' is false (REQ-FAC-051)
 * - Context resolution: $prev, $steps[N], $input.field (REQ-FAC-042)
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#6
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-040 plan=phase-100/task-100-3/step-100-3-5 jira_ref=DEX-458
 */

import { describe, expect, it } from "bun:test";
import type { ChainConfig } from "../src/config-loader";
import type { ToolExecutor } from "../src/layers/chaining/chain-executor";
import {
	DEFAULT_MAX_CHAIN_DEPTH,
	evaluateChainCondition,
	executeChain,
	resolveStepInput,
	validateAllChains,
	validateChain,
} from "../src/layers/chaining/chain-executor";

// ── Test executor ────────────────────────────────────────────────────────

/** Simple test executor that echoes args or returns a fixed value. */
function makeExecutor(
	responses: Record<string, string>,
): ToolExecutor {
	return async (toolName, args) => {
		if (toolName in responses) return responses[toolName];
		return `[${toolName}:${JSON.stringify(args)}]`;
	};
}

// ── executeChain ─────────────────────────────────────────────────────────

describe("executeChain", () => {
	it("REQ-FAC-042: executes steps in sequence and passes context", async () => {
		const chain: ChainConfig = {
			name: "test-chain",
			steps: [
				{ tool: "tool-a" },
				{ tool: "tool-b", input: { data: "$prev" } },
				{ tool: "tool-c", input: { data: "$prev" } },
			],
		};
		const executor = makeExecutor({
			"tool-a": "output-a",
			"tool-b": "output-b",
			"tool-c": "output-c",
		});
		const result = await executeChain(chain, executor);
		expect(result.chain_name).toBe("test-chain");
		expect(result.step_count).toBe(3);
		expect(result.executed_count).toBe(3);
		expect(result.final_output).toBe("output-c");
		expect(result.steps[0].tool).toBe("tool-a");
		expect(result.steps[1].tool).toBe("tool-b");
	});

	it("REQ-FAC-042: $prev resolves to previous step output", async () => {
		const capturedArgs: Record<string, unknown>[] = [];
		const executor: ToolExecutor = async (toolName, args) => {
			capturedArgs.push(args);
			return `result-${toolName}`;
		};
		const chain: ChainConfig = {
			name: "prev-chain",
			steps: [
				{ tool: "step1" },
				{ tool: "step2", input: { input_data: "$prev" } },
			],
		};
		await executeChain(chain, executor);
		expect(capturedArgs[1].input_data).toBe("result-step1");
	});

	it("REQ-FAC-042: $steps[N] resolves to step N output", async () => {
		const capturedArgs: Record<string, unknown>[] = [];
		const executor: ToolExecutor = async (toolName, args) => {
			capturedArgs.push(args);
			return `result-${toolName}`;
		};
		const chain: ChainConfig = {
			name: "steps-chain",
			steps: [
				{ tool: "step0" },
				{ tool: "step1" },
				{ tool: "step2", input: { from_step0: "$steps[0]" } },
			],
		};
		await executeChain(chain, executor);
		expect(capturedArgs[2].from_step0).toBe("result-step0");
	});

	it("REQ-FAC-042: $input.field resolves from chain input", async () => {
		const capturedArgs: Record<string, unknown>[] = [];
		const executor: ToolExecutor = async (_toolName, args) => {
			capturedArgs.push(args);
			return "ok";
		};
		const chain: ChainConfig = {
			name: "input-chain",
			steps: [{ tool: "step1", input: { query: "$input.search_term" } }],
		};
		await executeChain(chain, executor, { input: { search_term: "hello" } });
		expect(capturedArgs[0].query).toBe("hello");
	});

	it("REQ-FAC-043: throws when step count exceeds max depth", async () => {
		const steps = Array.from({ length: 11 }, (_, i) => ({ tool: `tool-${i}` }));
		const chain: ChainConfig = { name: "deep-chain", steps };
		await expect(
			executeChain(chain, makeExecutor({}), { maxDepth: 10 }),
		).rejects.toThrow("Tool chain depth exceeded");
	});

	it("REQ-FAC-045: fail-fast on step error", async () => {
		const executed: string[] = [];
		const executor: ToolExecutor = async (toolName) => {
			executed.push(toolName);
			if (toolName === "step2") throw new Error("step2 failed");
			return "ok";
		};
		const chain: ChainConfig = {
			name: "fail-chain",
			steps: [{ tool: "step1" }, { tool: "step2" }, { tool: "step3" }],
		};
		await expect(executeChain(chain, executor)).rejects.toThrow(
			'Chain "fail-chain" failed at step 1 (tool: step2): step2 failed',
		);
		// step3 must NOT have been executed
		expect(executed).not.toContain("step3");
	});

	it("REQ-FAC-048: returns structured result with all required fields", async () => {
		const chain: ChainConfig = {
			name: "structured-chain",
			steps: [{ tool: "tool-a" }, { tool: "tool-b" }],
		};
		const result = await executeChain(chain, makeExecutor({ "tool-a": "a", "tool-b": "b" }));
		expect(result).toHaveProperty("chain_name");
		expect(result).toHaveProperty("step_count");
		expect(result).toHaveProperty("executed_count");
		expect(result).toHaveProperty("total_latency_ms");
		expect(result).toHaveProperty("steps");
		expect(result).toHaveProperty("final_output");
		expect(typeof result.total_latency_ms).toBe("number");
	});

	it("REQ-FAC-049: throws for non-chainable tool", async () => {
		const chain: ChainConfig = {
			name: "blocked-chain",
			steps: [{ tool: "restricted-tool" }],
		};
		await expect(
			executeChain(chain, makeExecutor({}), {
				nonChainableTools: new Set(["restricted-tool"]),
			}),
		).rejects.toThrow("Tool 'restricted-tool' does not support chaining");
	});

	it("REQ-FAC-051: step is skipped when 'if' condition is false", async () => {
		const executed: string[] = [];
		const executor: ToolExecutor = async (toolName) => {
			executed.push(toolName);
			return "ok";
		};
		const chain: ChainConfig = {
			name: "conditional-chain",
			steps: [
				{ tool: "step1" },
				{ tool: "step2", if: "false" }, // always skip
				{ tool: "step3" },
			],
		};
		const result = await executeChain(chain, executor);
		expect(executed).not.toContain("step2");
		expect(result.steps[1].skipped).toBe(true);
		expect(result.executed_count).toBe(2);
	});

	it("REQ-FAC-051: step runs when 'if' condition is true", async () => {
		const executed: string[] = [];
		const executor: ToolExecutor = async (toolName) => {
			executed.push(toolName);
			return "output";
		};
		const chain: ChainConfig = {
			name: "cond-true-chain",
			steps: [
				{ tool: "step1" },
				{ tool: "step2", if: "$prev" }, // runs because prev is non-empty
			],
		};
		await executeChain(chain, executor);
		expect(executed).toContain("step2");
	});

	it("REQ-FAC-051: $prev.includes() condition works", async () => {
		const executed: string[] = [];
		const executor: ToolExecutor = async (toolName) => {
			executed.push(toolName);
			return toolName === "step1" ? "contains-the-word" : "done";
		};
		const chain: ChainConfig = {
			name: "includes-chain",
			steps: [
				{ tool: "step1" },
				{ tool: "step2", if: "$prev.includes('the-word')" },
				{ tool: "step3", if: "$prev.includes('missing')" }, // skipped
			],
		};
		await executeChain(chain, executor);
		expect(executed).toContain("step2");
		expect(executed).not.toContain("step3");
	});

	it("REQ-FAC-046: chain total latency is bounded (< 2x sum of step latencies + 50ms overhead)", async () => {
		// axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-046 plan=phase-100/task-100-backlog/fb-018 jira_ref=DEX-458
		const chain: ChainConfig = {
			name: "latency-chain",
			steps: [
				{ tool: "step1" },
				{ tool: "step2" },
				{ tool: "step3" },
			],
		};
		// Each step takes ~0ms (synchronous mock)
		const executor: ToolExecutor = async (toolName) => `result-${toolName}`;
		const result = await executeChain(chain, executor);

		// Sum of individual step latencies
		const sumOfSteps = result.steps.reduce((sum, s) => sum + s.latency_ms, 0);

		// Total latency must be < 2x sum of steps + 50ms overhead allowance
		// REQ-FAC-046: chain execution overhead < 2x individual tool latencies
		expect(result.total_latency_ms).toBeLessThan(2 * sumOfSteps + 50);
		expect(result.total_latency_ms).toBeGreaterThanOrEqual(0);
	});
});

// ── validateChain ────────────────────────────────────────────────────────

describe("validateChain", () => {
	it("REQ-FAC-050: valid chain passes", () => {
		const chain: ChainConfig = {
			name: "valid-chain",
			steps: [{ tool: "tool-a" }, { tool: "tool-b" }],
		};
		const result = validateChain(chain, [chain]);
		expect(result.valid).toBe(true);
		expect(result.errors).toHaveLength(0);
	});

	it("REQ-FAC-050: empty name fails", () => {
		const chain: ChainConfig = { name: "", steps: [{ tool: "tool-a" }] };
		const result = validateChain(chain, [chain]);
		expect(result.valid).toBe(false);
		expect(result.errors.some((e) => e.includes("non-empty name"))).toBe(true);
	});

	it("REQ-FAC-050: empty steps fails", () => {
		const chain: ChainConfig = { name: "empty-steps", steps: [] };
		const result = validateChain(chain, [chain]);
		expect(result.valid).toBe(false);
		expect(result.errors.some((e) => e.includes("at least one step"))).toBe(true);
	});

	it("REQ-FAC-050: step with empty tool name fails", () => {
		const chain: ChainConfig = { name: "bad-step", steps: [{ tool: "" }] };
		const result = validateChain(chain, [chain]);
		expect(result.valid).toBe(false);
		expect(result.errors.some((e) => e.includes("tool name is required"))).toBe(true);
	});

	it("REQ-FAC-043/050: self-referencing chain (cycle) fails validation", () => {
		const chain: ChainConfig = {
			name: "cyclic-chain",
			steps: [{ tool: "cyclic-chain" }], // references itself
		};
		const result = validateChain(chain, [chain]);
		expect(result.valid).toBe(false);
		expect(result.errors.some((e) => e.includes("cycle"))).toBe(true);
	});
});

describe("validateAllChains", () => {
	it("REQ-FAC-050: validates all chains and aggregates errors", () => {
		const chains: ChainConfig[] = [
			{ name: "good-chain", steps: [{ tool: "tool-a" }] },
			{ name: "", steps: [{ tool: "tool-b" }] }, // invalid
		];
		const result = validateAllChains(chains);
		expect(result.valid).toBe(false);
		expect(result.errors.length).toBeGreaterThan(0);
	});

	it("REQ-FAC-050: all valid chains returns valid=true", () => {
		const chains: ChainConfig[] = [
			{ name: "chain-a", steps: [{ tool: "tool-a" }] },
			{ name: "chain-b", steps: [{ tool: "tool-b" }] },
		];
		const result = validateAllChains(chains);
		expect(result.valid).toBe(true);
	});
});

// ── evaluateChainCondition ───────────────────────────────────────────────

describe("evaluateChainCondition", () => {
	const ctx = {
		prev: "hello world",
		steps: { 0: "step0-output", 1: "step1-output" },
		input: {},
	};

	it("REQ-FAC-051: 'true' returns true", () => {
		expect(evaluateChainCondition("true", ctx)).toBe(true);
	});

	it("REQ-FAC-051: 'false' returns false", () => {
		expect(evaluateChainCondition("false", ctx)).toBe(false);
	});

	it("REQ-FAC-051: '$prev' returns true when prev is non-empty", () => {
		expect(evaluateChainCondition("$prev", ctx)).toBe(true);
	});

	it("REQ-FAC-051: '$prev' returns false when prev is empty", () => {
		expect(evaluateChainCondition("$prev", { ...ctx, prev: "" })).toBe(false);
	});

	it("REQ-FAC-051: '$prev.includes()' matches substring", () => {
		expect(evaluateChainCondition("$prev.includes('hello')", ctx)).toBe(true);
		expect(evaluateChainCondition("$prev.includes('missing')", ctx)).toBe(false);
	});

	it("REQ-FAC-051: '$steps[N]' returns true when step N has output", () => {
		expect(evaluateChainCondition("$steps[0]", ctx)).toBe(true);
		expect(evaluateChainCondition("$steps[99]", ctx)).toBe(false);
	});
});

// ── resolveStepInput ─────────────────────────────────────────────────────

describe("resolveStepInput", () => {
	const ctx = {
		prev: "prev-output",
		steps: { 0: "step0", 1: "step1" },
		input: { query: "search-term" },
	};

	it("REQ-FAC-042: $prev resolves to previous output", () => {
		const resolved = resolveStepInput({ data: "$prev" }, ctx);
		expect(resolved.data).toBe("prev-output");
	});

	it("REQ-FAC-042: $steps[N] resolves to step N output", () => {
		const resolved = resolveStepInput({ from: "$steps[0]" }, ctx);
		expect(resolved.from).toBe("step0");
	});

	it("REQ-FAC-042: $input.field resolves from chain input", () => {
		const resolved = resolveStepInput({ q: "$input.query" }, ctx);
		expect(resolved.q).toBe("search-term");
	});

	it("REQ-FAC-042: literal values pass through unchanged", () => {
		const resolved = resolveStepInput({ mode: "strict" }, ctx);
		expect(resolved.mode).toBe("strict");
	});

	it("REQ-FAC-042: undefined inputSpec returns empty object", () => {
		expect(resolveStepInput(undefined, ctx)).toEqual({});
	});
});
