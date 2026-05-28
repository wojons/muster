/**
 * Hook wiring integration tests — verifies copilot auto-load, session init,
 * compaction survival, and shell env injection per the verified SDK hook
 * compatibility matrix from specs/70-OpenCode-Plugin.md#4.0.
 *
 * These tests are written BEFORE the hook implementation exists.
 * They MUST fail initially and drive the implementation.
 *
 * Hook mapping (from verified SDK v1.2.27 matrix):
 *   REQ-PLG-013: copilot auto-load → experimental.chat.system.transform
 *   REQ-PLG-030: session.created → lazy init on first command.execute.before or chat.message (fallback)
 *   REQ-PLG-031: compaction survival → experimental.session.compacting
 *   REQ-PLG-036: shell env injection → shell.env
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-013, #REQ-PLG-030, #REQ-PLG-031, #REQ-PLG-036
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-013 plan=phase-70-1/task-70-1-5/step-70-1-5-2
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { MaturityLevel } from "../src/detect";
// ── Hook module import (will fail until implemented) ────────────────────
// These imports drive the test-first contract: the module does not exist yet.
import {
	createCompactionHook,
	createCopilotSystemHook,
	createSessionInitHook,
	createShellEnvHook,
} from "../src/hooks";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-hooks-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

/** Helper to create nested directories */
function mkdirs(...paths: string[]): void {
	for (const p of paths) {
		mkdirSync(p, { recursive: true });
	}
}

/** Helper to create a file with content */
function touch(filePath: string, content = ""): void {
	mkdirSync(join(filePath, ".."), { recursive: true });
	writeFileSync(filePath, content);
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-013: Copilot auto-load via experimental.chat.system.transform
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-013: copilot auto-load via system prompt transform", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("createCopilotSystemHook returns a function matching experimental.chat.system.transform signature", () => {
		const hook = createCopilotSystemHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
		});
		expect(typeof hook).toBe("function");
	});

	it("injects copilot skill instruction for Level 0 repos", async () => {
		// Level 0 = New repo, copilot MUST be auto-loaded per REQ-PLG-013
		const hook = createCopilotSystemHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
		});
		const output = { system: ["You are a helpful assistant."] };
		await hook(
			{
				sessionID: "test-session-1",
				model: { providerID: "anthropic", modelID: "claude-sonnet-4-20250514" },
			},
			output,
		);
		// Must have appended copilot context to the system array
		expect(output.system.length).toBeGreaterThan(1);
		// The injected content must mention Axiom copilot
		const injected = output.system.slice(1).join("\n");
		expect(injected.toLowerCase()).toContain("axiom");
	});

	it("injects copilot skill instruction for Level 1 repos", async () => {
		mkdirs(join(tempDir, ".opencode"));
		const hook = createCopilotSystemHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Scaffolded,
		});
		const output = { system: ["Base system prompt."] };
		await hook(
			{
				sessionID: "test-session-2",
				model: { providerID: "anthropic", modelID: "claude-sonnet-4-20250514" },
			},
			output,
		);
		expect(output.system.length).toBeGreaterThan(1);
	});

	it("injects copilot skill instruction for Level 2 repos", async () => {
		mkdirs(join(tempDir, ".opencode"), join(tempDir, "specs"));
		touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
		const hook = createCopilotSystemHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Specced,
		});
		const output = { system: ["Base system prompt."] };
		await hook(
			{
				sessionID: "test-session-3",
				model: { providerID: "anthropic", modelID: "claude-sonnet-4-20250514" },
			},
			output,
		);
		expect(output.system.length).toBeGreaterThan(1);
	});

	it("does NOT inject copilot for Level 3+ repos (available on demand only)", async () => {
		// Level 3 = Active, copilot is available on demand but NOT auto-loaded
		mkdirs(
			join(tempDir, ".opencode"),
			join(tempDir, "specs"),
			join(tempDir, ".memory-bank"),
		);
		touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
		touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
		const hook = createCopilotSystemHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
		});
		const output = { system: ["Base system prompt."] };
		await hook(
			{
				sessionID: "test-session-4",
				model: { providerID: "anthropic", modelID: "claude-sonnet-4-20250514" },
			},
			output,
		);
		// System array must remain unchanged — no copilot injection
		expect(output.system).toEqual(["Base system prompt."]);
	});

	it("does NOT inject copilot for Level 4 repos", async () => {
		const hook = createCopilotSystemHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Mature,
		});
		const output = { system: ["Base system prompt."] };
		await hook(
			{
				sessionID: "test-session-5",
				model: { providerID: "anthropic", modelID: "claude-sonnet-4-20250514" },
			},
			output,
		);
		expect(output.system).toEqual(["Base system prompt."]);
	});

	it("is non-blocking: errors are caught and do not throw", async () => {
		// REQ-PLG-037: All hooks MUST be non-blocking
		const hook = createCopilotSystemHook({
			directory: "/nonexistent/path/that/will/fail",
			maturityLevel: MaturityLevel.New,
		});
		const output = { system: ["Base."] };
		// Must not throw even with a bad directory
		await expect(
			hook(
				{
					sessionID: "err-session",
					model: {
						providerID: "anthropic",
						modelID: "claude-sonnet-4-20250514",
					},
				},
				output,
			),
		).resolves.toBeUndefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-030: Session init via lazy init / event fallback
// (session.created is unavailable; fallback to lazy init on first
//  command.execute.before or chat.message, or use the event hook)
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-030: session init via lazy init fallback", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("createSessionInitHook returns a function", () => {
		const hook = createSessionInitHook({ directory: tempDir });
		expect(typeof hook).toBe("function");
	});

	it("initializes session state on first invocation", async () => {
		const hook = createSessionInitHook({ directory: tempDir });
		const result = await hook({ sessionID: "lazy-init-1" });
		expect(result).toBeDefined();
		expect(result.initialized).toBe(true);
		expect(result.maturityLevel).toBeDefined();
		expect(typeof result.maturityLevel).toBe("number");
	});

	it("is idempotent: second call returns same state without re-detecting", async () => {
		const hook = createSessionInitHook({ directory: tempDir });
		const first = await hook({ sessionID: "lazy-init-2" });
		const second = await hook({ sessionID: "lazy-init-2" });
		expect(first.maturityLevel).toBe(second.maturityLevel);
		// Both calls must succeed
		expect(first.initialized).toBe(true);
		expect(second.initialized).toBe(true);
	});

	it("detects maturity level during init", async () => {
		// Level 0 repo — no Axiom directories
		const hook = createSessionInitHook({ directory: tempDir });
		const result = await hook({ sessionID: "lazy-init-3" });
		expect(result.maturityLevel).toBe(MaturityLevel.New);
	});

	it("detects Level 1 when .opencode/ exists", async () => {
		mkdirs(join(tempDir, ".opencode"));
		const hook = createSessionInitHook({ directory: tempDir });
		const result = await hook({ sessionID: "lazy-init-4" });
		expect(result.maturityLevel).toBe(MaturityLevel.Scaffolded);
	});

	it("initializes active capability layers", async () => {
		// REQ-PLG-030: session init must initialize active capability layers
		const hook = createSessionInitHook({ directory: tempDir });
		const result = await hook({ sessionID: "lazy-init-5" });
		expect(result.activeLayers).toBeDefined();
		expect(Array.isArray(result.activeLayers)).toBe(true);
		// L0 is always on per spec
		expect(result.activeLayers).toContain("L0");
	});

	it("is non-blocking: errors are caught and do not throw", async () => {
		// REQ-PLG-037: All hooks MUST be non-blocking
		const hook = createSessionInitHook({ directory: "/nonexistent/path" });
		await expect(hook({ sessionID: "err-session" })).resolves.toBeDefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-031: Compaction survival via experimental.session.compacting
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-031: compaction survival via experimental.session.compacting", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("createCompactionHook returns a function matching experimental.session.compacting signature", () => {
		const hook = createCompactionHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
		});
		expect(typeof hook).toBe("function");
	});

	it("injects maturity level into compaction context", async () => {
		const hook = createCompactionHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
		});
		const output = {
			context: [] as string[],
			prompt: undefined as string | undefined,
		};
		await hook({ sessionID: "compact-1" }, output);
		// Must have appended context strings
		expect(output.context.length).toBeGreaterThan(0);
		// Context must mention the maturity level
		const joined = output.context.join("\n");
		expect(joined).toContain("maturity");
	});

	it("injects active work item reference when available", async () => {
		// Create a memory bank with _current.md pointing to a work item
		mkdirs(join(tempDir, ".memory-bank", "work-items"));
		touch(
			join(tempDir, ".memory-bank", "work-items", "_current.md"),
			"# Current Work Item\n\nActive: test-work-item-01",
		);
		const hook = createCompactionHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
			activeWorkItem: "test-work-item-01",
		});
		const output = {
			context: [] as string[],
			prompt: undefined as string | undefined,
		};
		await hook({ sessionID: "compact-2" }, output);
		const joined = output.context.join("\n");
		expect(joined).toContain("test-work-item-01");
	});

	it("injects key spec references", async () => {
		mkdirs(join(tempDir, "specs"));
		touch(join(tempDir, "specs", "README.md"), "# Specs\n- 00-PRD.md");
		touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
		const hook = createCompactionHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Specced,
		});
		const output = {
			context: [] as string[],
			prompt: undefined as string | undefined,
		};
		await hook({ sessionID: "compact-3" }, output);
		const joined = output.context.join("\n");
		expect(joined).toContain("spec");
	});

	it("injects memory bank pointers when memory bank exists", async () => {
		mkdirs(join(tempDir, ".memory-bank"));
		touch(join(tempDir, ".memory-bank", "_index.md"), "# Memory Bank Index");
		const hook = createCompactionHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
		});
		const output = {
			context: [] as string[],
			prompt: undefined as string | undefined,
		};
		await hook({ sessionID: "compact-4" }, output);
		const joined = output.context.join("\n");
		expect(joined).toContain("memory");
	});

	it("does NOT replace the default compaction prompt (only appends context)", async () => {
		const hook = createCompactionHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
		});
		const output = {
			context: [] as string[],
			prompt: undefined as string | undefined,
		};
		await hook({ sessionID: "compact-5" }, output);
		// prompt must remain undefined — we only append context, never replace
		expect(output.prompt).toBeUndefined();
	});

	it("is non-blocking: errors are caught and do not throw", async () => {
		// REQ-PLG-037: All hooks MUST be non-blocking
		const hook = createCompactionHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.New,
		});
		const output = {
			context: [] as string[],
			prompt: undefined as string | undefined,
		};
		await expect(
			hook({ sessionID: "err-session" }, output),
		).resolves.toBeUndefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-036: Shell env injection via shell.env
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-036: shell env injection via shell.env", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("createShellEnvHook returns a function matching shell.env signature", () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
			backend: "python",
		});
		expect(typeof hook).toBe("function");
	});

	it("injects AXIOM_MATURITY_LEVEL", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
			backend: "python",
		});
		const output = { env: {} as Record<string, string> };
		await hook({ cwd: tempDir, sessionID: "env-1" }, output);
		expect(output.env.AXIOM_MATURITY_LEVEL).toBe("3");
	});

	it("injects AXIOM_BACKEND", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
			backend: "python",
		});
		const output = { env: {} as Record<string, string> };
		await hook({ cwd: tempDir, sessionID: "env-2" }, output);
		expect(output.env.AXIOM_BACKEND).toBe("python");
	});

	it("injects AXIOM_REPO_ROOT", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
			backend: "none",
		});
		const output = { env: {} as Record<string, string> };
		await hook({ cwd: tempDir, sessionID: "env-3" }, output);
		expect(output.env.AXIOM_REPO_ROOT).toBe(tempDir);
	});

	it("injects AXIOM_SESSION_ID from input", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
			backend: "none",
		});
		const output = { env: {} as Record<string, string> };
		await hook({ cwd: tempDir, sessionID: "my-session-42" }, output);
		expect(output.env.AXIOM_SESSION_ID).toBe("my-session-42");
	});

	it("injects AXIOM_ACTIVE_LAYERS as comma-separated string", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.Active,
			backend: "python",
			activeLayers: ["L0", "L1", "L2"],
		});
		const output = { env: {} as Record<string, string> };
		await hook({ cwd: tempDir, sessionID: "env-4" }, output);
		expect(output.env.AXIOM_ACTIVE_LAYERS).toBe("L0,L1,L2");
	});

	it("defaults AXIOM_ACTIVE_LAYERS to L0 when no layers specified", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
			backend: "none",
		});
		const output = { env: {} as Record<string, string> };
		await hook({ cwd: tempDir, sessionID: "env-5" }, output);
		expect(output.env.AXIOM_ACTIVE_LAYERS).toBe("L0");
	});

	it("preserves existing env vars (does not clobber)", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
			backend: "none",
		});
		const output = {
			env: { EXISTING_VAR: "keep-me" } as Record<string, string>,
		};
		await hook({ cwd: tempDir, sessionID: "env-6" }, output);
		expect(output.env.EXISTING_VAR).toBe("keep-me");
		expect(output.env.AXIOM_MATURITY_LEVEL).toBe("0");
	});

	it("handles missing sessionID gracefully", async () => {
		const hook = createShellEnvHook({
			directory: tempDir,
			maturityLevel: MaturityLevel.New,
			backend: "none",
		});
		const output = { env: {} as Record<string, string> };
		// sessionID is optional in the SDK type
		await hook({ cwd: tempDir }, output);
		// AXIOM_SESSION_ID should be empty or "unknown", not crash
		expect(output.env.AXIOM_SESSION_ID).toBeDefined();
	});

	it("is non-blocking: errors are caught and do not throw", async () => {
		// REQ-PLG-037: All hooks MUST be non-blocking
		const hook = createShellEnvHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.New,
			backend: "none",
		});
		const output = { env: {} as Record<string, string> };
		await expect(
			hook({ cwd: "/nonexistent/path" }, output),
		).resolves.toBeUndefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-013a: Lightweight copilot marker for Level 1+ repos
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-013a: lightweight copilot marker for Level 1+ repos", () => {
	it("Level 0 (New) repos get full copilot instruction (existing behavior)", async () => {
		const hook = createCopilotSystemHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.New,
		});
		const output = { system: [] as string[] };
		await hook({ model: {} }, output);
		expect(output.system.length).toBe(1);
		// Full instruction contains Axiom context
		expect(output.system[0]).toContain("Axiom");
	});

	it("Level 1 repos get compact 4-line marker instead of full skill", async () => {
		const hook = createCopilotSystemHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.Specs,
		});
		const output = { system: [] as string[] };
		await hook({ model: {} }, output);
		expect(output.system.length).toBe(1);
		const marker = output.system[0];
		// Marker should be compact (4 lines)
		const lines = marker.split("\n");
		expect(lines.length).toBe(4);
		expect(lines[0]).toContain("Axiom active");
		expect(lines[1]).toContain("axiom-copilot/SKILL.md");
		expect(lines[2]).toContain("Active work");
		expect(lines[3]).toContain("Active layers");
	});

	it("Level 2 repos get compact marker", async () => {
		const hook = createCopilotSystemHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.Memory,
		});
		const output = { system: [] as string[] };
		await hook({ model: {} }, output);
		expect(output.system.length).toBe(1);
		expect(output.system[0]).toContain("# Axiom active");
	});

	it("Level 3+ repos get no injection (existing behavior)", async () => {
		const hook = createCopilotSystemHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.Active,
		});
		const output = { system: [] as string[] };
		await hook({ model: {} }, output);
		expect(output.system.length).toBe(0);
	});

	it("marker includes maturity level number", async () => {
		const hook = createCopilotSystemHook({
			directory: "/nonexistent/path",
			maturityLevel: MaturityLevel.Specs,
		});
		const output = { system: [] as string[] };
		await hook({ model: {} }, output);
		expect(output.system[0]).toContain(`maturity: ${MaturityLevel.Specs}`);
	});
});
