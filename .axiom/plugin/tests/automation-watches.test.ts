/**
 * Automation watches engine tests — verifies watch triggering, glob pattern
 * matching, run/block actions, and logging per
 * REQ-PLG-416, REQ-PLG-417, REQ-PLG-418, REQ-PLG-419, REQ-PLG-420.
 *
 * These tests are written BEFORE the watches engine module exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-416 through #REQ-PLG-420
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-416 plan=phase-70-4/task-70-4-3/step-70-4-3-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { WatchEntry } from "../src/config-loader";
// ── Watches engine module import (will fail until implemented) ───────────
// REQ-PLG-416: Watches trigger on tool.execute.before or tool.execute.after
// REQ-PLG-417: Support tool.read, tool.write, tool.edit, tool.bash, tool.*
// REQ-PLG-418: Support glob patterns for file matching
// REQ-PLG-419: Support run (non-blocking) and block action types
// REQ-PLG-420: Block actions logged; used sparingly for security-critical patterns
import {
	createWatchesEngine,
	matchesGlobPattern,
	matchesWatchTrigger,
	type WatchesEngine,
	type WatchTriggerEvent,
} from "../src/layers/automation/watches";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-watches-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-416: Watches trigger on tool invocations
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-416: watches trigger on tool invocations", () => {
	let tempDir: string;
	let engine: WatchesEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createWatchesEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("triggers a watch when a matching tool.write event fires", async () => {
		const watches: WatchEntry[] = [
			{
				name: "spec-change-guard",
				trigger: "tool.write",
				pattern: "specs/*.md",
				run: "echo spec-changed",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.write",
			filePath: "specs/my-spec.md",
		};
		const results = await engine.processEvent(event, watches);
		expect(results.length).toBe(1);
		expect(results[0].watchName).toBe("spec-change-guard");
		expect(results[0].triggered).toBe(true);
	});

	it("does not trigger a watch when trigger type does not match", async () => {
		const watches: WatchEntry[] = [
			{
				name: "write-only-watch",
				trigger: "tool.write",
				pattern: "*.ts",
				run: "echo write-detected",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.read",
			filePath: "src/index.ts",
		};
		const results = await engine.processEvent(event, watches);
		expect(results.length).toBe(1);
		expect(results[0].triggered).toBe(false);
	});

	it("does not trigger a watch when file pattern does not match", async () => {
		const watches: WatchEntry[] = [
			{
				name: "spec-guard",
				trigger: "tool.write",
				pattern: "specs/*.md",
				run: "echo spec-changed",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.write",
			filePath: "src/index.ts",
		};
		const results = await engine.processEvent(event, watches);
		expect(results[0].triggered).toBe(false);
	});

	it("returns empty array when no watches defined", async () => {
		const event: WatchTriggerEvent = { type: "tool.write", filePath: "any.ts" };
		const results = await engine.processEvent(event, []);
		expect(results).toEqual([]);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-417: Trigger types
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-417: trigger types", () => {
	it("matchesWatchTrigger: tool.read matches tool.read event", () => {
		expect(matchesWatchTrigger("tool.read", "tool.read")).toBe(true);
	});

	it("matchesWatchTrigger: tool.write matches tool.write event", () => {
		expect(matchesWatchTrigger("tool.write", "tool.write")).toBe(true);
	});

	it("matchesWatchTrigger: tool.edit matches tool.edit event", () => {
		expect(matchesWatchTrigger("tool.edit", "tool.edit")).toBe(true);
	});

	it("matchesWatchTrigger: tool.bash matches tool.bash event", () => {
		expect(matchesWatchTrigger("tool.bash", "tool.bash")).toBe(true);
	});

	it("matchesWatchTrigger: tool.* matches any tool event", () => {
		expect(matchesWatchTrigger("tool.*", "tool.read")).toBe(true);
		expect(matchesWatchTrigger("tool.*", "tool.write")).toBe(true);
		expect(matchesWatchTrigger("tool.*", "tool.bash")).toBe(true);
		expect(matchesWatchTrigger("tool.*", "tool.edit")).toBe(true);
	});

	it("matchesWatchTrigger: tool.read does not match tool.write", () => {
		expect(matchesWatchTrigger("tool.read", "tool.write")).toBe(false);
	});

	it("matchesWatchTrigger: tool.write does not match tool.read", () => {
		expect(matchesWatchTrigger("tool.write", "tool.read")).toBe(false);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-418: Glob pattern matching
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-418: glob pattern matching", () => {
	it("matchesGlobPattern: exact match", () => {
		expect(matchesGlobPattern("specs/my-spec.md", "specs/my-spec.md")).toBe(
			true,
		);
	});

	it("matchesGlobPattern: *.md matches markdown files", () => {
		expect(matchesGlobPattern("README.md", "*.md")).toBe(true);
		expect(matchesGlobPattern("specs/foo.md", "*.md")).toBe(false); // *.md doesn't cross dirs
	});

	it("matchesGlobPattern: specs/*.md matches files in specs/", () => {
		expect(matchesGlobPattern("specs/my-spec.md", "specs/*.md")).toBe(true);
		expect(matchesGlobPattern("specs/nested/deep.md", "specs/*.md")).toBe(
			false,
		);
	});

	it("matchesGlobPattern: *.env* matches .env files", () => {
		expect(matchesGlobPattern(".env", "*.env*")).toBe(true);
		expect(matchesGlobPattern(".env.local", "*.env*")).toBe(true);
		expect(matchesGlobPattern("production.env", "*.env*")).toBe(true);
	});

	it("matchesGlobPattern: src/**/*.ts matches nested TypeScript files", () => {
		expect(matchesGlobPattern("src/index.ts", "src/**/*.ts")).toBe(true);
		expect(
			matchesGlobPattern("src/layers/automation/hooks.ts", "src/**/*.ts"),
		).toBe(true);
		expect(matchesGlobPattern("tests/hooks.test.ts", "src/**/*.ts")).toBe(
			false,
		);
	});

	it("matchesGlobPattern: no pattern means always match", () => {
		expect(matchesGlobPattern("any/file.txt", undefined)).toBe(true);
		expect(matchesGlobPattern("another.ts", undefined)).toBe(true);
	});

	it("matchesGlobPattern: *.pem matches PEM files", () => {
		expect(matchesGlobPattern("server.pem", "*.pem")).toBe(true);
		expect(matchesGlobPattern("server.key", "*.pem")).toBe(false);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-419: Run and block action types
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-419: run and block action types", () => {
	let tempDir: string;
	let engine: WatchesEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createWatchesEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("run action executes the command and returns output", async () => {
		const watches: WatchEntry[] = [
			{
				name: "run-watch",
				trigger: "tool.write",
				pattern: "specs/*.md",
				run: "echo spec-updated",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.write",
			filePath: "specs/test.md",
		};
		const results = await engine.processEvent(event, watches);
		expect(results[0].triggered).toBe(true);
		expect(results[0].action).toBe("run");
		expect(results[0].blocked).toBeFalsy();
		expect(results[0].output).toContain("spec-updated");
	});

	it("block action prevents the tool invocation and returns message", async () => {
		const watches: WatchEntry[] = [
			{
				name: "security-file-guard",
				trigger: "tool.read",
				pattern: "*.env*",
				action: "block",
				message: "Reading .env files is blocked by security policy",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.read",
			filePath: ".env",
		};
		const results = await engine.processEvent(event, watches);
		expect(results[0].triggered).toBe(true);
		expect(results[0].action).toBe("block");
		expect(results[0].blocked).toBe(true);
		expect(results[0].blockMessage).toContain("blocked by security policy");
	});

	it("block action uses default message when none provided", async () => {
		const watches: WatchEntry[] = [
			{
				name: "block-no-msg",
				trigger: "tool.read",
				pattern: "*.pem",
				action: "block",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.read",
			filePath: "server.pem",
		};
		const results = await engine.processEvent(event, watches);
		expect(results[0].blocked).toBe(true);
		expect(typeof results[0].blockMessage).toBe("string");
		expect(results[0].blockMessage?.length).toBeGreaterThan(0);
	});

	it("run action is non-blocking (does not block the session)", async () => {
		const watches: WatchEntry[] = [
			{
				name: "slow-run-watch",
				trigger: "tool.write",
				pattern: "src/**/*.ts",
				run: "sleep 0.05 && echo done",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.write",
			filePath: "src/index.ts",
		};
		// processEvent should complete (run action is awaited for test purposes,
		// but in production it runs in background)
		const results = await engine.processEvent(event, watches);
		expect(results[0].triggered).toBe(true);
		expect(results[0].action).toBe("run");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-420: Block actions logged
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-420: block actions are logged", () => {
	let tempDir: string;
	let engine: WatchesEngine;

	beforeEach(() => {
		tempDir = makeTempDir();
		engine = createWatchesEngine({ repoRoot: tempDir });
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("records block actions in the block log", async () => {
		const watches: WatchEntry[] = [
			{
				name: "env-guard",
				trigger: "tool.read",
				pattern: "*.env*",
				action: "block",
				message: "Blocked: env file",
			},
		];
		const event: WatchTriggerEvent = {
			type: "tool.read",
			filePath: ".env.local",
		};
		await engine.processEvent(event, watches);
		const log = engine.getBlockLog();
		expect(log.length).toBeGreaterThanOrEqual(1);
		expect(log[0].watchName).toBe("env-guard");
		expect(log[0].filePath).toBe(".env.local");
	});

	it("getBlockLog returns empty array when no blocks have occurred", () => {
		const log = engine.getBlockLog();
		expect(Array.isArray(log)).toBe(true);
		expect(log.length).toBe(0);
	});

	it("accumulates multiple block log entries", async () => {
		const watches: WatchEntry[] = [
			{
				name: "env-guard",
				trigger: "tool.read",
				pattern: "*.env*",
				action: "block",
				message: "Blocked",
			},
		];
		await engine.processEvent({ type: "tool.read", filePath: ".env" }, watches);
		await engine.processEvent(
			{ type: "tool.read", filePath: ".env.local" },
			watches,
		);
		const log = engine.getBlockLog();
		expect(log.length).toBe(2);
	});
});
