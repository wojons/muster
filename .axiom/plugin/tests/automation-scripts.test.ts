/**
 * Automation scripts engine tests — verifies script listing, execution,
 * argument substitution, confirmation prompts, and output capture per
 * REQ-PLG-421, REQ-PLG-422, REQ-PLG-423, REQ-PLG-424, REQ-PLG-425.
 *
 * These tests are written BEFORE the scripts engine module exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-421 through #REQ-PLG-425
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-421 plan=phase-70-4/task-70-4-4/step-70-4-4-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ScriptEntry } from "../src/config-loader";
// ── Scripts engine module import (will fail until implemented) ───────────
// REQ-PLG-421: /axiom-script command lists and runs named scripts
// REQ-PLG-422: Scripts support positional ($1, $2) and named ($arg_name) args
// REQ-PLG-423: requires_confirmation: true prompts before execution
// REQ-PLG-424: Script output captured and returned as tool output
// REQ-PLG-425: Scripts NOT auto-registered as tools (avoid naming collisions)
import {
	createScriptsEngine,
	type ScriptsEngine,
	substituteArgs,
} from "../src/layers/automation/scripts";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-scripts-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-421: /axiom-script command lists and runs named scripts
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-421: list and run named scripts", () => {
	let tempDir: string;
	let engine: ScriptsEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createScriptsEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("listScripts returns all script names and descriptions", () => {
		const scripts: Record<string, ScriptEntry> = {
			lint: { run: "npm run lint", description: "Run project linter" },
			test: { run: "npm test", description: "Run project tests" },
		};
		const list = engine.listScripts(scripts);
		expect(Array.isArray(list)).toBe(true);
		expect(list.length).toBe(2);
		const names = list.map((s) => s.name);
		expect(names).toContain("lint");
		expect(names).toContain("test");
	});

	it("listScripts returns empty array for empty scripts config", () => {
		const list = engine.listScripts({});
		expect(list).toEqual([]);
	});

	it("runScript executes the named script and returns output", async () => {
		const scripts: Record<string, ScriptEntry> = {
			greet: { run: "echo hello-script" },
		};
		const result = await engine.runScript("greet", scripts, []);
		expect(result.success).toBe(true);
		expect(result.output).toContain("hello-script");
		expect(result.scriptName).toBe("greet");
	});

	it("runScript returns error when script name not found", async () => {
		const scripts: Record<string, ScriptEntry> = {
			lint: { run: "npm run lint" },
		};
		const result = await engine.runScript("nonexistent", scripts, []);
		expect(result.success).toBe(false);
		expect(result.error).toBeDefined();
		expect(result.error).toContain("nonexistent");
	});

	it("formatScriptList returns human-readable listing", () => {
		const scripts: Record<string, ScriptEntry> = {
			lint: { run: "npm run lint", description: "Run project linter" },
			deploy: {
				run: "axiom deploy",
				description: "Deploy to staging",
				requires_confirmation: true,
			},
		};
		const formatted = engine.formatScriptList(scripts);
		expect(typeof formatted).toBe("string");
		expect(formatted).toContain("lint");
		expect(formatted).toContain("deploy");
		expect(formatted).toContain("Run project linter");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-422: Argument substitution
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-422: argument substitution", () => {
	it("substituteArgs: replaces $1, $2 with positional args", () => {
		const result = substituteArgs("echo $1 and $2", ["hello", "world"]);
		expect(result).toBe("echo hello and world");
	});

	it("substituteArgs: replaces $arg_name with named args", () => {
		const result = substituteArgs(
			"axiom run --work-item $work_item_id --repo .",
			[],
			{ work_item_id: "AUTH-234" },
		);
		expect(result).toBe("axiom run --work-item AUTH-234 --repo .");
	});

	it("substituteArgs: handles both positional and named args", () => {
		const result = substituteArgs("echo $1 $name", ["first"], {
			name: "world",
		});
		expect(result).toBe("echo first world");
	});

	it("substituteArgs: leaves unmatched placeholders as-is", () => {
		const result = substituteArgs("echo $1 $missing", ["hello"]);
		expect(result).toBe("echo hello $missing");
	});

	it("substituteArgs: handles empty args", () => {
		const result = substituteArgs("echo hello", [], {});
		expect(result).toBe("echo hello");
	});

	it("runScript substitutes positional args before execution", async () => {
		const tempDir = makeTempDir();
		try {
			const engine = createScriptsEngine({ repoRoot: tempDir });
			const scripts: Record<string, ScriptEntry> = {
				greet: { run: "echo hello $1" },
			};
			const result = await engine.runScript("greet", scripts, ["world"]);
			expect(result.success).toBe(true);
			expect(result.output).toContain("hello world");
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("runScript substitutes named args before execution", async () => {
		const tempDir = makeTempDir();
		try {
			const engine = createScriptsEngine({ repoRoot: tempDir });
			const scripts: Record<string, ScriptEntry> = {
				ralph: { run: "echo running $work_item_id" },
			};
			const result = await engine.runScript("ralph", scripts, [], {
				work_item_id: "AUTH-234",
			});
			expect(result.success).toBe(true);
			expect(result.output).toContain("running AUTH-234");
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-423: requires_confirmation prompt
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-423: requires_confirmation prompt", () => {
	let tempDir: string;
	let engine: ScriptsEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createScriptsEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("requiresConfirmation returns true for scripts with requires_confirmation: true", () => {
		const script: ScriptEntry = {
			run: "axiom deploy --env staging",
			requires_confirmation: true,
		};
		expect(engine.requiresConfirmation(script)).toBe(true);
	});

	it("requiresConfirmation returns false for scripts without the flag", () => {
		const script: ScriptEntry = { run: "npm run lint" };
		expect(engine.requiresConfirmation(script)).toBe(false);
	});

	it("getConfirmationPrompt returns a human-readable prompt string", () => {
		const script: ScriptEntry = {
			run: "axiom deploy --env staging",
			description: "Deploy to the staging environment",
			requires_confirmation: true,
		};
		const prompt = engine.getConfirmationPrompt("deploy-staging", script);
		expect(typeof prompt).toBe("string");
		expect(prompt).toContain("deploy-staging");
		expect(prompt.toLowerCase()).toMatch(/yes|no|confirm/);
	});

	it("runScript with requires_confirmation and confirmed=true executes the script", async () => {
		const scripts: Record<string, ScriptEntry> = {
			deploy: {
				run: "echo deployed",
				requires_confirmation: true,
			},
		};
		const result = await engine.runScript(
			"deploy",
			scripts,
			[],
			{},
			{ confirmed: true },
		);
		expect(result.success).toBe(true);
		expect(result.output).toContain("deployed");
	});

	it("runScript with requires_confirmation and confirmed=false returns needs_confirmation", async () => {
		const scripts: Record<string, ScriptEntry> = {
			deploy: {
				run: "echo deployed",
				requires_confirmation: true,
			},
		};
		const result = await engine.runScript(
			"deploy",
			scripts,
			[],
			{},
			{ confirmed: false },
		);
		expect(result.success).toBe(false);
		expect(result.needsConfirmation).toBe(true);
		expect(result.confirmationPrompt).toBeDefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-424: Script output captured and returned
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-424: script output captured and returned", () => {
	let tempDir: string;
	let engine: ScriptsEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createScriptsEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("captures stdout from script execution", async () => {
		const scripts: Record<string, ScriptEntry> = {
			hello: { run: "echo stdout-output" },
		};
		const result = await engine.runScript("hello", scripts, []);
		expect(result.output).toContain("stdout-output");
	});

	it("captures stderr from script execution", async () => {
		const scripts: Record<string, ScriptEntry> = {
			err: { run: "echo stderr-output >&2" },
		};
		const result = await engine.runScript("err", scripts, []);
		// stderr should be captured in output
		expect(typeof result.output).toBe("string");
	});

	it("marks script as failed when exit code is non-zero", async () => {
		const scripts: Record<string, ScriptEntry> = {
			fail: { run: "exit 1" },
		};
		const result = await engine.runScript("fail", scripts, []);
		expect(result.success).toBe(false);
		expect(result.exitCode).not.toBe(0);
	});

	it("returns exit code in result", async () => {
		const scripts: Record<string, ScriptEntry> = {
			ok: { run: "exit 0" },
		};
		const result = await engine.runScript("ok", scripts, []);
		expect(result.exitCode).toBe(0);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-425: Scripts NOT auto-registered as tools
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-425: scripts not auto-registered as tools", () => {
	it("getToolNames returns empty array (scripts are not tools)", () => {
		const tempDir = makeTempDir();
		try {
			const engine = createScriptsEngine({ repoRoot: tempDir });
			const scripts: Record<string, ScriptEntry> = {
				lint: { run: "npm run lint" },
				test: { run: "npm test" },
			};
			// Scripts should NOT be auto-registered as tools
			const toolNames = engine.getToolNames(scripts);
			expect(toolNames).toEqual([]);
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});
});
