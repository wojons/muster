/**
 * Spin-up manager tests — verifies the /axiom-spinup command surface for
 * starting, stopping, and monitoring Axiom backend processes.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/spinup which
 * does not exist yet. All tests are expected to fail at the module-resolution
 * level until the spin-up module is implemented.
 *
 * REQ-PLG-600: /axiom-spinup command starts Axiom backend processes
 * REQ-PLG-601: Supports ralph, morty, serve, opencode targets
 * REQ-PLG-602: Detects already-running targets and reports status
 * REQ-PLG-603: /axiom-spinup stop <target> stops running backends
 * REQ-PLG-604: Background execution — does not block OpenCode session
 * REQ-PLG-605: Crash monitoring and reporting
 * REQ-PLG-606: codeops_spinup tool for programmatic agent use
 * REQ-PLG-607: Respects backend detection — helpful message when unavailable
 * REQ-PLG-608: Custom spin-up targets from .axiom/plugin.yaml
 * REQ-PLG-609: Custom target health checks
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-600
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-600 plan=phase-70-2/task-70-2-3/step-70-2-3-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Import from the not-yet-existing spinup module — this WILL fail at module
// resolution, which is the expected test-first signal.
import {
	BUILTIN_TARGETS,
	type CustomSpinupTarget,
	createSpinupManager,
	type SpinupTarget,
} from "../src/spinup";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-spinup-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
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
// REQ-PLG-600: /axiom-spinup command
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-600 — /axiom-spinup command", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		// Create a minimal Axiom repo structure
		mkdirs(join(tempDir, ".axiom"));
		mkdirs(join(tempDir, ".opencode"));
		mkdirs(join(tempDir, "specs"));
		mkdirs(join(tempDir, ".memory-bank"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("creates a SpinupManager from a repo directory", () => {
		const manager = createSpinupManager({ repoDir: tempDir });
		expect(manager).toBeDefined();
		expect(typeof manager.start).toBe("function");
		expect(typeof manager.stop).toBe("function");
		expect(typeof manager.status).toBe("function");
		expect(typeof manager.listTargets).toBe("function");
	});

	it("exposes start, stop, status, and listTargets methods", () => {
		const manager = createSpinupManager({ repoDir: tempDir });
		// Verify the full API surface exists
		expect(manager.start).toBeDefined();
		expect(manager.stop).toBeDefined();
		expect(manager.status).toBeDefined();
		expect(manager.listTargets).toBeDefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-601: Built-in spin-up targets
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-601 — Built-in spin-up targets", () => {
	it("defines the four spec-required built-in targets", () => {
		expect(BUILTIN_TARGETS).toBeDefined();
		expect(Array.isArray(BUILTIN_TARGETS)).toBe(true);

		const targetNames = BUILTIN_TARGETS.map((t: SpinupTarget) => t.name);
		expect(targetNames).toContain("ralph");
		expect(targetNames).toContain("morty");
		expect(targetNames).toContain("serve");
		expect(targetNames).toContain("opencode");
	});

	it("ralph target uses 'axiom run --work-item <id> --repo . --in-process'", () => {
		const ralph = BUILTIN_TARGETS.find((t: SpinupTarget) => t.name === "ralph");
		expect(ralph).toBeDefined();
		expect(ralph?.command).toContain("axiom run");
		expect(ralph?.command).toContain("--in-process");
	});

	it("morty target uses 'morty run --work-item <id> --repo .'", () => {
		const morty = BUILTIN_TARGETS.find((t: SpinupTarget) => t.name === "morty");
		expect(morty).toBeDefined();
		expect(morty?.command).toContain("morty run");
	});

	it("serve target uses 'axiom serve --port 8100'", () => {
		const serve = BUILTIN_TARGETS.find((t: SpinupTarget) => t.name === "serve");
		expect(serve).toBeDefined();
		expect(serve?.command).toContain("axiom serve");
	});

	it("opencode target uses 'axiom run --spinup-opencode'", () => {
		const opencode = BUILTIN_TARGETS.find(
			(t: SpinupTarget) => t.name === "opencode",
		);
		expect(opencode).toBeDefined();
		expect(opencode?.command).toContain("--spinup-opencode");
	});

	it("each built-in target has a description", () => {
		for (const target of BUILTIN_TARGETS) {
			expect(typeof target.description).toBe("string");
			expect(target.description.length).toBeGreaterThan(0);
		}
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-602: Duplicate detection
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-602 — Duplicate detection", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("reports status instead of starting a duplicate when target is already running", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			// Use a mock executor that simulates an already-running process
			executor: {
				isRunning: async (_target: string) => true,
				start: async () => ({
					pid: 0,
					success: false,
					reason: "already running",
				}),
				stop: async () => ({ success: true }),
			},
		});

		const result = await manager.start("serve");
		expect(result.success).toBe(false);
		expect(result.reason).toContain("already running");
	});

	it("status() returns running state for active targets", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async (target: string) => target === "serve",
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const status = await manager.status("serve");
		expect(status.running).toBe(true);
		expect(status.target).toBe("serve");
	});

	it("status() returns stopped state for inactive targets", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const status = await manager.status("ralph");
		expect(status.running).toBe(false);
		expect(status.target).toBe("ralph");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-603: Stop command
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-603 — /axiom-spinup stop", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("stops a running backend target", async () => {
		let isRunning = true;
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => isRunning,
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => {
					isRunning = false;
					return { success: true };
				},
			},
		});

		const result = await manager.stop("serve");
		expect(result.success).toBe(true);
	});

	it("returns failure when stopping a target that is not running", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 0, success: false, reason: "not started" }),
				stop: async () => ({ success: false, reason: "not running" }),
			},
		});

		const result = await manager.stop("ralph");
		expect(result.success).toBe(false);
		expect(result.reason).toContain("not running");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-604: Background execution
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-604 — Background execution", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("start() returns immediately without blocking", async () => {
		const startTime = Date.now();
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const result = await manager.start("serve");
		const elapsed = Date.now() - startTime;

		expect(result.success).toBe(true);
		expect(result.pid).toBeDefined();
		// Should return in well under 1 second (background, not blocking)
		expect(elapsed).toBeLessThan(1000);
	});

	it("start() returns a PID for the background process", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 42, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const result = await manager.start("serve");
		expect(result.pid).toBe(42);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-605: Crash monitoring
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-605 — Crash monitoring and reporting", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("reports crash events via onCrash callback", async () => {
		let crashReport: { target: string; exitCode: number } | undefined;

		const manager = createSpinupManager({
			repoDir: tempDir,
			onCrash: (target, exitCode) => {
				crashReport = { target, exitCode };
			},
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		// Simulate a crash notification
		manager.simulateCrash?.("ralph", 1);

		expect(crashReport).toBeDefined();
		expect(crashReport?.target).toBe("ralph");
		expect(crashReport?.exitCode).toBe(1);
	});

	it("crash message includes target name and exit code", async () => {
		let crashMessage = "";

		const manager = createSpinupManager({
			repoDir: tempDir,
			onCrash: (target, exitCode) => {
				crashMessage = `The ${target} process exited with code ${exitCode}. Want to see the logs?`;
			},
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		manager.simulateCrash?.("ralph", 1);

		expect(crashMessage).toContain("ralph");
		expect(crashMessage).toContain("code 1");
		expect(crashMessage).toContain("logs");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-607: Backend detection integration
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-607 — Backend detection integration", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		// Minimal repo — no .axiom/ dir means spec-only mode
		mkdirs(join(tempDir, "specs"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("returns helpful message when Python backend is not installed and ralph is requested", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({
					pid: 0,
					success: false,
					reason: "backend not available",
				}),
				stop: async () => ({ success: true }),
			},
			backendMode: "spec-only",
		});

		const result = await manager.start("ralph");
		expect(result.success).toBe(false);
		expect(result.reason).toBeDefined();
		// Per REQ-PLG-607: helpful message about missing backend
		expect(result.reason).toMatch(/not (installed|available)/i);
	});

	it("returns helpful message when Go backend is not installed and morty is requested", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({
					pid: 0,
					success: false,
					reason: "backend not available",
				}),
				stop: async () => ({ success: true }),
			},
			backendMode: "spec-only",
		});

		const result = await manager.start("morty");
		expect(result.success).toBe(false);
		expect(result.reason).toMatch(/not (installed|available)/i);
	});

	it("allows serve target when Python backend is available", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 8100, success: true }),
				stop: async () => ({ success: true }),
			},
			backendMode: "python",
		});

		const result = await manager.start("serve");
		expect(result.success).toBe(true);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-608: Custom spin-up targets from .axiom/plugin.yaml
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-608 — Custom spin-up targets", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("loads custom targets from .axiom/plugin.yaml spinup section", () => {
		const pluginYaml = `
spinup:
  custom_targets:
    - name: "dev-server"
      run: "npm run dev"
      description: "Start the development server"
      health_check: "curl -sf http://localhost:3000/health"
      health_interval_seconds: 10
`;
		touch(join(tempDir, ".axiom", "plugin.yaml"), pluginYaml);

		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 3000, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const targets = manager.listTargets();
		const customTarget = targets.find((t) => t.name === "dev-server");
		expect(customTarget).toBeDefined();
		expect(customTarget?.description).toBe("Start the development server");
		expect(customTarget?.command).toBe("npm run dev");
	});

	it("merges custom targets with built-in targets", () => {
		const pluginYaml = `
spinup:
  custom_targets:
    - name: "storybook"
      run: "npx storybook dev"
      description: "Start Storybook"
`;
		touch(join(tempDir, ".axiom", "plugin.yaml"), pluginYaml);

		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 6006, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const targets = manager.listTargets();
		const targetNames = targets.map((t) => t.name);

		// Built-in targets still present
		expect(targetNames).toContain("ralph");
		expect(targetNames).toContain("morty");
		expect(targetNames).toContain("serve");
		expect(targetNames).toContain("opencode");
		// Custom target added
		expect(targetNames).toContain("storybook");
	});

	it("custom targets cannot override built-in target names", () => {
		const pluginYaml = `
spinup:
  custom_targets:
    - name: "serve"
      run: "my-custom-serve"
      description: "Override attempt"
`;
		touch(join(tempDir, ".axiom", "plugin.yaml"), pluginYaml);

		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 0, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const targets = manager.listTargets();
		const serveTargets = targets.filter((t) => t.name === "serve");
		// Only one serve target — the built-in one
		expect(serveTargets.length).toBe(1);
		expect(serveTargets[0].command).toContain("axiom serve");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-609: Custom target health checks
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-609 — Custom target health checks", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("custom targets include health check configuration", () => {
		const pluginYaml = `
spinup:
  custom_targets:
    - name: "api"
      run: "node server.js"
      description: "API server"
      health_check: "curl -sf http://localhost:4000/health"
      health_interval_seconds: 5
`;
		touch(join(tempDir, ".axiom", "plugin.yaml"), pluginYaml);

		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 4000, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const targets = manager.listTargets();
		const apiTarget = targets.find((t) => t.name === "api") as
			| CustomSpinupTarget
			| undefined;
		expect(apiTarget).toBeDefined();
		expect(apiTarget?.healthCheck).toBe(
			"curl -sf http://localhost:4000/health",
		);
		expect(apiTarget?.healthIntervalSeconds).toBe(5);
	});

	it("health check defaults to 30 seconds when interval not specified", () => {
		const pluginYaml = `
spinup:
  custom_targets:
    - name: "worker"
      run: "node worker.js"
      description: "Background worker"
      health_check: "curl -sf http://localhost:5000/health"
`;
		touch(join(tempDir, ".axiom", "plugin.yaml"), pluginYaml);

		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 5000, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const targets = manager.listTargets();
		const workerTarget = targets.find((t) => t.name === "worker") as
			| CustomSpinupTarget
			| undefined;
		expect(workerTarget).toBeDefined();
		expect(workerTarget?.healthIntervalSeconds).toBe(30);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-606: codeops_spinup tool for programmatic agent use
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-606 — codeops_spinup tool", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("SpinupManager exposes a toToolResult() method for agent consumption", () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		expect(typeof manager.toToolResult).toBe("function");
	});

	it("toToolResult() returns JSON-serializable status for all targets", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async (target: string) => target === "serve",
				start: async () => ({ pid: 12345, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const result = await manager.toToolResult();
		expect(typeof result).toBe("string");

		const parsed = JSON.parse(result);
		expect(parsed.targets).toBeDefined();
		expect(Array.isArray(parsed.targets)).toBe(true);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// Integration: listTargets includes both built-in and custom
// ═══════════════════════════════════════════════════════════════════════════

describe("Integration — listTargets", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		mkdirs(join(tempDir, ".axiom"));
	});

	afterEach(() => {
		rmSync(tempDir, { recursive: true, force: true });
	});

	it("returns at least the 4 built-in targets when no plugin.yaml exists", () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({ pid: 0, success: true }),
				stop: async () => ({ success: true }),
			},
		});

		const targets = manager.listTargets();
		expect(targets.length).toBeGreaterThanOrEqual(4);
	});

	it("returns an unknown-target error for unrecognized target names", async () => {
		const manager = createSpinupManager({
			repoDir: tempDir,
			executor: {
				isRunning: async () => false,
				start: async () => ({
					pid: 0,
					success: false,
					reason: "unknown target",
				}),
				stop: async () => ({ success: true }),
			},
		});

		const result = await manager.start("nonexistent-target");
		expect(result.success).toBe(false);
		expect(result.reason).toContain("unknown");
	});
});
