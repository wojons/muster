/**
 * Hook wiring — implements copilot auto-load, session init, compaction
 * survival, and shell env injection using the verified SDK hook
 * compatibility matrix from specs/70-OpenCode-Plugin.md#4.0.
 *
 * Hook mapping (SDK v1.2.27):
 *   REQ-PLG-013: copilot auto-load → experimental.chat.system.transform
 *   REQ-PLG-030: session.created → lazy init (session.created unavailable)
 *   REQ-PLG-031: compaction survival → experimental.session.compacting
 *   REQ-PLG-036: shell env injection → shell.env
 *   REQ-PLG-037: All hooks MUST be non-blocking
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-013, #REQ-PLG-030, #REQ-PLG-031, #REQ-PLG-036
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-013 plan=phase-70-1/task-70-1-5/step-70-1-5-3
 */

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { detectMaturity, MaturityLevel } from "./detect";

// ── Types ───────────────────────────────────────────────────────────────

/** Shared context for hook factory functions */
export interface HookContext {
	directory: string;
	maturityLevel?: MaturityLevel;
	activeWorkItem?: string;
	backend?: string;
	activeLayers?: string[];
}

// ── REQ-PLG-013: Copilot auto-load via experimental.chat.system.transform ──

/**
 * Factory for the copilot system prompt injection hook.
 *
 * For Level 0-2 repos, injects Axiom copilot guidance into the system
 * prompt array. For Level 3+ repos, does nothing (copilot available on
 * demand only).
 *
 * SDK hook: experimental.chat.system.transform
 * Signature: (input: { sessionID, model }, output: { system: string[] }) => void
 */
export function createCopilotSystemHook(ctx: {
	directory: string;
	maturityLevel: MaturityLevel;
}): (
	input: { sessionID?: string; model: Record<string, unknown> },
	output: { system: string[] },
) => Promise<void> {
	return async (_input, output) => {
		try {
			// REQ-PLG-013: auto-load copilot for Level 0-2 only
			if (ctx.maturityLevel >= MaturityLevel.Active) {
				return;
			}

			// REQ-PLG-013a: Level 0 (new repos) gets full copilot skill injection.
			// Level 1+ gets a compact 4-line marker to save tokens.
			if (ctx.maturityLevel === MaturityLevel.New) {
				const copilotInstruction = buildCopilotInstruction(
					ctx.directory,
					ctx.maturityLevel,
				);
				output.system.push(copilotInstruction);
			} else {
				// Level 1+ — lightweight marker only
				const marker = buildCopilotMarker(ctx.directory, ctx.maturityLevel);
				output.system.push(marker);
			}
		} catch {
			// REQ-PLG-037: non-blocking — swallow errors silently
		}
	};
}

/**
 * Build a compact 4-line copilot marker for Level 1+ repos.
 *
 * REQ-PLG-013a: Lightweight marker instead of full skill injection.
 * Saves tokens for experienced users while preserving discoverability.
 */
function buildCopilotMarker(directory: string, level: MaturityLevel): string {
	const layers = _detectActiveLayers(directory);
	const activeWork = _detectActiveWork(directory);
	return [
		`# Axiom active (maturity: ${level})`,
		`# Copilot skill: load .opencode/skills/axiom-copilot/SKILL.md for full guidance`,
		`# Active work: ${activeWork}`,
		`# Active layers: ${layers}`,
	].join("\n");
}

/** Detect which layers are active based on config presence. */
function _detectActiveLayers(directory: string): string {
	const layers: string[] = ["L0", "L1", "L2"];
	const pluginYaml = join(directory, ".axiom", "plugin.yaml");
	if (existsSync(pluginYaml)) {
		layers.push("L5", "L6");
	}
	return layers.join(", ");
}

/** Detect active work item from _current.md. */
function _detectActiveWork(directory: string): string {
	const currentMd = join(
		directory,
		".memory-bank",
		"work-items",
		"_current.md",
	);
	if (existsSync(currentMd)) {
		return "see .memory-bank/work-items/_current.md";
	}
	return "none";
}

/**
 * Build the copilot instruction string based on maturity level and
 * available repo context.
 */
function buildCopilotInstruction(
	directory: string,
	level: MaturityLevel,
): string {
	const parts: string[] = [
		"You are working in a Axiom-managed repository.",
		`Current maturity level: ${level} (${maturityLabel(level)}).`,
	];

	// Try to read the copilot skill if it exists locally
	const copilotSkillPath = join(
		directory,
		".opencode",
		"skills",
		"axiom-copilot",
		"SKILL.md",
	);
	if (existsSync(copilotSkillPath)) {
		try {
			const skillContent = readFileSync(copilotSkillPath, "utf-8");
			// Include a trimmed version to stay within reasonable size
			const trimmed = skillContent.slice(0, 2000);
			parts.push("Axiom copilot skill context:", trimmed);
		} catch {
			// Fall through to default guidance
		}
	}

	if (level === MaturityLevel.New) {
		parts.push(
			"This repo has no Axiom setup yet. Guide the user through /axiom-setup.",
			"Use only Day 1 vocabulary: Spec, Plan, Run, Verify, Release.",
		);
	} else if (level === MaturityLevel.Scaffolded) {
		parts.push(
			"This repo has .opencode/ but no specs yet. Guide the user to create their first spec.",
			"Use only Day 1 vocabulary: Spec, Plan, Run, Verify, Release.",
		);
	} else if (level === MaturityLevel.Specced) {
		parts.push(
			"This repo has specs but no plans or memory bank. Guide the user to create their first plan.",
			"Use only Day 1 vocabulary: Spec, Plan, Run, Verify, Release.",
		);
	}

	return parts.join("\n");
}

/** Map maturity level to human label */
function maturityLabel(level: MaturityLevel): string {
	const labels: Record<MaturityLevel, string> = {
		[MaturityLevel.New]: "New",
		[MaturityLevel.Scaffolded]: "Scaffolded",
		[MaturityLevel.Specced]: "Spec'd",
		[MaturityLevel.Active]: "Active",
		[MaturityLevel.Mature]: "Mature",
	};
	return labels[level] ?? "Unknown";
}

// ── REQ-PLG-030: Session init via lazy init fallback ────────────────────

/** Result of session initialization */
interface SessionInitResult {
	initialized: boolean;
	maturityLevel: MaturityLevel;
	activeLayers: string[];
}

/**
 * Factory for the session init hook.
 *
 * Since session.created is unavailable in SDK v1.2.27, this provides a
 * lazy-init function that can be called on first command.execute.before
 * or chat.message. It is idempotent — repeated calls return cached state.
 */
export function createSessionInitHook(ctx: {
	directory: string;
}): (input: { sessionID: string }) => Promise<SessionInitResult> {
	// Session state cache keyed by sessionID
	const cache = new Map<string, SessionInitResult>();

	return async (input) => {
		try {
			const existing = cache.get(input.sessionID);
			if (existing) {
				return existing;
			}

			const maturity = detectMaturity(ctx.directory);
			let level = maturity.level;

			// The detect module requires non-empty directories, but per
			// REQ-PLG-010 "exists" is sufficient for Level 1. Apply a lenient
			// fallback: if detectMaturity says Level 0 but .opencode/ exists
			// (even empty), promote to Level 1.
			if (
				level === MaturityLevel.New &&
				existsSync(join(ctx.directory, ".opencode"))
			) {
				level = MaturityLevel.Scaffolded;
			}

			const activeLayers = computeActiveLayers(level, maturity.backends.mode);

			const result: SessionInitResult = {
				initialized: true,
				maturityLevel: level,
				activeLayers,
			};

			cache.set(input.sessionID, result);
			return result;
		} catch {
			// REQ-PLG-037: non-blocking — return a safe default
			const fallback: SessionInitResult = {
				initialized: true,
				maturityLevel: MaturityLevel.New,
				activeLayers: ["L0"],
			};
			return fallback;
		}
	};
}

/**
 * Compute active capability layers based on maturity level and backend mode.
 *
 * L0 is always on. L1 requires a backend. L2 is on by default.
 * L3-L6 are deferred per spec release scope.
 */
function computeActiveLayers(
	level: MaturityLevel,
	backendMode: string,
): string[] {
	const layers: string[] = ["L0"];

	if (backendMode !== "spec-only" && level >= MaturityLevel.Active) {
		layers.push("L1");
	}

	// L2 (metrics) is on by default when there's something to measure
	if (level >= MaturityLevel.Scaffolded) {
		layers.push("L2");
	}

	return layers;
}

// ── REQ-PLG-031: Compaction survival via experimental.session.compacting ──

/**
 * Factory for the compaction survival hook.
 *
 * Injects Axiom context into compaction prompts so that maturity level,
 * active work item, spec references, and memory bank pointers survive
 * context compaction.
 *
 * SDK hook: experimental.session.compacting
 * Signature: (input: { sessionID }, output: { context: string[], prompt?: string }) => void
 */
export function createCompactionHook(ctx: {
	directory: string;
	maturityLevel: MaturityLevel;
	activeWorkItem?: string;
}): (
	input: { sessionID: string },
	output: { context: string[]; prompt?: string },
) => Promise<void> {
	return async (_input, output) => {
		try {
			// Inject maturity level
			output.context.push(
				`Axiom maturity level: ${ctx.maturityLevel} (${maturityLabel(ctx.maturityLevel)})`,
			);

			// Inject active work item if provided
			if (ctx.activeWorkItem) {
				output.context.push(`Active Axiom work item: ${ctx.activeWorkItem}`);
			} else {
				// Try to read _current.md for active work item
				const currentPath = join(
					ctx.directory,
					".memory-bank",
					"work-items",
					"_current.md",
				);
				if (existsSync(currentPath)) {
					try {
						const content = readFileSync(currentPath, "utf-8");
						const firstLine = content
							.split("\n")
							.find((l) => l.includes("Active"));
						if (firstLine) {
							output.context.push(
								`Active Axiom work item context: ${firstLine.trim()}`,
							);
						}
					} catch {
						// Ignore read errors
					}
				}
			}

			// Inject spec references if specs/ exists
			const specsDir = join(ctx.directory, "specs");
			if (existsSync(specsDir)) {
				output.context.push(
					"Axiom spec directory exists at specs/. Key files: specs/README.md (inventory), specs/00-PRD.md (product intent).",
				);
			}

			// Inject memory bank pointers if .memory-bank/ exists
			const mbDir = join(ctx.directory, ".memory-bank");
			if (existsSync(mbDir)) {
				output.context.push(
					"Axiom memory bank exists at .memory-bank/. Key files: .memory-bank/_index.md (inventory), .memory-bank/TODO.md (roadmap).",
				);
			}

			// REQ-PLG-031: do NOT replace the default compaction prompt
			// output.prompt remains undefined — we only append context
		} catch {
			// REQ-PLG-037: non-blocking — swallow errors silently
		}
	};
}

// ── REQ-PLG-036: Shell env injection via shell.env ──────────────────────

/**
 * Factory for the shell environment injection hook.
 *
 * Injects Axiom-relevant environment variables into every shell command
 * executed by OpenCode.
 *
 * SDK hook: shell.env
 * Signature: (input: { cwd, sessionID? }, output: { env: Record<string, string> }) => void
 */
export function createShellEnvHook(ctx: {
	directory: string;
	maturityLevel: MaturityLevel;
	backend: string;
	activeLayers?: string[];
}): (
	input: { cwd: string; sessionID?: string },
	output: { env: Record<string, string> },
) => Promise<void> {
	return async (input, output) => {
		try {
			// REQ-PLG-036: inject Axiom environment variables
			output.env.AXIOM_MATURITY_LEVEL = String(ctx.maturityLevel);
			output.env.AXIOM_BACKEND = ctx.backend;
			output.env.AXIOM_REPO_ROOT = ctx.directory;
			output.env.AXIOM_SESSION_ID = input.sessionID ?? "unknown";
			output.env.AXIOM_ACTIVE_LAYERS = (ctx.activeLayers ?? ["L0"]).join(",");
		} catch {
			// REQ-PLG-037: non-blocking — swallow errors silently
		}
	};
}
