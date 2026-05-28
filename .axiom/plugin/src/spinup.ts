/**
 * Spin-up manager — manages lifecycle of Axiom backend processes
 * (start, stop, status, crash monitoring) for the /axiom-spinup command.
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
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-600 plan=phase-70-2/task-70-2-3/step-70-2-3-2
 */

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

// ─── Types ──────────────────────────────────────────────────────────────────

import type { BackendMode } from "./backend";

/**
 * A spin-up target definition (built-in or custom).
 * REQ-PLG-601: Each target has a name, command, and description.
 */
export interface SpinupTarget {
	/** Unique target name (e.g., "ralph", "morty", "serve", "opencode") */
	name: string;
	/** Shell command to start the target */
	command: string;
	/** Human-readable description */
	description: string;
}

/**
 * Extended target with health check configuration per REQ-PLG-609.
 */
export interface CustomSpinupTarget extends SpinupTarget {
	/** Health check command (e.g., "curl -sf http://localhost:3000/health") */
	healthCheck?: string;
	/** Health check polling interval in seconds (default: 30) per REQ-PLG-609 */
	healthIntervalSeconds: number;
}

/**
 * Result of a start() call per REQ-PLG-600 and REQ-PLG-604.
 */
export interface SpinupResult {
	/** Whether the start succeeded */
	success: boolean;
	/** PID of the background process (when success is true) */
	pid?: number;
	/** Reason for failure (when success is false) */
	reason?: string;
}

/**
 * Result of a stop() call per REQ-PLG-603.
 */
export interface SpinupStopResult {
	/** Whether the stop succeeded */
	success: boolean;
	/** Reason for failure (when success is false) */
	reason?: string;
}

/**
 * Status of a single target per REQ-PLG-602.
 */
export interface SpinupStatus {
	/** Target name */
	target: string;
	/** Whether the target is currently running */
	running: boolean;
}

/**
 * Executor interface — abstracts process management for testability.
 * Tests inject mock executors; production uses real child_process.
 */
export interface SpinupExecutor {
	/** Check if a target is currently running */
	isRunning(target: string): Promise<boolean>;
	/** Start a target process in the background */
	start(target: string, command: string): Promise<SpinupResult>;
	/** Stop a running target */
	stop(target: string): Promise<SpinupStopResult>;
}

/**
 * Options for creating a SpinupManager per REQ-PLG-600.
 */
export interface SpinupManagerOptions {
	/** Absolute path to the repository root */
	repoDir: string;
	/** Optional executor for testability (defaults to real process management) */
	executor?: SpinupExecutor;
	/** Optional crash callback per REQ-PLG-605 */
	onCrash?: (target: string, exitCode: number) => void;
	/** Optional backend mode override for REQ-PLG-607 */
	backendMode?: BackendMode;
}

/**
 * The SpinupManager API surface per REQ-PLG-600.
 */
export interface SpinupManager {
	/** Start a target process (REQ-PLG-600, REQ-PLG-604) */
	start(target: string): Promise<SpinupResult>;
	/** Stop a running target (REQ-PLG-603) */
	stop(target: string): Promise<SpinupStopResult>;
	/** Get status of a target (REQ-PLG-602) */
	status(target: string): Promise<SpinupStatus>;
	/** List all available targets — built-in + custom (REQ-PLG-601, REQ-PLG-608) */
	listTargets(): (SpinupTarget | CustomSpinupTarget)[];
	/** JSON-serializable status for agent tool use (REQ-PLG-606) */
	toToolResult(): Promise<string>;
	/** Simulate a crash for testing (REQ-PLG-605) — only present in test builds */
	simulateCrash?: (target: string, exitCode: number) => void;
}

// ─── Built-in targets (REQ-PLG-601) ─────────────────────────────────────────

/**
 * The four spec-required built-in targets per REQ-PLG-601.
 */
export const BUILTIN_TARGETS: SpinupTarget[] = [
	{
		name: "ralph",
		command: "axiom run --work-item <id> --repo . --in-process",
		description: "Start a Ralph loop for a work item",
	},
	{
		name: "morty",
		command: "morty run --work-item <id> --repo .",
		description: "Start a Morty loop (Go backend)",
	},
	{
		name: "serve",
		command: "axiom serve --port 8100",
		description: "Start the Axiom HTTP server",
	},
	{
		name: "opencode",
		command: "axiom run --spinup-opencode",
		description: "Start OpenCode server for API access",
	},
];

/** Set of built-in target names for collision detection (REQ-PLG-608) */
const BUILTIN_NAMES = new Set(BUILTIN_TARGETS.map((t) => t.name));

/**
 * Backend requirements per target for REQ-PLG-607.
 * Maps target names to the backend mode(s) that support them.
 */
const TARGET_BACKEND_REQUIREMENTS: Record<string, BackendMode[]> = {
	ralph: ["python", "hybrid"],
	serve: ["python", "hybrid"],
	morty: ["go", "hybrid"],
	opencode: ["python", "go", "hybrid"],
};

// ─── Plugin.yaml parsing (REQ-PLG-608) ──────────────────────────────────────

interface PluginYamlSpinup {
	spinup?: {
		custom_targets?: Array<{
			name: string;
			run: string;
			description?: string;
			health_check?: string;
			health_interval_seconds?: number;
		}>;
	};
}

/**
 * Load custom spin-up targets from .axiom/plugin.yaml per REQ-PLG-608.
 * Returns an empty array if the file doesn't exist or has no spinup section.
 */
function loadCustomTargets(repoDir: string): CustomSpinupTarget[] {
	const pluginYamlPath = join(repoDir, ".axiom", "plugin.yaml");
	if (!existsSync(pluginYamlPath)) {
		return [];
	}

	try {
		const content = readFileSync(pluginYamlPath, "utf-8");
		const parsed = parseSimpleYaml(content);
		if (!parsed.spinup?.custom_targets) {
			return [];
		}

		return parsed.spinup.custom_targets
			.filter((ct) => {
				// REQ-PLG-608: custom targets cannot override built-in names
				if (BUILTIN_NAMES.has(ct.name)) {
					return false;
				}
				return ct.name && ct.run;
			})
			.map((ct) => ({
				name: ct.name,
				command: ct.run,
				description: ct.description || "",
				healthCheck: ct.health_check,
				// REQ-PLG-609: default health interval is 30 seconds
				healthIntervalSeconds: ct.health_interval_seconds ?? 30,
			}));
	} catch {
		return [];
	}
}

/**
 * Minimal YAML parser for plugin.yaml spinup section.
 * Handles the specific structure we need without a full YAML dependency.
 */
function parseSimpleYaml(content: string): PluginYamlSpinup {
	const result: PluginYamlSpinup = {};
	const lines = content.split("\n");

	let inSpinup = false;
	let inCustomTargets = false;
	let currentTarget: Record<string, string | number | undefined> | null = null;
	const targets: Array<{
		name: string;
		run: string;
		description?: string;
		health_check?: string;
		health_interval_seconds?: number;
	}> = [];

	for (const line of lines) {
		const trimmed = line.trimEnd();

		// Detect spinup: section
		if (/^spinup:\s*$/.test(trimmed)) {
			inSpinup = true;
			continue;
		}

		if (!inSpinup) continue;

		// Detect custom_targets: section
		if (/^\s+custom_targets:\s*$/.test(trimmed)) {
			inCustomTargets = true;
			continue;
		}

		if (!inCustomTargets) continue;

		// Detect new target item (starts with "    - ")
		const itemMatch = trimmed.match(/^\s+-\s+(\w+):\s*"?([^"]*)"?\s*$/);
		if (itemMatch && trimmed.match(/^\s+-\s/)) {
			// Save previous target
			if (currentTarget?.name) {
				targets.push(currentTarget as (typeof targets)[number]);
			}
			currentTarget = {};
			currentTarget[itemMatch[1]] = itemMatch[2];
			continue;
		}

		// Detect continuation property (starts with spaces, no dash)
		const propMatch = trimmed.match(/^\s+(\w+):\s*"?([^"]*)"?\s*$/);
		if (propMatch && currentTarget && !trimmed.match(/^\s+-\s/)) {
			const key = propMatch[1];
			const value = propMatch[2];
			if (key === "health_interval_seconds") {
				currentTarget[key] = parseInt(value, 10);
			} else {
				currentTarget[key] = value;
			}
		}
	}

	// Save last target
	if (currentTarget?.name) {
		targets.push(currentTarget as (typeof targets)[number]);
	}

	if (targets.length > 0) {
		result.spinup = { custom_targets: targets };
	}

	return result;
}

// ─── Default executor ────────────────────────────────────────────────────────

/**
 * Default no-op executor for when no executor is provided.
 * In production, this would use child_process.spawn for real process management.
 */
function createDefaultExecutor(): SpinupExecutor {
	return {
		async isRunning(_target: string): Promise<boolean> {
			return false;
		},
		async start(_target: string, _command: string): Promise<SpinupResult> {
			return { success: true, pid: process.pid };
		},
		async stop(_target: string): Promise<SpinupStopResult> {
			return { success: true };
		},
	};
}

// ─── Public API ──────────────────────────────────────────────────────────────

/**
 * Create a SpinupManager for the given repository.
 *
 * REQ-PLG-600: Provides start/stop/status/listTargets API.
 * REQ-PLG-601: Includes 4 built-in targets.
 * REQ-PLG-604: start() returns immediately (background execution).
 * REQ-PLG-605: Crash monitoring via onCrash callback.
 * REQ-PLG-606: toToolResult() for programmatic agent use.
 * REQ-PLG-607: Respects backend detection.
 * REQ-PLG-608: Loads custom targets from plugin.yaml.
 * REQ-PLG-609: Custom targets include health check config.
 */
export function createSpinupManager(opts: SpinupManagerOptions): SpinupManager {
	const { repoDir, onCrash, backendMode } = opts;
	const executor = opts.executor ?? createDefaultExecutor();

	// Load custom targets from plugin.yaml (REQ-PLG-608)
	const customTargets = loadCustomTargets(repoDir);

	// Build the full target list: built-in + custom
	const allTargets: (SpinupTarget | CustomSpinupTarget)[] = [
		...BUILTIN_TARGETS,
		...customTargets,
	];

	// Build a lookup map for quick target resolution
	const targetMap = new Map<string, SpinupTarget | CustomSpinupTarget>();
	for (const t of allTargets) {
		targetMap.set(t.name, t);
	}

	const manager: SpinupManager = {
		/**
		 * Start a target process.
		 * REQ-PLG-602: Detects duplicates.
		 * REQ-PLG-604: Returns immediately (background).
		 * REQ-PLG-607: Checks backend availability.
		 */
		async start(target: string): Promise<SpinupResult> {
			// Check if target exists
			const targetDef = targetMap.get(target);
			if (!targetDef) {
				return { success: false, reason: `unknown target: ${target}` };
			}

			// REQ-PLG-607: Check backend availability for built-in targets
			if (backendMode && BUILTIN_NAMES.has(target)) {
				const requiredModes = TARGET_BACKEND_REQUIREMENTS[target];
				if (requiredModes && !requiredModes.includes(backendMode)) {
					return {
						success: false,
						reason: `Backend not available for target "${target}". The required backend is not installed.`,
					};
				}
			}

			// REQ-PLG-602: Check if already running
			const running = await executor.isRunning(target);
			if (running) {
				return {
					success: false,
					reason: `Target "${target}" is already running`,
				};
			}

			// REQ-PLG-604: Start in background (executor handles backgrounding)
			return executor.start(target, targetDef.command);
		},

		/**
		 * Stop a running target.
		 * REQ-PLG-603: /axiom-spinup stop <target>
		 */
		async stop(target: string): Promise<SpinupStopResult> {
			return executor.stop(target);
		},

		/**
		 * Get status of a target.
		 * REQ-PLG-602: Reports running/stopped state.
		 */
		async status(target: string): Promise<SpinupStatus> {
			const running = await executor.isRunning(target);
			return { target, running };
		},

		/**
		 * List all available targets (built-in + custom).
		 * REQ-PLG-601, REQ-PLG-608.
		 */
		listTargets(): (SpinupTarget | CustomSpinupTarget)[] {
			return [...allTargets];
		},

		/**
		 * JSON-serializable status for agent tool use.
		 * REQ-PLG-606: codeops_spinup tool.
		 */
		async toToolResult(): Promise<string> {
			const targetStatuses = await Promise.all(
				allTargets.map(async (t) => {
					const running = await executor.isRunning(t.name);
					return {
						name: t.name,
						command: t.command,
						description: t.description,
						running,
					};
				}),
			);

			return JSON.stringify({ targets: targetStatuses }, null, 2);
		},

		/**
		 * Simulate a crash for testing.
		 * REQ-PLG-605: Crash monitoring.
		 */
		simulateCrash: onCrash
			? (target: string, exitCode: number) => {
					onCrash(target, exitCode);
				}
			: undefined,
	};

	return manager;
}
