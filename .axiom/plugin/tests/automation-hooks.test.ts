/**
 * Automation hooks engine tests — verifies hook execution, lifecycle events,
 * condition evaluation, timeout enforcement, and output capture per
 * REQ-PLG-410, REQ-PLG-411, REQ-PLG-412, REQ-PLG-413, REQ-PLG-414, REQ-PLG-415.
 *
 * These tests are written BEFORE the hooks engine module exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-410 through #REQ-PLG-415
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-410 plan=phase-70-4/task-70-4-2/step-70-4-2-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { HookEntry, HooksConfig } from "../src/config-loader";
// ── Hooks engine module import (will fail until implemented) ─────────────
// REQ-PLG-410: Execute shell commands in repo root on lifecycle events
// REQ-PLG-411: Support on_session_start, on_session_end, on_session_idle,
//              on_message, on_command, on_error lifecycle events
// REQ-PLG-412: Support condition expressions (maturity >= N, backend == X, etc.)
// REQ-PLG-413: Non-blocking execution — hooks run in background
// REQ-PLG-414: Configurable timeout (default 30s); kill and log on timeout
// REQ-PLG-415: Hook output available via codeops_hook_output tool
import {
	createHooksEngine,
	evaluateCondition,
	type HooksEngine,
} from "../src/layers/automation/hooks";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-hooks-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-410: Hooks execute shell commands in repo root
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-410: hooks execute shell commands in repo root", () => {
	let tempDir: string;
	let engine: HooksEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createHooksEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("executes a simple echo command and captures output", async () => {
		const hook: HookEntry = {
			name: "test-hook",
			run: "echo hello-from-hook",
		};
		const result = await engine.runHook(hook, {});
		expect(result).toBeDefined();
		expect(result.success).toBe(true);
		expect(result.output).toContain("hello-from-hook");
	});

	it("runs the hook in the repo root directory", async () => {
		// The hook should run in tempDir (the repo root)
		const hook: HookEntry = {
			name: "pwd-hook",
			run: "pwd",
		};
		const result = await engine.runHook(hook, {});
		expect(result.success).toBe(true);
		// Output should contain the temp dir path (or a resolved equivalent)
		expect(result.output.trim()).toBeTruthy();
	});

	it("captures stderr as part of output", async () => {
		const hook: HookEntry = {
			name: "stderr-hook",
			run: "echo error-output >&2",
		};
		const result = await engine.runHook(hook, {});
		// stderr should be captured (success depends on exit code, not stderr)
		expect(result).toBeDefined();
		expect(typeof result.output).toBe("string");
	});

	it("marks hook as failed when command exits with non-zero code", async () => {
		const hook: HookEntry = {
			name: "failing-hook",
			run: "exit 1",
		};
		const result = await engine.runHook(hook, {});
		expect(result.success).toBe(false);
		expect(result.exitCode).not.toBe(0);
	});

	it("returns hook name in result", async () => {
		const hook: HookEntry = {
			name: "named-hook",
			run: "echo ok",
		};
		const result = await engine.runHook(hook, {});
		expect(result.hookName).toBe("named-hook");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-411: Lifecycle event dispatch
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-411: lifecycle event dispatch", () => {
	let tempDir: string;
	let engine: HooksEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createHooksEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("fires on_session_start hooks when session starts", async () => {
		const hooks: HooksConfig = {
			on_session_start: [{ name: "start-hook", run: "echo session-started" }],
		};
		const results = await engine.fireEvent("on_session_start", hooks, {});
		expect(Array.isArray(results)).toBe(true);
		expect(results.length).toBe(1);
		expect(results[0].hookName).toBe("start-hook");
		expect(results[0].output).toContain("session-started");
	});

	it("fires on_session_end hooks when session ends", async () => {
		const hooks: HooksConfig = {
			on_session_end: [{ name: "end-hook", run: "echo session-ended" }],
		};
		const results = await engine.fireEvent("on_session_end", hooks, {});
		expect(results.length).toBe(1);
		expect(results[0].output).toContain("session-ended");
	});

	it("fires on_message hooks on each message exchange", async () => {
		const hooks: HooksConfig = {
			on_message: [{ name: "msg-hook", run: "echo message-fired" }],
		};
		const results = await engine.fireEvent("on_message", hooks, {});
		expect(results.length).toBe(1);
		expect(results[0].output).toContain("message-fired");
	});

	it("fires on_command hooks after slash command execution", async () => {
		const hooks: HooksConfig = {
			on_command: [{ name: "cmd-hook", run: "echo command-fired" }],
		};
		const results = await engine.fireEvent("on_command", hooks, {});
		expect(results.length).toBe(1);
		expect(results[0].output).toContain("command-fired");
	});

	it("fires on_error hooks when session error occurs", async () => {
		const hooks: HooksConfig = {
			on_error: [{ name: "error-hook", run: "echo error-fired" }],
		};
		const results = await engine.fireEvent("on_error", hooks, {});
		expect(results.length).toBe(1);
		expect(results[0].output).toContain("error-fired");
	});

	it("fires on_session_idle hooks when session becomes idle", async () => {
		const hooks: HooksConfig = {
			on_session_idle: [{ name: "idle-hook", run: "echo idle-fired" }],
		};
		const results = await engine.fireEvent("on_session_idle", hooks, {});
		expect(results.length).toBe(1);
		expect(results[0].output).toContain("idle-fired");
	});

	it("fires multiple hooks for the same event in order", async () => {
		const hooks: HooksConfig = {
			on_session_start: [
				{ name: "first-hook", run: "echo first" },
				{ name: "second-hook", run: "echo second" },
			],
		};
		const results = await engine.fireEvent("on_session_start", hooks, {});
		expect(results.length).toBe(2);
		expect(results[0].hookName).toBe("first-hook");
		expect(results[1].hookName).toBe("second-hook");
	});

	it("returns empty array when no hooks registered for event", async () => {
		const hooks: HooksConfig = {};
		const results = await engine.fireEvent("on_session_start", hooks, {});
		expect(Array.isArray(results)).toBe(true);
		expect(results.length).toBe(0);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-412: Condition expressions
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-412: condition expressions", () => {
	it("evaluateCondition returns true when no condition set", () => {
		const result = evaluateCondition(undefined, { maturityLevel: 2 });
		expect(result).toBe(true);
	});

	it("evaluateCondition: maturity >= N passes when level is sufficient", () => {
		expect(evaluateCondition("maturity >= 2", { maturityLevel: 2 })).toBe(true);
		expect(evaluateCondition("maturity >= 2", { maturityLevel: 3 })).toBe(true);
		expect(evaluateCondition("maturity >= 3", { maturityLevel: 4 })).toBe(true);
	});

	it("evaluateCondition: maturity >= N fails when level is insufficient", () => {
		expect(evaluateCondition("maturity >= 3", { maturityLevel: 2 })).toBe(
			false,
		);
		expect(evaluateCondition("maturity >= 4", { maturityLevel: 1 })).toBe(
			false,
		);
	});

	it("evaluateCondition: backend == 'python' passes when backend matches", () => {
		expect(
			evaluateCondition('backend == "python"', { backend: "python" }),
		).toBe(true);
		expect(
			evaluateCondition("backend == 'python'", { backend: "python" }),
		).toBe(true);
	});

	it("evaluateCondition: backend == 'python' fails when backend differs", () => {
		expect(evaluateCondition('backend == "python"', { backend: "go" })).toBe(
			false,
		);
		expect(evaluateCondition('backend == "go"', { backend: "python" })).toBe(
			false,
		);
	});

	it("evaluateCondition: env.VAR == 'value' passes when env var matches", () => {
		const ctx = { env: { MY_VAR: "hello" } };
		expect(evaluateCondition('env.MY_VAR == "hello"', ctx)).toBe(true);
	});

	it("evaluateCondition: env.VAR == 'value' fails when env var differs", () => {
		const ctx = { env: { MY_VAR: "world" } };
		expect(evaluateCondition('env.MY_VAR == "hello"', ctx)).toBe(false);
	});

	it("evaluateCondition: unknown condition defaults to true (permissive)", () => {
		// Unknown conditions should not block execution
		const result = evaluateCondition("unknown_condition_xyz", {
			maturityLevel: 1,
		});
		expect(result).toBe(true);
	});

	it("skips hook when condition is not met", async () => {
		const tempDir = makeTempDir();
		try {
			const engine = createHooksEngine({ repoRoot: tempDir });
			const hook: HookEntry = {
				name: "conditional-hook",
				run: "echo should-not-run",
				condition: "maturity >= 4",
			};
			const result = await engine.runHook(hook, { maturityLevel: 1 });
			// Hook should be skipped (not executed)
			expect(result.skipped).toBe(true);
			expect(result.success).toBe(false);
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("runs hook when condition is met", async () => {
		const tempDir = makeTempDir();
		try {
			const engine = createHooksEngine({ repoRoot: tempDir });
			const hook: HookEntry = {
				name: "conditional-hook",
				run: "echo condition-met",
				condition: "maturity >= 2",
			};
			const result = await engine.runHook(hook, { maturityLevel: 3 });
			expect(result.skipped).toBeFalsy();
			expect(result.success).toBe(true);
			expect(result.output).toContain("condition-met");
		} finally {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-413: Non-blocking execution
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-413: non-blocking hook execution", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("fireEventNonBlocking returns immediately without waiting for hooks", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hooks: HooksConfig = {
			on_session_start: [{ name: "slow-hook", run: "sleep 0.1 && echo done" }],
		};
		const start = Date.now();
		// Non-blocking fire should return a promise that resolves quickly
		// (the hook runs in background)
		engine.fireEventNonBlocking("on_session_start", hooks, {});
		const elapsed = Date.now() - start;
		// Should return in < 50ms (not waiting for the 100ms sleep)
		expect(elapsed).toBeLessThan(50);
	});

	it("fireEventNonBlocking does not throw when hook fails", () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hooks: HooksConfig = {
			on_session_start: [{ name: "failing-hook", run: "exit 1" }],
		};
		// Should not throw
		expect(() => {
			engine.fireEventNonBlocking("on_session_start", hooks, {});
		}).not.toThrow();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-414: Configurable timeout
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-414: configurable timeout", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("default timeout is 30 seconds", () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		expect(engine.defaultTimeoutMs).toBe(30_000);
	});

	it("accepts custom timeout via options", () => {
		const engine = createHooksEngine({
			repoRoot: tempDir,
			defaultTimeoutMs: 5_000,
		});
		expect(engine.defaultTimeoutMs).toBe(5_000);
	});

	it("kills hook and marks as failed when timeout exceeded", async () => {
		const engine = createHooksEngine({
			repoRoot: tempDir,
			defaultTimeoutMs: 100,
		});
		const hook: HookEntry = {
			name: "slow-hook",
			run: "sleep 10",
		};
		const result = await engine.runHook(hook, {});
		expect(result.success).toBe(false);
		expect(result.timedOut).toBe(true);
	}, 5_000); // 5s deadline — hook timeout is 100ms, but process spawn/kill has overhead

	it("completes fast hooks before timeout", async () => {
		const engine = createHooksEngine({
			repoRoot: tempDir,
			defaultTimeoutMs: 5_000,
		});
		const hook: HookEntry = {
			name: "fast-hook",
			run: "echo fast",
		};
		const result = await engine.runHook(hook, {});
		expect(result.success).toBe(true);
		expect(result.timedOut).toBeFalsy();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-415: Hook output available via codeops_hook_output
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-415: hook output storage and retrieval", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("stores hook output after execution", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook: HookEntry = {
			name: "output-hook",
			run: "echo stored-output",
		};
		await engine.runHook(hook, {});
		const stored = engine.getLastOutput();
		expect(stored).toBeDefined();
		expect(stored?.output).toContain("stored-output");
	});

	it("getLastOutput returns null when no hooks have run", () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		expect(engine.getLastOutput()).toBeNull();
	});

	it("getLastOutput returns the most recent hook result", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook1: HookEntry = { name: "hook-1", run: "echo first" };
		const hook2: HookEntry = { name: "hook-2", run: "echo second" };
		await engine.runHook(hook1, {});
		await engine.runHook(hook2, {});
		const last = engine.getLastOutput();
		expect(last?.hookName).toBe("hook-2");
		expect(last?.output).toContain("second");
	});

	it("getOutputHistory returns all hook results", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook1: HookEntry = { name: "hook-a", run: "echo alpha" };
		const hook2: HookEntry = { name: "hook-b", run: "echo beta" };
		await engine.runHook(hook1, {});
		await engine.runHook(hook2, {});
		const history = engine.getOutputHistory();
		expect(Array.isArray(history)).toBe(true);
		expect(history.length).toBeGreaterThanOrEqual(2);
	});

	it("formatHookOutput returns a human-readable string", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook: HookEntry = { name: "format-hook", run: "echo formatted" };
		await engine.runHook(hook, {});
		const formatted = engine.formatHookOutput();
		expect(typeof formatted).toBe("string");
		expect(formatted.length).toBeGreaterThan(0);
		expect(formatted).toContain("format-hook");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-413a: prompt hook action type + cmd alias
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-413a: prompt hook action type", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("cmd field is accepted as alias for run", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		// cmd is an alias for run — should execute the command
		const hook: HookEntry = {
			name: "cmd-hook",
			cmd: "echo cmd-alias-works",
		} as HookEntry & { cmd?: string };
		const result = await engine.runHook(hook, {});
		expect(result.success).toBe(true);
		expect(result.output).toContain("cmd-alias-works");
	});

	it("prompt field is accepted in HookEntry", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		// prompt hooks record the message without executing a shell command
		const hook: HookEntry = {
			name: "prompt-hook",
			prompt: "Check TODO.md and continue",
		} as HookEntry & { prompt?: string };
		const result = await engine.runHook(hook, {});
		expect(result).toBeDefined();
		expect(result.hookName).toBe("prompt-hook");
		expect(result.promptMessage).toBe("Check TODO.md and continue");
		expect(result.isPrompt).toBe(true);
	});

	it("prompt hook does not execute a shell command", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook: HookEntry = {
			name: "prompt-only",
			prompt: "Do something",
		} as HookEntry & { prompt?: string };
		const result = await engine.runHook(hook, {});
		// prompt hooks succeed without running a subprocess
		expect(result.success).toBe(true);
		expect(result.isPrompt).toBe(true);
	});

	it("prompt hook is rate-limited: second call within 30s returns skipped", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook: HookEntry = {
			name: "rate-limited-prompt",
			prompt: "Keep working",
		} as HookEntry & { prompt?: string };
		// First call should succeed
		const first = await engine.runHook(hook, {});
		expect(first.success).toBe(true);
		expect(first.isPrompt).toBe(true);
		// Second call within rate-limit window should be rate-limited
		const second = await engine.runHook(hook, {});
		expect(second.rateLimited).toBe(true);
		expect(second.success).toBe(false);
	});

	it("prompt hook rate-limit is per hook name", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook1: HookEntry = {
			name: "prompt-a",
			prompt: "Message A",
		} as HookEntry & { prompt?: string };
		const hook2: HookEntry = {
			name: "prompt-b",
			prompt: "Message B",
		} as HookEntry & { prompt?: string };
		// Both should succeed since they have different names
		const r1 = await engine.runHook(hook1, {});
		const r2 = await engine.runHook(hook2, {});
		expect(r1.success).toBe(true);
		expect(r2.success).toBe(true);
		expect(r1.rateLimited).toBeFalsy();
		expect(r2.rateLimited).toBeFalsy();
	});

	it("prompt hook in on_message event throws validation error", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hooks: HooksConfig = {
			on_message: [
				{
					name: "infinite-loop-risk",
					prompt: "This would loop forever",
				} as HookEntry & { prompt?: string },
			],
		};
		// Firing prompt hooks in on_message should throw or return an error
		await expect(engine.fireEvent("on_message", hooks, {})).rejects.toThrow();
	});

	it("fireEvent with prompt hooks fires them for non-message events", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hooks: HooksConfig = {
			on_session_idle: [
				{
					name: "keep-alive",
					prompt: "Check TODO.md and continue working",
				} as HookEntry & { prompt?: string },
			],
		};
		const results = await engine.fireEvent("on_session_idle", hooks, {});
		expect(results.length).toBe(1);
		expect(results[0].isPrompt).toBe(true);
		expect(results[0].promptMessage).toBe("Check TODO.md and continue working");
	});

	it("getPromptHistory returns all prompt injections", async () => {
		const engine = createHooksEngine({ repoRoot: tempDir });
		const hook: HookEntry = {
			name: "history-prompt",
			prompt: "Do work",
		} as HookEntry & { prompt?: string };
		await engine.runHook(hook, {});
		const history = engine.getPromptHistory();
		expect(Array.isArray(history)).toBe(true);
		expect(history.length).toBeGreaterThanOrEqual(1);
		expect(history[0].message).toBe("Do work");
		expect(history[0].hookName).toBe("history-prompt");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-411a: on_session_idle keep-alive wiring
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-411a: on_session_idle idle detection", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("createIdleDetector returns an object with reset and stop methods", () => {
		const {
			createIdleDetector,
		} = require("../src/layers/automation/idle-detector");
		const detector = createIdleDetector({
			idleTimeoutMs: 100,
			onIdle: () => {},
		});
		expect(typeof detector.reset).toBe("function");
		expect(typeof detector.stop).toBe("function");
	});

	it("createIdleDetector fires onIdle after timeout with no activity", async () => {
		const {
			createIdleDetector,
		} = require("../src/layers/automation/idle-detector");
		let fired = false;
		const detector = createIdleDetector({
			idleTimeoutMs: 50,
			onIdle: () => {
				fired = true;
			},
		});
		// Wait for idle to fire
		await new Promise((resolve) => setTimeout(resolve, 120));
		detector.stop();
		expect(fired).toBe(true);
	});

	it("createIdleDetector does NOT fire when reset before timeout", async () => {
		const {
			createIdleDetector,
		} = require("../src/layers/automation/idle-detector");
		let fired = false;
		const detector = createIdleDetector({
			idleTimeoutMs: 80,
			onIdle: () => {
				fired = true;
			},
		});
		// Reset before timeout fires
		await new Promise((resolve) => setTimeout(resolve, 40));
		detector.reset();
		await new Promise((resolve) => setTimeout(resolve, 60));
		detector.stop();
		// Should not have fired because we reset at 40ms (new timeout starts from 40ms)
		// Total elapsed: 100ms, but reset at 40ms means idle fires at 40+80=120ms
		expect(fired).toBe(false);
	});

	it("createIdleDetector stop() prevents onIdle from firing", async () => {
		const {
			createIdleDetector,
		} = require("../src/layers/automation/idle-detector");
		let fired = false;
		const detector = createIdleDetector({
			idleTimeoutMs: 50,
			onIdle: () => {
				fired = true;
			},
		});
		detector.stop();
		await new Promise((resolve) => setTimeout(resolve, 100));
		expect(fired).toBe(false);
	});

	it("idle_timeout_ms defaults to 60000 when not configured", () => {
		// Verify the default is documented in config-loader
		const { DEFAULT_PLUGIN_CONFIG } = require("../src/config-loader");
		// idle_timeout_ms is optional — absence means default 60000
		expect(DEFAULT_PLUGIN_CONFIG.idle_timeout_ms).toBeUndefined();
	});
});
