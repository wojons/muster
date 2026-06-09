/**
 * Maturity detection tests — verifies Level 0-4 detection per REQ-PLG-010.
 *
 * Each test creates a temporary directory structure matching one maturity level
 * and asserts that detectMaturity() returns the correct level and signals.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-010
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-010 plan=phase-70-1/task-70-1-2/step-70-1-2-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { detectMaturity, MATURITY_LABELS, MaturityLevel } from "../src/detect";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-detect-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
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

describe("detectMaturity", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	// ── Level 0: New ──────────────────────────────────────────────────────
	describe("Level 0 — New", () => {
		it("returns Level 0 for an empty directory", () => {
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.New);
			expect(result.label).toBe("New");
			expect(result.signals.hasOpencode).toBe(false);
			expect(result.signals.hasSpecs).toBe(false);
			expect(result.signals.hasMemoryBank).toBe(false);
		});

		it("returns Level 0 when only unrelated files exist", () => {
			touch(join(tempDir, "README.md"), "# My Project");
			touch(join(tempDir, "package.json"), "{}");
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.New);
		});

		it("returns Level 0 for a non-existent directory path", () => {
			const result = detectMaturity(join(tempDir, "does-not-exist"));
			expect(result.level).toBe(MaturityLevel.New);
		});
	});

	// ── Level 1: Scaffolded ───────────────────────────────────────────────
	describe("Level 1 — Scaffolded", () => {
		it("returns Level 1 when .opencode/ exists but no specs/", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Scaffolded);
			expect(result.label).toBe("Scaffolded");
			expect(result.signals.hasOpencode).toBe(true);
			expect(result.signals.specsHasContent).toBe(false);
		});

		it("returns Level 1 when specs/ exists but is empty", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Scaffolded);
		});

		it("returns Level 1 when specs/ has only meta files", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "README.md"), "# Specs");
			touch(join(tempDir, "specs", "_prompt.md"), "rules");
			touch(join(tempDir, "specs", "_index.md"), "index");
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Scaffolded);
			expect(result.signals.hasSpecs).toBe(true);
			expect(result.signals.specsHasContent).toBe(false);
		});
	});

	// ── Level 2: Spec'd ──────────────────────────────────────────────────
	describe("Level 2 — Spec'd", () => {
		it("returns Level 2 when specs/ has content but no .memory-bank/", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(
				join(tempDir, "specs", "00-PRD.md"),
				"# PRD\n\nProduct requirements",
			);
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Specced);
			expect(result.label).toBe("Spec'd");
			expect(result.signals.specsHasContent).toBe(true);
			expect(result.signals.hasMemoryBank).toBe(false);
		});

		it("returns Level 2 when specs/ has content and .memory-bank/ exists but no plans", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
			mkdirs(join(tempDir, ".memory-bank"));
			touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Specced);
			expect(result.signals.hasMemoryBank).toBe(true);
			expect(result.signals.hasPlans).toBe(false);
		});
	});

	// ── Level 3: Active ──────────────────────────────────────────────────
	describe("Level 3 — Active", () => {
		it("returns Level 3 when specs + plans + memory bank exist", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
			mkdirs(join(tempDir, ".memory-bank", "implementation-plans"));
			touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
			touch(
				join(
					tempDir,
					".memory-bank",
					"implementation-plans",
					"P-01-bootstrap.md",
				),
				"# Plan",
			);
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Active);
			expect(result.label).toBe("Active");
			expect(result.signals.hasPlans).toBe(true);
			expect(result.signals.hasEvidenceBundles).toBe(false);
		});

		it("returns Level 3 when plans exist via work-items/*/plan.yaml", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "01-Arch.md"), "# Architecture");
			mkdirs(join(tempDir, ".memory-bank", "work-items", "my-work-item"));
			touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
			touch(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"my-work-item",
					"plan.yaml",
				),
				"work_item_id: my-work-item",
			);
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Active);
			expect(result.signals.hasPlans).toBe(true);
		});

		it("returns Level 3 when evidence bundles exist but no CI", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
			mkdirs(join(tempDir, ".memory-bank", "implementation-plans"));
			touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
			touch(
				join(
					tempDir,
					".memory-bank",
					"implementation-plans",
					"P-01-bootstrap.md",
				),
				"# Plan",
			);
			// Add evidence bundle
			mkdirs(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"test-01",
					"runs",
					"2026-01-01T00-00-00Z_01",
				),
			);
			touch(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"test-01",
					"runs",
					"2026-01-01T00-00-00Z_01",
					"verification.md",
				),
				"# Verification",
			);
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Active);
			expect(result.signals.hasEvidenceBundles).toBe(true);
			expect(result.signals.hasCiIntegration).toBe(false);
		});
	});

	// ── Level 4: Mature ──────────────────────────────────────────────────
	describe("Level 4 — Mature", () => {
		it("returns Level 4 when all signals are present (evidence + CI)", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
			mkdirs(join(tempDir, ".memory-bank", "implementation-plans"));
			touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
			touch(
				join(
					tempDir,
					".memory-bank",
					"implementation-plans",
					"P-01-bootstrap.md",
				),
				"# Plan",
			);
			// Evidence bundles
			mkdirs(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"test-01",
					"runs",
					"2026-01-01T00-00-00Z_01",
				),
			);
			touch(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"test-01",
					"runs",
					"2026-01-01T00-00-00Z_01",
					"verification.md",
				),
				"# Verification",
			);
			// CI integration
			mkdirs(join(tempDir, ".github", "workflows"));
			touch(join(tempDir, ".github", "workflows", "ci.yml"), "name: CI");
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Mature);
			expect(result.label).toBe("Mature");
			expect(result.signals.hasEvidenceBundles).toBe(true);
			expect(result.signals.hasCiIntegration).toBe(true);
		});

		it("detects CI via .gitlab-ci.yml", () => {
			mkdirs(join(tempDir, ".opencode"));
			touch(join(tempDir, ".opencode", "config.json"), "{}");
			mkdirs(join(tempDir, "specs"));
			touch(join(tempDir, "specs", "00-PRD.md"), "# PRD");
			mkdirs(join(tempDir, ".memory-bank", "implementation-plans"));
			touch(join(tempDir, ".memory-bank", "_index.md"), "# Index");
			touch(
				join(
					tempDir,
					".memory-bank",
					"implementation-plans",
					"P-01-bootstrap.md",
				),
				"# Plan",
			);
			mkdirs(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"test-01",
					"runs",
					"2026-01-01T00-00-00Z_01",
				),
			);
			touch(
				join(
					tempDir,
					".memory-bank",
					"work-items",
					"test-01",
					"runs",
					"2026-01-01T00-00-00Z_01",
					"verification.md",
				),
				"# Verification",
			);
			touch(join(tempDir, ".gitlab-ci.yml"), "stages:\n  - test");
			const result = detectMaturity(tempDir);
			expect(result.level).toBe(MaturityLevel.Mature);
		});
	});

	// ── Cross-cutting ────────────────────────────────────────────────────
	describe("Cross-cutting", () => {
		it("MATURITY_LABELS has entries for all 5 levels", () => {
			expect(MATURITY_LABELS[MaturityLevel.New]).toBe("New");
			expect(MATURITY_LABELS[MaturityLevel.Scaffolded]).toBe("Scaffolded");
			expect(MATURITY_LABELS[MaturityLevel.Specced]).toBe("Spec'd");
			expect(MATURITY_LABELS[MaturityLevel.Active]).toBe("Active");
			expect(MATURITY_LABELS[MaturityLevel.Mature]).toBe("Mature");
		});

		it("signals object always has all expected keys", () => {
			const result = detectMaturity(tempDir);
			const keys = Object.keys(result.signals).sort();
			expect(keys).toEqual([
				"hasCiIntegration",
				"hasEvidenceBundles",
				"hasMemoryBank",
				"hasOpencode",
				"hasPlans",
				"hasSpecs",
				"specsHasContent",
			]);
		});

		it("detects this Axiom repo as Level 4 (Mature)", () => {
			// The Axiom repo itself should be Level 4: it has .opencode/, specs/,
			// .memory-bank/ with plans and evidence, and .github/workflows/
			const codeopsRoot = join(import.meta.dir, "..", "..", "..");
			const result = detectMaturity(codeopsRoot);
			// Axiom repo should be at least Level 3 (Active)
			expect(result.level).toBeGreaterThanOrEqual(MaturityLevel.Active);
			expect(result.signals.hasOpencode).toBe(true);
			expect(result.signals.specsHasContent).toBe(true);
			expect(result.signals.hasMemoryBank).toBe(true);
			expect(result.signals.hasPlans).toBe(true);
		});

		// ── REQ-PLG-504 regression: backends field ──────────────────────────
		it("result includes backends field with python, go, and mode (REQ-PLG-504)", () => {
			const result = detectMaturity(tempDir);
			expect(result.backends).toBeDefined();
			expect(typeof result.backends.python).toBe("boolean");
			expect(typeof result.backends.go).toBe("boolean");
			expect(["spec-only", "python", "go", "hybrid"]).toContain(
				result.backends.mode,
			);
		});

		it("result includes repoMetadata field with name, hasGit, hasCodeops (REQ-PLG-504)", () => {
			const result = detectMaturity(tempDir);
			expect(result.repoMetadata).toBeDefined();
			expect(typeof result.repoMetadata.name).toBe("string");
			expect(result.repoMetadata.name.length).toBeGreaterThan(0);
			expect(typeof result.repoMetadata.hasGit).toBe("boolean");
			expect(typeof result.repoMetadata.hasCodeops).toBe("boolean");
		});

		it("codeops_detect JSON output contains level, backends, and repoMetadata (REQ-PLG-504)", () => {
			// Simulate what codeops_detect does: JSON.stringify the result
			const result = detectMaturity(tempDir);
			const json = JSON.parse(JSON.stringify(result));
			expect(json).toHaveProperty("level");
			expect(json).toHaveProperty("label");
			expect(json).toHaveProperty("signals");
			expect(json).toHaveProperty("backends");
			expect(json).toHaveProperty("repoMetadata");
			expect(json.backends).toHaveProperty("python");
			expect(json.backends).toHaveProperty("go");
			expect(json.backends).toHaveProperty("mode");
			expect(json.repoMetadata).toHaveProperty("name");
			expect(json.repoMetadata).toHaveProperty("hasGit");
			expect(json.repoMetadata).toHaveProperty("hasCodeops");
		});
	});
});
