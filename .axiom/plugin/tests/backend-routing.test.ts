/**
 * Backend routing tests — verifies backend selection, switching, and the
 * /axiom-backend command surface per REQ-PLG-052 and REQ-PLG-053.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/backend-routing
 * which does not exist yet. All tests are expected to fail at the
 * module-resolution level until the routing module is implemented.
 *
 * REQ-PLG-052: Hybrid mode defaults to Python for interactive, Go opt-in for autonomous
 * REQ-PLG-053: /axiom-backend command shows available backends and allows switching
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-052, specs/70-OpenCode-Plugin.md#REQ-PLG-053
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-052 plan=phase-70-2/task-70-2-2/step-70-2-2-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Import from the not-yet-existing backend-routing module — this WILL fail
// at module resolution, which is the expected test-first signal.
import { createBackendRouter } from "../src/backend-routing";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-backend-routing-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
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
// REQ-PLG-052: Backend routing and default selection
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-052 — Backend routing and default selection", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	// ── Factory ─────────────────────────────────────────────────────────
	describe("createBackendRouter factory", () => {
		it("returns a BackendRouter object", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			expect(router).toBeDefined();
			expect(typeof router.getActive).toBe("function");
			expect(typeof router.switchTo).toBe("function");
			expect(typeof router.status).toBe("function");
		});
	});

	// ── Spec-only routing ───────────────────────────────────────────────
	describe("spec-only mode routing", () => {
		it("active backend is null when no backends are available", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			const active = router.getActive();
			expect(active).toBeNull();
		});

		it("switching to any backend fails gracefully in spec-only mode", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			const result = router.switchTo("python");
			expect(result.success).toBe(false);
			expect(result.reason).toBeDefined();
			expect(typeof result.reason).toBe("string");
		});
	});

	// ── Python-only routing ─────────────────────────────────────────────
	describe("Python-only mode routing", () => {
		it("active backend is python when only Python is available", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: false },
			});
			const active = router.getActive();
			expect(active).not.toBeNull();
			expect(active?.backend).toBe("python");
		});

		it("switching to go fails when go is not available", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: false },
			});
			const result = router.switchTo("go");
			expect(result.success).toBe(false);
			expect(result.reason).toContain("not available");
		});

		it("switching to python is a no-op success when already active", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: false },
			});
			const result = router.switchTo("python");
			expect(result.success).toBe(true);
			expect(router.getActive()?.backend).toBe("python");
		});
	});

	// ── Go-only routing ─────────────────────────────────────────────────
	describe("Go-only mode routing", () => {
		it("active backend is go when only Go is available", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: false, goCliAvailable: true },
			});
			const active = router.getActive();
			expect(active).not.toBeNull();
			expect(active?.backend).toBe("go");
		});

		it("switching to python fails when python is not available", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: false, goCliAvailable: true },
			});
			const result = router.switchTo("python");
			expect(result.success).toBe(false);
			expect(result.reason).toContain("not available");
		});
	});

	// ── Hybrid mode routing (REQ-PLG-052 core) ─────────────────────────
	describe("Hybrid mode routing (REQ-PLG-052 core)", () => {
		it("defaults to Python for interactive use in hybrid mode", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			const active = router.getActive();
			expect(active).not.toBeNull();
			expect(active?.backend).toBe("python");
		});

		it("allows switching to Go in hybrid mode", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			const result = router.switchTo("go");
			expect(result.success).toBe(true);
			expect(router.getActive()?.backend).toBe("go");
		});

		it("allows switching back to Python after switching to Go", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			router.switchTo("go");
			expect(router.getActive()?.backend).toBe("go");
			const result = router.switchTo("python");
			expect(result.success).toBe(true);
			expect(router.getActive()?.backend).toBe("python");
		});

		it("active backend includes the use-case label", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			const active = router.getActive();
			expect(active).not.toBeNull();
			// Python default should indicate interactive use
			expect(active?.useCase).toBe("interactive");
		});

		it("Go backend indicates autonomous use case", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			router.switchTo("go");
			const active = router.getActive();
			expect(active?.useCase).toBe("autonomous");
		});
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-053: /axiom-backend command (status report)
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-053 — Backend status report (/axiom-backend)", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	// ── Status report shape ─────────────────────────────────────────────
	describe("status report shape", () => {
		it("returns a BackendStatusReport with required fields", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			const report = router.status();
			expect(report).toBeDefined();
			expect(report.mode).toBeDefined();
			expect(typeof report.mode).toBe("string");
			expect(report.available).toBeDefined();
			expect(Array.isArray(report.available)).toBe(true);
			expect(typeof report.active).toBe("string"); // "python" | "go" | "none"
			expect(typeof report.summary).toBe("string");
		});
	});

	// ── Spec-only status ────────────────────────────────────────────────
	describe("spec-only status", () => {
		it("reports mode as spec-only with no available backends", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			const report = router.status();
			expect(report.mode).toBe("spec-only");
			expect(report.available).toEqual([]);
			expect(report.active).toBe("none");
		});

		it("summary includes guidance for installing a backend", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			const report = router.status();
			expect(report.summary.length).toBeGreaterThan(0);
			// Should mention installing a backend
			expect(report.summary.toLowerCase()).toContain("install");
		});
	});

	// ── Python-only status ──────────────────────────────────────────────
	describe("Python-only status", () => {
		it("reports python as the only available and active backend", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: false },
			});
			const report = router.status();
			expect(report.mode).toBe("python");
			expect(report.available).toEqual(["python"]);
			expect(report.active).toBe("python");
		});
	});

	// ── Go-only status ──────────────────────────────────────────────────
	describe("Go-only status", () => {
		it("reports go as the only available and active backend", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: false, goCliAvailable: true },
			});
			const report = router.status();
			expect(report.mode).toBe("go");
			expect(report.available).toEqual(["go"]);
			expect(report.active).toBe("go");
		});
	});

	// ── Hybrid status ───────────────────────────────────────────────────
	describe("Hybrid status", () => {
		it("reports both backends as available with python active by default", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			const report = router.status();
			expect(report.mode).toBe("hybrid");
			expect(report.available).toContain("python");
			expect(report.available).toContain("go");
			expect(report.available.length).toBe(2);
			expect(report.active).toBe("python");
		});

		it("status reflects switch to Go", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			router.switchTo("go");
			const report = router.status();
			expect(report.active).toBe("go");
			// Available backends don't change
			expect(report.available).toContain("python");
			expect(report.available).toContain("go");
		});

		it("summary mentions both backends and the active one", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			const report = router.status();
			expect(report.summary.toLowerCase()).toContain("python");
			expect(report.summary.toLowerCase()).toContain("go");
		});
	});

	// ── Switch result shape ─────────────────────────────────────────────
	describe("switch result shape", () => {
		it("successful switch includes previous and current backend", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: { pythonCliAvailable: true, goCliAvailable: true },
			});
			const result = router.switchTo("go");
			expect(result.success).toBe(true);
			expect(result.previous).toBe("python");
			expect(result.current).toBe("go");
		});

		it("failed switch includes reason and no current change", () => {
			const router = createBackendRouter({
				repoDir: tempDir,
				detectionOverrides: {
					pythonCliAvailable: false,
					goCliAvailable: false,
				},
			});
			const result = router.switchTo("python");
			expect(result.success).toBe(false);
			expect(result.reason).toBeDefined();
			expect(result.current).toBeUndefined();
		});
	});
});
