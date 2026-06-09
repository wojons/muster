/**
 * Backend routing — manages active backend selection, switching, and status
 * reporting for the /axiom-backend command surface.
 *
 * REQ-PLG-052: Hybrid mode defaults to Python for interactive use; Go is
 *   opt-in for autonomous mode.
 * REQ-PLG-053: /axiom-backend command shows available backends and allows
 *   switching.
 *
 * Depends on: backend.ts (detection layer — REQ-PLG-050, REQ-PLG-051)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-052, specs/70-OpenCode-Plugin.md#REQ-PLG-053
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-052 plan=phase-70-2/task-70-2-2/step-70-2-2-1
 */

import {
	type BackendDetectionOptions,
	type BackendDetectionResult,
	type BackendMode,
	detectBackendMode,
} from "./backend";

// ─── Types ──────────────────────────────────────────────────────────────────

/** The two switchable backend identifiers */
export type BackendId = "python" | "go";

/** Use-case label per REQ-PLG-052 */
export type BackendUseCase = "interactive" | "autonomous";

/**
 * Describes the currently active backend.
 * Null when no backend is available (spec-only mode).
 */
export interface ActiveBackend {
	/** Which backend is currently active */
	backend: BackendId;
	/** The use-case this backend is associated with per REQ-PLG-052 */
	useCase: BackendUseCase;
}

/**
 * Result of a backend switch attempt.
 */
export interface BackendSwitchResult {
	/** Whether the switch succeeded */
	success: boolean;
	/** The previously active backend (before the switch), if any */
	previous?: BackendId;
	/** The currently active backend (after the switch), if switch succeeded */
	current?: BackendId;
	/** Human-readable reason if the switch failed */
	reason?: string;
}

/**
 * Status report for the /axiom-backend command per REQ-PLG-053.
 */
export interface BackendStatusReport {
	/** The detected backend mode (from detection layer) */
	mode: BackendMode;
	/** List of available backend identifiers */
	available: BackendId[];
	/** The currently active backend, or "none" if spec-only */
	active: "python" | "go" | "none";
	/** Human-readable summary for the /axiom-backend command output */
	summary: string;
}

/**
 * Options for creating a BackendRouter.
 */
export interface BackendRouterOptions {
	/** Absolute path to the repository root */
	repoDir: string;
	/** Optional detection overrides for testability */
	detectionOverrides?: BackendDetectionOptions;
}

/**
 * The backend router manages which backend is active and allows switching.
 */
export interface BackendRouter {
	/** Get the currently active backend, or null if no backend is available */
	getActive(): ActiveBackend | null;
	/** Switch to a different backend. Returns success/failure with reason. */
	switchTo(target: BackendId): BackendSwitchResult;
	/** Get a full status report for the /axiom-backend command */
	status(): BackendStatusReport;
}

// ─── Constants ──────────────────────────────────────────────────────────────

/** Use-case mapping per REQ-PLG-052 */
const USE_CASE_MAP: Record<BackendId, BackendUseCase> = {
	python: "interactive",
	go: "autonomous",
};

// ─── Internal helpers ───────────────────────────────────────────────────────

/**
 * Build the human-readable summary for the /axiom-backend command.
 */
function buildSummary(
	mode: BackendMode,
	_available: BackendId[],
	active: "python" | "go" | "none",
): string {
	if (mode === "spec-only") {
		return (
			"No backend runtime detected. Running in spec-only mode. " +
			"Install the Python runtime (`pip install axiom`) or the Go Morty Loop (`morty`) to unlock execution."
		);
	}

	if (mode === "python") {
		return "Backend: Python (axiom CLI). This is the default execution backend for interactive use.";
	}

	if (mode === "go") {
		return "Backend: Go (Morty Loop). This backend is optimized for autonomous/CI execution.";
	}

	// Hybrid
	const activeLabel =
		active === "python" ? "Python (interactive)" : "Go (autonomous)";
	const otherLabel =
		active === "python" ? "Go (autonomous)" : "Python (interactive)";
	return (
		`Backends available: Python (axiom CLI) and Go (Morty Loop). ` +
		`Active: ${activeLabel}. ` +
		`Switch to ${otherLabel} with /axiom-backend switch ${active === "python" ? "go" : "python"}.`
	);
}

// ─── Public API ─────────────────────────────────────────────────────────────

/**
 * Create a backend router for the given repository.
 *
 * The router performs backend detection on creation and manages the active
 * backend selection for the session. In hybrid mode, it defaults to Python
 * for interactive use per REQ-PLG-052.
 *
 * @param opts - Router options including repo directory and optional detection overrides
 */
export function createBackendRouter(opts: BackendRouterOptions): BackendRouter {
	// Detect available backends using the detection layer
	const detection: BackendDetectionResult = detectBackendMode(
		opts.repoDir,
		opts.detectionOverrides,
	);

	// Build the available backends list
	const available: BackendId[] = [];
	if (detection.python) available.push("python");
	if (detection.go) available.push("go");

	// Determine initial active backend per REQ-PLG-052:
	// - Hybrid: default to Python for interactive use
	// - Single backend: use the only available one
	// - Spec-only: no active backend
	let activeBackend: BackendId | null = null;
	if (detection.mode === "hybrid") {
		// REQ-PLG-052: default to Python for interactive use
		activeBackend = "python";
	} else if (detection.mode === "python") {
		activeBackend = "python";
	} else if (detection.mode === "go") {
		activeBackend = "go";
	}
	// spec-only: activeBackend stays null

	return {
		getActive(): ActiveBackend | null {
			if (activeBackend === null) return null;
			return {
				backend: activeBackend,
				useCase: USE_CASE_MAP[activeBackend],
			};
		},

		switchTo(target: BackendId): BackendSwitchResult {
			// Check if target is available
			if (!available.includes(target)) {
				return {
					success: false,
					reason: `Backend "${target}" is not available. Available backends: ${available.length > 0 ? available.join(", ") : "none"}`,
				};
			}

			const previous = activeBackend;
			activeBackend = target;

			return {
				success: true,
				previous: previous ?? undefined,
				current: target,
			};
		},

		status(): BackendStatusReport {
			const active: "python" | "go" | "none" = activeBackend ?? "none";
			return {
				mode: detection.mode,
				available: [...available],
				active,
				summary: buildSummary(detection.mode, available, active),
			};
		},
	};
}
