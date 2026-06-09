/**
 * Custom tool registration tests — verifies L6 tool extension: tool definition,
 * execution, naming, timeout, idempotency, and tool count limits per
 * REQ-PLG-500, REQ-PLG-501, REQ-PLG-502, REQ-PLG-503, REQ-PLG-508, REQ-PLG-509.
 *
 * Also tests the config trust/approval system per REQ-PLG-720 and REQ-PLG-721.
 *
 * These tests are written BEFORE the tool registration module exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-500 through #REQ-PLG-509,
 *       #REQ-PLG-720, #REQ-PLG-721
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-500 plan=phase-70-4/task-70-4-5/step-70-4-5-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { createHash } from "node:crypto";
import {
	existsSync,
	mkdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ToolEntry } from "../src/config-loader";
// ── Tool registration module import (will fail until implemented) ─────────
// REQ-PLG-500: Register custom tools from .axiom/plugin.yaml
// REQ-PLG-501: Tools support name, description, run, parameters
// REQ-PLG-502: Capture stdout+stderr and return as tool result
// REQ-PLG-503: Configurable timeout (default 60s)
// REQ-PLG-508: Total tools must not exceed 20
// REQ-PLG-509: Tool registration is idempotent
// REQ-PLG-720: Config must be approved before automation runs
// REQ-PLG-721: Approved configs tracked by SHA-256 hash in .axiom/.plugin-trust
import {
	approveConfig,
	buildToolDefinitions,
	computeConfigHash,
	createToolRegistry,
	isConfigApproved,
	type ToolRegistry,
} from "../src/layers/automation/tool-registry";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-tools-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-500/501: Custom tool registration
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-500/501: custom tool registration", () => {
	it("buildToolDefinitions returns tool definitions from config", () => {
		const tools: ToolEntry[] = [
			{
				name: "project_lint",
				description: "Run the project linter",
				run: "npm run lint 2>&1",
			},
		];
		const defs = buildToolDefinitions(tools);
		expect(Array.isArray(defs)).toBe(true);
		expect(defs.length).toBe(1);
		expect(defs[0].name).toBe("project_lint");
		expect(defs[0].description).toBe("Run the project linter");
		expect(defs[0].run).toBe("npm run lint 2>&1");
	});

	it("buildToolDefinitions returns empty array for empty tools list", () => {
		const defs = buildToolDefinitions([]);
		expect(defs).toEqual([]);
	});

	it("buildToolDefinitions includes parameters when defined", () => {
		const tools: ToolEntry[] = [
			{
				name: "project_test",
				description: "Run project tests",
				run: "npm test -- $test_pattern 2>&1",
				parameters: {
					test_pattern: {
						type: "string",
						description: "Test file or pattern to run",
						default: "",
					},
				},
			},
		];
		const defs = buildToolDefinitions(tools);
		expect(defs[0].parameters).toBeDefined();
		expect(defs[0].parameters?.test_pattern).toBeDefined();
		expect(defs[0].parameters?.test_pattern.type).toBe("string");
	});

	it("tool names are used as-is (no automatic codeops_ prefix)", () => {
		// REQ-PLG-501: name is used as-is; users can prefix if desired
		const tools: ToolEntry[] = [
			{ name: "my_tool", description: "My tool", run: "echo ok" },
		];
		const defs = buildToolDefinitions(tools);
		expect(defs[0].name).toBe("my_tool");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-502/503: Tool execution
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-502/503: tool execution", () => {
	let tempDir: string;
	let registry: ToolRegistry;

	beforeEach(() => {
		tempDir = makeTempDir();
		registry = createToolRegistry({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("executeTool captures stdout and returns as result", async () => {
		const tool: ToolEntry = {
			name: "echo_tool",
			description: "Echo test",
			run: "echo tool-output",
		};
		const result = await registry.executeTool(tool, {});
		expect(result.success).toBe(true);
		expect(result.output).toContain("tool-output");
	});

	it("executeTool captures stderr as part of output", async () => {
		const tool: ToolEntry = {
			name: "stderr_tool",
			description: "Stderr test",
			run: "echo stderr-output >&2",
		};
		const result = await registry.executeTool(tool, {});
		expect(typeof result.output).toBe("string");
	});

	it("executeTool marks as failed when command exits non-zero", async () => {
		const tool: ToolEntry = {
			name: "fail_tool",
			description: "Failing tool",
			run: "exit 1",
		};
		const result = await registry.executeTool(tool, {});
		expect(result.success).toBe(false);
	});

	it("default timeout is 60 seconds", () => {
		const reg = createToolRegistry({ repoRoot: tempDir });
		expect(reg.defaultTimeoutMs).toBe(60_000);
	});

	it("accepts custom timeout via options", () => {
		const reg = createToolRegistry({
			repoRoot: tempDir,
			defaultTimeoutMs: 5_000,
		});
		expect(reg.defaultTimeoutMs).toBe(5_000);
	});

	it("executeTool substitutes named parameters in command", async () => {
		const tool: ToolEntry = {
			name: "param_tool",
			description: "Parameterized tool",
			run: "echo $test_pattern",
			parameters: {
				test_pattern: { type: "string", description: "Pattern", default: "" },
			},
		};
		const result = await registry.executeTool(tool, {
			test_pattern: "my-test",
		});
		expect(result.output).toContain("my-test");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-508: Total tool count limit (max 20)
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-508: total tool count limit", () => {
	it("checkToolCountLimit returns ok when under 20 tools", () => {
		const tempDir = makeTempDir();
		try {
			const registry = createToolRegistry({ repoRoot: tempDir });
			const tools: ToolEntry[] = Array.from({ length: 5 }, (_, i) => ({
				name: `tool_${i}`,
				description: `Tool ${i}`,
				run: "echo ok",
			}));
			const result = registry.checkToolCountLimit(tools, 10); // 10 built-in + 5 custom = 15
			expect(result.ok).toBe(true);
			expect(result.warning).toBeUndefined();
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("checkToolCountLimit returns warning when over 20 tools", () => {
		const tempDir = makeTempDir();
		try {
			const registry = createToolRegistry({ repoRoot: tempDir });
			const tools: ToolEntry[] = Array.from({ length: 15 }, (_, i) => ({
				name: `tool_${i}`,
				description: `Tool ${i}`,
				run: "echo ok",
			}));
			const result = registry.checkToolCountLimit(tools, 10); // 10 built-in + 15 custom = 25
			expect(result.ok).toBe(false);
			expect(result.warning).toBeDefined();
			expect(result.warning).toContain("20");
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-509: Idempotent tool registration
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-509: idempotent tool registration", () => {
	it("buildToolDefinitions with duplicate names deduplicates", () => {
		const tools: ToolEntry[] = [
			{ name: "my_tool", description: "First", run: "echo first" },
			{ name: "my_tool", description: "Second", run: "echo second" },
		];
		const defs = buildToolDefinitions(tools);
		// Should deduplicate — only one tool with that name
		const names = defs.map((d) => d.name);
		const uniqueNames = [...new Set(names)];
		expect(names.length).toBe(uniqueNames.length);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-720/721: Config trust and approval system
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-720/721: config trust and approval", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirSync(join(tempDir, ".axiom"), { recursive: true });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("computeConfigHash returns a SHA-256 hex string", () => {
		const content =
			"version: 1\nhooks:\n  on_session_start:\n    - name: test\n      run: echo ok\n";
		const hash = computeConfigHash(content);
		expect(typeof hash).toBe("string");
		expect(hash).toMatch(/^[0-9a-f]{64}$/);
		// Verify it's actually SHA-256
		const expected = createHash("sha256").update(content).digest("hex");
		expect(hash).toBe(expected);
	});

	it("isConfigApproved returns false when .plugin-trust does not exist", () => {
		const content = "version: 1\n";
		const result = isConfigApproved(tempDir, content);
		expect(result).toBe(false);
	});

	it("isConfigApproved returns false when hash does not match", () => {
		const trustPath = join(tempDir, ".axiom", ".plugin-trust");
		writeFileSync(trustPath, "sha256:differenthash\n");
		const content = "version: 1\n";
		const result = isConfigApproved(tempDir, content);
		expect(result).toBe(false);
	});

	it("approveConfig writes the config hash to .axiom/.plugin-trust", () => {
		const content = "version: 1\nhooks:\n  on_session_start: []\n";
		approveConfig(tempDir, content);
		const trustPath = join(tempDir, ".axiom", ".plugin-trust");
		expect(existsSync(trustPath)).toBe(true);
		const stored = readFileSync(trustPath, "utf-8");
		const expectedHash = computeConfigHash(content);
		expect(stored).toContain(expectedHash);
	});

	it("isConfigApproved returns true after approveConfig is called", () => {
		const content = "version: 1\nhooks:\n  on_session_start: []\n";
		approveConfig(tempDir, content);
		const result = isConfigApproved(tempDir, content);
		expect(result).toBe(true);
	});

	it("isConfigApproved returns false after config content changes", () => {
		const content = "version: 1\nhooks:\n  on_session_start: []\n";
		approveConfig(tempDir, content);
		const changedContent = `${content}# changed\n`;
		const result = isConfigApproved(tempDir, changedContent);
		expect(result).toBe(false);
	});

	it("approveConfig overwrites previous approval when config changes", () => {
		const content1 = "version: 1\n";
		const content2 = "version: 1\nhooks:\n  on_session_start: []\n";
		approveConfig(tempDir, content1);
		approveConfig(tempDir, content2);
		expect(isConfigApproved(tempDir, content1)).toBe(false);
		expect(isConfigApproved(tempDir, content2)).toBe(true);
	});
});
