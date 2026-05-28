/**
 * Backend detection — detects which Axiom backend runtime(s) are available
 * and resolves the active backend mode for the current session.
 *
 * REQ-PLG-050: Four backend modes (spec-only, python, go, hybrid)
 * REQ-PLG-051: Backend detection cached for session duration
 * REQ-PLG-054: Health check contracts (axiom run --help, morty --version)
 * REQ-PLG-055: Spec-only mode fully functional without backends
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-050
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-050 plan=phase-70-2/task-70-2-1/step-70-2-1-2
 */

import { execSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";

// ─── Types ──────────────────────────────────────────────────────────────────

/**
 * The four spec-defined backend modes per REQ-PLG-050.
 *
 * - "spec-only": No backend runtime available; plugin operates in read-only
 *   spec-guidance mode per REQ-PLG-055.
 * - "python": Python Axiom runtime (.axiom/ dir + `axiom` CLI on PATH).
 * - "go": Go Morty Loop runtime (`morty` CLI on PATH).
 * - "hybrid": Both Python and Go runtimes available; defaults to Python for
 *   interactive use per REQ-PLG-052.
 */
export type BackendMode = "spec-only" | "python" | "go" | "hybrid";

/** Health check descriptor for a single backend per REQ-PLG-054 */
export interface BackendHealthCheck {
	/** The CLI command used to verify the backend is alive */
	command: string;
}

/**
 * Full result of backend mode detection per REQ-PLG-050.
 */
export interface BackendDetectionResult {
	/** Resolved backend mode */
	mode: BackendMode;
	/** Whether the Python Axiom runtime is available */
	python: boolean;
	/** Whether the Go Morty Loop runtime is available */
	go: boolean;
	/**
	 * In hybrid mode, the backend preferred for interactive use per REQ-PLG-052.
	 * Only present when mode === "hybrid".
	 */
	defaultBackend?: "python" | "go";
	/**
	 * Health check descriptors per REQ-PLG-054.
	 * Present for each available backend.
	 */
	healthChecks?: {
		python?: BackendHealthCheck;
		go?: BackendHealthCheck;
	};
	/**
	 * Guidance message for spec-only mode per REQ-PLG-055.
	 * Only present when mode === "spec-only".
	 */
	guidance?: string;
}

/**
 * Session-scoped backend detection cache per REQ-PLG-051.
 * Detection is performed once per session and cached for the session duration.
 */
export interface SessionBackendCache {
	/**
	 * Return the cached detection result, or undefined if not yet detected.
	 */
	get(): BackendDetectionResult | undefined;
	/**
	 * Detect the backend mode for the given repo directory.
	 * Returns the cached result if already detected (same object reference).
	 * @param repoDir - Absolute path to the repository root
	 * @param opts - Optional overrides for testability
	 */
	detect(
		repoDir: string,
		opts?: BackendDetectionOptions,
	): BackendDetectionResult;
	/**
	 * Invalidate the cache so the next detect() call re-probes the environment.
	 */
	invalidate(): void;
}

// ─── Detection options ───────────────────────────────────────────────────────

/**
 * Options for detectBackendMode — allows test injection of CLI availability
 * without actually running subprocesses.
 */
export interface BackendDetectionOptions {
	/**
	 * Override whether the Python `axiom` CLI is available on PATH.
	 * When undefined, the function probes the real environment.
	 */
	pythonCliAvailable?: boolean;
	/**
	 * Override whether the Go `morty` CLI is available on PATH.
	 * When undefined, the function probes the real environment.
	 */
	goCliAvailable?: boolean;
}

// ─── Constants ───────────────────────────────────────────────────────────────

/** Health check command for the Python backend per REQ-PLG-054 */
const PYTHON_HEALTH_COMMAND = "axiom run --help";

/** Health check command for the Go backend per REQ-PLG-054 */
const GO_HEALTH_COMMAND = "morty --version";

/** Guidance message for spec-only mode per REQ-PLG-055 */
const SPEC_ONLY_GUIDANCE =
	"No Axiom backend runtime detected. The plugin is running in spec-only mode — " +
	"you can browse specs and get guidance, but execution features require a backend. " +
	"Install the Python runtime (`pip install axiom`) or the Go Morty Loop (`morty`) to unlock execution.";

// ─── Internal helpers ────────────────────────────────────────────────────────

/**
 * Probe whether the Python `axiom` CLI is available on PATH.
 * Uses `axiom run --help` per REQ-PLG-054.
 */
function probePythonCli(): boolean {
	try {
		execSync(PYTHON_HEALTH_COMMAND, { stdio: "ignore", timeout: 1500 });
		return true;
	} catch {
		return false;
	}
}

/**
 * Probe whether the Go `morty` CLI is available on PATH.
 * Uses `morty --version` per REQ-PLG-054.
 */
function probeGoCli(): boolean {
	try {
		execSync(GO_HEALTH_COMMAND, { stdio: "ignore", timeout: 1500 });
		return true;
	} catch {
		return false;
	}
}

/**
 * Check whether a .axiom/ directory exists in the repo root.
 * The Python backend requires both the CLI and the .axiom/ config directory.
 */
function hasCodeopsDir(repoDir: string): boolean {
	try {
		return existsSync(join(repoDir, ".axiom"));
	} catch {
		return false;
	}
}

// ─── Public API ──────────────────────────────────────────────────────────────

/**
 * Detect the active backend mode for the given repository.
 *
 * REQ-PLG-050: Resolves one of four modes:
 *   - "spec-only": no backend available
 *   - "python": .axiom/ dir + `axiom` CLI on PATH
 *   - "go": `morty` CLI on PATH
 *   - "hybrid": both Python and Go available
 *
 * REQ-PLG-054: Health check commands are recorded in the result.
 * REQ-PLG-055: spec-only result includes a guidance message.
 *
 * @param repoDir - Absolute path to the repository root
 * @param opts - Optional overrides for testability (skip real CLI probes)
 */
export function detectBackendMode(
	repoDir: string,
	opts?: BackendDetectionOptions,
): BackendDetectionResult {
	// Resolve Python availability:
	// - If opts.pythonCliAvailable is explicitly provided, use it.
	// - Otherwise probe the real environment.
	// Python backend also requires a .axiom/ directory.
	const codeopsDir = hasCodeopsDir(repoDir);
	const pythonCliAvailable =
		opts?.pythonCliAvailable !== undefined
			? opts.pythonCliAvailable
			: probePythonCli();
	const python = codeopsDir && pythonCliAvailable;

	// Resolve Go availability:
	// - If opts.goCliAvailable is explicitly provided, use it.
	// - Otherwise probe the real environment.
	const goCliAvailable =
		opts?.goCliAvailable !== undefined ? opts.goCliAvailable : probeGoCli();
	const go = goCliAvailable;

	// Resolve mode
	let mode: BackendMode;
	if (python && go) {
		mode = "hybrid";
	} else if (python) {
		mode = "python";
	} else if (go) {
		mode = "go";
	} else {
		mode = "spec-only";
	}

	// Build result
	const result: BackendDetectionResult = { mode, python, go };

	// REQ-PLG-054: attach health check descriptors for available backends
	if (python || go) {
		result.healthChecks = {};
		if (python) {
			result.healthChecks.python = { command: PYTHON_HEALTH_COMMAND };
		}
		if (go) {
			result.healthChecks.go = { command: GO_HEALTH_COMMAND };
		}
	}

	// REQ-PLG-052: in hybrid mode, default to Python for interactive use
	if (mode === "hybrid") {
		result.defaultBackend = "python";
	}

	// REQ-PLG-055: in spec-only mode, include guidance
	if (mode === "spec-only") {
		result.guidance = SPEC_ONLY_GUIDANCE;
	}

	return result;
}

/**
 * Create a session-scoped backend detection cache per REQ-PLG-051.
 *
 * Detection is performed once per session and the result is cached.
 * Call invalidate() to force re-detection (e.g., after environment changes).
 *
 * Each call to createSessionCache() returns an independent cache instance
 * with no shared state.
 */
export function createSessionCache(): SessionBackendCache {
	let cached: BackendDetectionResult | undefined;

	return {
		get(): BackendDetectionResult | undefined {
			return cached;
		},

		detect(
			repoDir: string,
			opts?: BackendDetectionOptions,
		): BackendDetectionResult {
			if (cached !== undefined) {
				// Return the same object reference — callers can use === to verify caching
				return cached;
			}
			cached = detectBackendMode(repoDir, opts);
			return cached;
		},

		invalidate(): void {
			cached = undefined;
		},
	};
}
