/**
 * Backend detection tests — verifies four backend modes per REQ-PLG-050
 * and session-cached detection per REQ-PLG-051.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/backend which
 * does not exist yet. All tests are expected to fail at the module-resolution
 * level until the backend module is implemented.
 *
 * REQ-PLG-050: Four backend modes (spec-only, Python, Go, hybrid)
 * REQ-PLG-051: Backend detection cached for session duration
 * REQ-PLG-054: Health check contracts (axiom run --help, morty --version)
 * REQ-PLG-055: Spec-only mode fully functional without backends
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-050, specs/70-OpenCode-Plugin.md#REQ-PLG-051
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-050 plan=phase-70-2/task-70-2-1/step-70-2-1-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Import from the not-yet-existing backend module — this WILL fail at module
// resolution, which is the expected test-first signal.
import {
	type BackendMode,
	createSessionCache,
	detectBackendMode,
} from "../src/backend";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-backend-detect-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
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
// REQ-PLG-050: Four backend modes
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-050 — Backend mode detection", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	// ── Spec-only mode ──────────────────────────────────────────────────
	describe("spec-only mode (no backend)", () => {
		it("returns spec-only when no .axiom/ dir and no CLIs available", () => {
			// Empty repo — no .axiom/, no axiom CLI, no morty CLI
			const result = detectBackendMode(tempDir);
			expect(result.mode).toBe("spec-only");
			expect(result.python).toBe(false);
			expect(result.go).toBe(false);
		});

		it("returns spec-only when .axiom/ exists but axiom CLI is missing", () => {
			// .axiom/ dir present but no CLI on PATH
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: false,
				goCliAvailable: false,
			});
			expect(result.mode).toBe("spec-only");
			expect(result.python).toBe(false);
		});

		it("spec-only result includes helpful guidance message per REQ-PLG-055", () => {
			const result = detectBackendMode(tempDir);
			expect(result.mode).toBe("spec-only");
			// REQ-PLG-055: guide user to install a backend
			expect(result.guidance).toBeDefined();
			expect(typeof result.guidance).toBe("string");
			expect(result.guidance?.length).toBeGreaterThan(0);
		});
	});

	// ── Python-only mode ────────────────────────────────────────────────
	describe("Python-only mode", () => {
		it("returns python when .axiom/ exists and axiom CLI is available", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: true,
				goCliAvailable: false,
			});
			expect(result.mode).toBe("python");
			expect(result.python).toBe(true);
			expect(result.go).toBe(false);
		});

		it("python health check uses 'axiom run --help' per REQ-PLG-054", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: true,
				goCliAvailable: false,
			});
			expect(result.python).toBe(true);
			expect(result.healthChecks?.python?.command).toBe("axiom run --help");
		});
	});

	// ── Go-only mode ────────────────────────────────────────────────────
	describe("Go-only mode (Morty Loop)", () => {
		it("returns go when morty CLI is available but no Python backend", () => {
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: false,
				goCliAvailable: true,
			});
			expect(result.mode).toBe("go");
			expect(result.python).toBe(false);
			expect(result.go).toBe(true);
		});

		it("go health check uses 'morty --version' per REQ-PLG-054", () => {
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: false,
				goCliAvailable: true,
			});
			expect(result.go).toBe(true);
			expect(result.healthChecks?.go?.command).toBe("morty --version");
		});
	});

	// ── Hybrid mode ─────────────────────────────────────────────────────
	describe("Hybrid mode (Python + Go)", () => {
		it("returns hybrid when both Python and Go backends are available", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: true,
				goCliAvailable: true,
			});
			expect(result.mode).toBe("hybrid");
			expect(result.python).toBe(true);
			expect(result.go).toBe(true);
		});

		it("hybrid mode defaults to Python for interactive use per REQ-PLG-052", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: true,
				goCliAvailable: true,
			});
			expect(result.mode).toBe("hybrid");
			// REQ-PLG-052: default to Python for interactive
			expect(result.defaultBackend).toBe("python");
		});

		it("hybrid mode reports both health checks per REQ-PLG-054", () => {
			mkdirs(join(tempDir, ".axiom"));
			touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
			const result = detectBackendMode(tempDir, {
				pythonCliAvailable: true,
				goCliAvailable: true,
			});
			expect(result.healthChecks?.python?.command).toBe("axiom run --help");
			expect(result.healthChecks?.go?.command).toBe("morty --version");
		});
	});

	// ── Mode string exhaustiveness ──────────────────────────────────────
	describe("Mode type safety", () => {
		it("mode is one of the four spec-defined values", () => {
			const result = detectBackendMode(tempDir);
			const validModes: BackendMode[] = ["spec-only", "python", "go", "hybrid"];
			expect(validModes).toContain(result.mode);
		});

		it("result shape includes python, go, mode, and optional fields", () => {
			const result = detectBackendMode(tempDir);
			expect(typeof result.python).toBe("boolean");
			expect(typeof result.go).toBe("boolean");
			expect(typeof result.mode).toBe("string");
			// Optional fields exist on the type
			expect("guidance" in result || result.guidance === undefined).toBe(true);
			expect(
				"healthChecks" in result || result.healthChecks === undefined,
			).toBe(true);
			expect(
				"defaultBackend" in result || result.defaultBackend === undefined,
			).toBe(true);
		});
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-051: Session-cached detection
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-051 — Session-cached backend detection", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("createSessionCache returns a cache object", () => {
		const cache = createSessionCache();
		expect(cache).toBeDefined();
		expect(typeof cache.get).toBe("function");
		expect(typeof cache.detect).toBe("function");
		expect(typeof cache.invalidate).toBe("function");
	});

	it("first call to detect() performs actual detection", () => {
		const cache = createSessionCache();
		const result = cache.detect(tempDir);
		expect(result).toBeDefined();
		expect(result.mode).toBe("spec-only");
	});

	it("second call to detect() returns cached result without re-detecting", () => {
		const cache = createSessionCache();
		const first = cache.detect(tempDir);
		const second = cache.detect(tempDir);
		// Same object reference — cached, not re-detected
		expect(second).toBe(first);
	});

	it("get() returns undefined before first detect()", () => {
		const cache = createSessionCache();
		expect(cache.get()).toBeUndefined();
	});

	it("get() returns cached result after detect()", () => {
		const cache = createSessionCache();
		cache.detect(tempDir);
		const cached = cache.get();
		expect(cached).toBeDefined();
		expect(cached?.mode).toBe("spec-only");
	});

	it("invalidate() clears the cache so next detect() re-probes", () => {
		const cache = createSessionCache();
		const first = cache.detect(tempDir);
		cache.invalidate();
		expect(cache.get()).toBeUndefined();
		// After invalidation, detect() should re-probe (new object)
		const second = cache.detect(tempDir);
		expect(second).not.toBe(first); // Different object reference
		expect(second.mode).toBe(first.mode); // Same result for same dir
	});

	it("cache is scoped to a single session (new cache = fresh state)", () => {
		const cache1 = createSessionCache();
		const cache2 = createSessionCache();
		cache1.detect(tempDir);
		// cache2 should have no cached result
		expect(cache2.get()).toBeUndefined();
	});

	it("cached result preserves all detection fields", () => {
		mkdirs(join(tempDir, ".axiom"));
		touch(join(tempDir, ".axiom", "axiom.config.yaml"), "repo_id: test");
		const cache = createSessionCache();
		const _result = cache.detect(tempDir, {
			pythonCliAvailable: true,
			goCliAvailable: false,
		});
		const cached = cache.get();
		expect(cached).toBeDefined();
		expect(cached?.mode).toBe("python");
		expect(cached?.python).toBe(true);
		expect(cached?.go).toBe(false);
	});
});
