/**
 * Bootstrap integration tests — verifies /axiom-setup ordered scaffold,
 * idempotent gap-fill, and immediate visible value per REQ-PLG-011, REQ-PLG-012,
 * and REQ-PLG-014.
 *
 * These tests are written BEFORE the bootstrap implementation exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-011, #REQ-PLG-012, #REQ-PLG-014
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-011 plan=phase-70-1/task-70-1-3/step-70-1-3-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import {
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
// ── Bootstrap function import (will fail until implemented) ─────────────
// REQ-PLG-011: The /axiom-setup command MUST perform steps in order
import { bootstrap } from "../src/bootstrap";
import { detectBackends } from "../src/detect";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-bootstrap-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

/** Helper to create nested directories */
function _mkdirs(...paths: string[]): void {
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
// REQ-PLG-011: Ordered scaffold steps
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-011: /axiom-setup ordered scaffold", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("bootstrap() returns a BootstrapResult with stepsCompleted array", async () => {
		const result = await bootstrap(tempDir);
		expect(result).toBeDefined();
		expect(Array.isArray(result.stepsCompleted)).toBe(true);
		expect(result.stepsCompleted.length).toBeGreaterThan(0);
	});

	it("scaffolds .opencode/ directory with agents, commands, skills", async () => {
		// REQ-PLG-011 step 2: Scaffold .opencode/ directory
		await bootstrap(tempDir);
		expect(existsSync(join(tempDir, ".opencode"))).toBe(true);
		// Must have at least agents/ and commands/ subdirectories
		expect(existsSync(join(tempDir, ".opencode", "agents"))).toBe(true);
		expect(existsSync(join(tempDir, ".opencode", "commands"))).toBe(true);
	});

	it("scaffolds specs/ with README.md, _prompt.md, _index.md, 00-PRD.md", async () => {
		// REQ-PLG-011 step 3: Scaffold specs/
		await bootstrap(tempDir);
		expect(existsSync(join(tempDir, "specs"))).toBe(true);
		expect(existsSync(join(tempDir, "specs", "README.md"))).toBe(true);
		expect(existsSync(join(tempDir, "specs", "_prompt.md"))).toBe(true);
		expect(existsSync(join(tempDir, "specs", "_index.md"))).toBe(true);
		expect(existsSync(join(tempDir, "specs", "00-PRD.md"))).toBe(true);
	});

	it("scaffolds .memory-bank/ with _prompt.md, _index.md", async () => {
		// REQ-PLG-011 step 4: Scaffold .memory-bank/
		await bootstrap(tempDir);
		expect(existsSync(join(tempDir, ".memory-bank"))).toBe(true);
		expect(existsSync(join(tempDir, ".memory-bank", "_prompt.md"))).toBe(true);
		expect(existsSync(join(tempDir, ".memory-bank", "_index.md"))).toBe(true);
	});

	it("creates or updates opencode.jsonc with Axiom defaults", async () => {
		// REQ-PLG-011 step 5: Create or update opencode.jsonc
		await bootstrap(tempDir);
		expect(existsSync(join(tempDir, "opencode.jsonc"))).toBe(true);
		const content = readFileSync(join(tempDir, "opencode.jsonc"), "utf-8");
		// Must be valid JSONC (at minimum, parseable after stripping comments)
		expect(content.length).toBeGreaterThan(0);
	});

	it("reports what was created in stepsCompleted", async () => {
		// REQ-PLG-011 step 7: Report what was created
		const result = await bootstrap(tempDir);
		expect(result.stepsCompleted).toBeDefined();
		// Must report at least the major scaffold directories
		const stepNames = result.stepsCompleted.map((s: string) => s.toLowerCase());
		expect(
			stepNames.some(
				(s: string) => s.includes("opencode") || s.includes(".opencode"),
			),
		).toBe(true);
		expect(stepNames.some((s: string) => s.includes("spec"))).toBe(true);
		expect(stepNames.some((s: string) => s.includes("memory"))).toBe(true);
	});

	it("suggests 1-3 next steps", async () => {
		// REQ-PLG-011 step 7: suggest exactly 1-3 next steps
		const result = await bootstrap(tempDir);
		expect(result.nextSteps).toBeDefined();
		expect(Array.isArray(result.nextSteps)).toBe(true);
		expect(result.nextSteps.length).toBeGreaterThanOrEqual(1);
		expect(result.nextSteps.length).toBeLessThanOrEqual(3);
	});

	it("performs steps in the required order", async () => {
		// REQ-PLG-011: steps MUST be in order:
		// 1. Detect repo type
		// 2. Scaffold .opencode/
		// 3. Scaffold specs/
		// 4. Scaffold .memory-bank/
		// 5. Create/update opencode.jsonc
		// 6. Run health check on backends
		// 7. Report + suggest next steps
		const result = await bootstrap(tempDir);
		const steps = result.stepsCompleted;

		// Find indices of key steps to verify ordering
		const opencodeIdx = steps.findIndex(
			(s: string) =>
				s.toLowerCase().includes("opencode") &&
				!s.toLowerCase().includes("opencode.jsonc"),
		);
		const specsIdx = steps.findIndex((s: string) =>
			s.toLowerCase().includes("spec"),
		);
		const memoryIdx = steps.findIndex((s: string) =>
			s.toLowerCase().includes("memory"),
		);
		const configIdx = steps.findIndex(
			(s: string) =>
				s.toLowerCase().includes("opencode.jsonc") ||
				s.toLowerCase().includes("config"),
		);

		// .opencode/ before specs/
		if (opencodeIdx >= 0 && specsIdx >= 0) {
			expect(opencodeIdx).toBeLessThan(specsIdx);
		}
		// specs/ before .memory-bank/
		if (specsIdx >= 0 && memoryIdx >= 0) {
			expect(specsIdx).toBeLessThan(memoryIdx);
		}
		// .memory-bank/ before opencode.jsonc
		if (memoryIdx >= 0 && configIdx >= 0) {
			expect(memoryIdx).toBeLessThan(configIdx);
		}
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-012: Idempotent gap-fill
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-012: idempotent gap-fill", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("does NOT overwrite user-authored specs/00-PRD.md on rerun", async () => {
		// First run: bootstrap creates 00-PRD.md
		await bootstrap(tempDir);
		const _templateContent = readFileSync(
			join(tempDir, "specs", "00-PRD.md"),
			"utf-8",
		);

		// User edits the file
		const userContent = "# My Custom PRD\n\nThis is my project.\n";
		writeFileSync(join(tempDir, "specs", "00-PRD.md"), userContent);

		// Second run: must NOT overwrite user content
		await bootstrap(tempDir);
		const afterRerun = readFileSync(
			join(tempDir, "specs", "00-PRD.md"),
			"utf-8",
		);
		expect(afterRerun).toBe(userContent);
	});

	it("does NOT overwrite user-authored .memory-bank/_prompt.md on rerun", async () => {
		await bootstrap(tempDir);

		// User edits the file
		const userContent =
			"# My Custom Memory Bank Rules\n\nCustom invariants here.\n";
		writeFileSync(join(tempDir, ".memory-bank", "_prompt.md"), userContent);

		// Second run: must NOT overwrite
		await bootstrap(tempDir);
		const afterRerun = readFileSync(
			join(tempDir, ".memory-bank", "_prompt.md"),
			"utf-8",
		);
		expect(afterRerun).toBe(userContent);
	});

	it("does NOT overwrite user-authored opencode.jsonc on rerun", async () => {
		await bootstrap(tempDir);

		// User edits the config
		const userConfig = '{\n  "custom": true\n}\n';
		writeFileSync(join(tempDir, "opencode.jsonc"), userConfig);

		// Second run: must NOT overwrite
		await bootstrap(tempDir);
		const afterRerun = readFileSync(join(tempDir, "opencode.jsonc"), "utf-8");
		expect(afterRerun).toBe(userConfig);
	});

	it("fills gaps when some directories are missing on rerun", async () => {
		// First run: full scaffold
		await bootstrap(tempDir);

		// User deletes .memory-bank/ but keeps specs/
		rmSync(join(tempDir, ".memory-bank"), { recursive: true, force: true });
		expect(existsSync(join(tempDir, ".memory-bank"))).toBe(false);
		expect(existsSync(join(tempDir, "specs"))).toBe(true);

		// Second run: must recreate .memory-bank/ without touching specs/
		const specsContentBefore = readFileSync(
			join(tempDir, "specs", "00-PRD.md"),
			"utf-8",
		);
		await bootstrap(tempDir);
		expect(existsSync(join(tempDir, ".memory-bank"))).toBe(true);
		expect(existsSync(join(tempDir, ".memory-bank", "_prompt.md"))).toBe(true);
		const specsContentAfter = readFileSync(
			join(tempDir, "specs", "00-PRD.md"),
			"utf-8",
		);
		expect(specsContentAfter).toBe(specsContentBefore);
	});

	it("reports only newly created items on rerun (not pre-existing)", async () => {
		// First run
		const firstResult = await bootstrap(tempDir);
		const firstCount = firstResult.stepsCompleted.length;

		// Second run on already-configured repo
		const secondResult = await bootstrap(tempDir);
		// Should report fewer steps (only gap-fill, if any)
		expect(secondResult.stepsCompleted.length).toBeLessThanOrEqual(firstCount);
	});

	it("second run on fully-configured repo completes without error", async () => {
		await bootstrap(tempDir);
		// Must not throw
		const result = await bootstrap(tempDir);
		expect(result).toBeDefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-014: Immediate visible value
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-014: immediate visible value", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
		// Simulate a basic repo with a README
		touch(
			join(tempDir, "README.md"),
			"# My Awesome Project\n\nA tool for building widgets.\n",
		);
		touch(
			join(tempDir, "package.json"),
			JSON.stringify({
				name: "my-awesome-project",
				description: "A tool for building widgets",
				version: "1.0.0",
			}),
		);
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("shows what Axiom detected about the repo", async () => {
		// REQ-PLG-014: Show the user what Axiom detected (languages, frameworks, existing CI)
		const result = await bootstrap(tempDir);
		expect(result.repoInfo).toBeDefined();
		expect(typeof result.repoInfo.name).toBe("string");
		expect(result.repoInfo.name.length).toBeGreaterThan(0);
	});

	it("generates a draft specs/00-PRD.md from repo context", async () => {
		// REQ-PLG-014: Generate a draft specs/00-PRD.md from README/package.json/existing docs
		await bootstrap(tempDir);
		expect(existsSync(join(tempDir, "specs", "00-PRD.md"))).toBe(true);
		const prd = readFileSync(join(tempDir, "specs", "00-PRD.md"), "utf-8");
		// The draft PRD should reference the project name or description
		expect(prd.length).toBeGreaterThan(50);
		// Should contain some structure (headings)
		expect(prd).toContain("#");
	});

	it("suggests the single most impactful next action", async () => {
		// REQ-PLG-014: Suggest the single most impactful next action
		const result = await bootstrap(tempDir);
		expect(result.nextSteps).toBeDefined();
		expect(result.nextSteps.length).toBeGreaterThanOrEqual(1);
		// First next step should be actionable (non-empty string)
		expect(result.nextSteps[0].length).toBeGreaterThan(10);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-011 step 6: Health check on available backends
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-011 step 6: health check on backends", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("stepsCompleted includes a health-check step", async () => {
		// REQ-PLG-011 step 6: Run health check on available backends
		const result = await bootstrap(tempDir);
		const stepNames = result.stepsCompleted.map((s: string) => s.toLowerCase());
		expect(
			stepNames.some(
				(s: string) => s.includes("health") || s.includes("backend"),
			),
		).toBe(true);
	});

	// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-011 plan=phase-70-1/task-70-1-3/step-70-1-3-2 test=bootstrap.test.ts#health-check-mode-discriminator
	it("health-check step contains real detectBackends() mode (discriminating regression)", async () => {
		// This test discriminates between a real detectBackends() call and a hardcoded
		// string like "Checked backend health (ok)". The health-check step message MUST
		// contain one of the valid mode values that only detectBackends() can produce.
		const validModes = ["spec-only", "python", "go", "hybrid"] as const;

		const result = await bootstrap(tempDir);
		const healthStep = result.stepsCompleted.find(
			(s: string) =>
				s.toLowerCase().includes("health") ||
				s.toLowerCase().includes("backend"),
		);
		expect(healthStep).toBeDefined();

		// The step message must contain exactly one of the valid mode values.
		// A hardcoded string like "Checked backend health (ok)" would FAIL this assertion.
		const containsMode = validModes.some((mode) => healthStep?.includes(mode));
		expect(containsMode).toBe(true);

		// Cross-check: the mode in the step message must match what detectBackends() returns
		const backends = detectBackends(tempDir);
		expect(healthStep).toContain(backends.mode);
	});

	it("repoInfo includes backend detection results", async () => {
		// The health check should surface backend availability in repoInfo.
		// Tightened: verify the full RepoInfo structure from detectRepoType(),
		// not just that name is a string.
		const result = await bootstrap(tempDir);
		expect(result.repoInfo).toBeDefined();
		// name must be a non-empty string (directory basename or package name)
		expect(typeof result.repoInfo.name).toBe("string");
		expect(result.repoInfo.name.length).toBeGreaterThan(0);
		// languages must be an array (may be empty for a bare temp dir)
		expect(Array.isArray(result.repoInfo.languages)).toBe(true);
		// frameworks must be an array
		expect(Array.isArray(result.repoInfo.frameworks)).toBe(true);
		// hasCI must be a boolean
		expect(typeof result.repoInfo.hasCI).toBe("boolean");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-017: --minimal flag
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-017: --minimal flag", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("minimal mode scaffolds only .opencode/ and opencode.jsonc", async () => {
		// REQ-PLG-017: --minimal scaffolds only the absolute minimum
		const _result = await bootstrap(tempDir, { minimal: true });
		expect(existsSync(join(tempDir, ".opencode"))).toBe(true);
		expect(existsSync(join(tempDir, "opencode.jsonc"))).toBe(true);
		// Should NOT create specs/ or .memory-bank/ in minimal mode
		expect(existsSync(join(tempDir, "specs"))).toBe(false);
		expect(existsSync(join(tempDir, ".memory-bank"))).toBe(false);
	});

	it("minimal mode includes copilot skill in .opencode/skills/", async () => {
		// REQ-PLG-017: --minimal scaffolds copilot skill
		// "scaffolds only the absolute minimum (.opencode/, opencode.jsonc, copilot skill)"
		const _result = await bootstrap(tempDir, { minimal: true });
		expect(existsSync(join(tempDir, ".opencode", "skills"))).toBe(true);
		// Must have at least a copilot skill file or directory
		const skillsDir = join(tempDir, ".opencode", "skills");
		const entries = readdirSync(skillsDir);
		const hasCopilot = entries.some((e: string) =>
			e.toLowerCase().includes("copilot"),
		);
		expect(hasCopilot).toBe(true);
	});
});
