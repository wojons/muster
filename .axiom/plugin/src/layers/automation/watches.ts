/**
 * Automation watches engine — triggers on tool invocations matching patterns.
 *
 * REQ-PLG-416: Watches trigger on tool.execute.before or tool.execute.after.
 * REQ-PLG-417: Support tool.read, tool.write, tool.edit, tool.bash, tool.*.
 * REQ-PLG-418: Support glob patterns for file matching.
 * REQ-PLG-419: Support run (non-blocking) and block action types.
 * REQ-PLG-420: Block actions logged; used sparingly for security-critical patterns.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-416 through #REQ-PLG-420
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-416 plan=phase-70-4/task-70-4-3/step-70-4-3-2
 */

import { spawn } from "node:child_process";
import type { WatchEntry } from "../../config-loader";

// ── Types ────────────────────────────────────────────────────────────────

/** A tool invocation event that watches can react to. */
export interface WatchTriggerEvent {
	/** The tool event type (e.g., "tool.read", "tool.write"). */
	type: string;
	/** The file path involved (if applicable). */
	filePath?: string;
	/** The bash command (for tool.bash events). */
	command?: string;
}

/** Result of processing a watch against an event. */
export interface WatchActionResult {
	watchName: string;
	triggered: boolean;
	action?: "run" | "block";
	output?: string;
	blocked?: boolean;
	blockMessage?: string;
	error?: string;
}

/** Block log entry. */
export interface BlockLogEntry {
	watchName: string;
	filePath?: string;
	message: string;
	timestamp: Date;
}

/** Options for creating a watches engine. */
export interface WatchesEngineOptions {
	repoRoot: string;
	defaultTimeoutMs?: number;
}

/** The watches engine interface. */
export interface WatchesEngine {
	/** Process a tool event against a list of watches. */
	processEvent(
		event: WatchTriggerEvent,
		watches: WatchEntry[],
	): Promise<WatchActionResult[]>;

	/** Get the block log (all block actions that have fired). */
	getBlockLog(): BlockLogEntry[];
}

// ── Glob matching ────────────────────────────────────────────────────────

/**
 * Match a file path against a glob pattern.
 *
 * REQ-PLG-418: Supports *, **, and ? glob wildcards.
 * If pattern is undefined, always matches.
 */
export function matchesGlobPattern(
	filePath: string,
	pattern: string | undefined,
): boolean {
	if (pattern === undefined || pattern === "") {
		return true;
	}

	// Convert glob pattern to regex using a segment-by-segment approach
	// to correctly handle **, *, and ? wildcards.
	const segments = pattern.split("/");
	const regexParts: string[] = [];

	for (let i = 0; i < segments.length; i++) {
		const seg = segments[i];
		if (seg === "**") {
			// ** matches zero or more path segments
			// If it's the last segment, match anything remaining
			// If followed by more segments, match zero or more dirs
			if (i === segments.length - 1) {
				regexParts.push(".*");
			} else {
				// Match zero or more directory segments (including none)
				regexParts.push("(?:.+/)?");
				// Skip the trailing slash that will be added below
				continue;
			}
		} else {
			// Escape special regex chars in the segment, then convert * and ?
			const segRegex = seg
				.replace(/[.+^${}()|[\]\\]/g, "\\$&")
				.replace(/\*/g, "[^/]*")
				.replace(/\?/g, "[^/]");
			regexParts.push(segRegex);
		}

		// Add separator between segments (except after the last)
		if (i < segments.length - 1) {
			regexParts.push("/");
		}
	}

	const regexStr = regexParts.join("");
	const regex = new RegExp(`^${regexStr}$`);
	return regex.test(filePath);
}

// ── Trigger matching ─────────────────────────────────────────────────────

/**
 * Check if a watch trigger matches an event type.
 *
 * REQ-PLG-417: tool.* matches any tool event.
 */
export function matchesWatchTrigger(
	watchTrigger: string,
	eventType: string,
): boolean {
	if (watchTrigger === "tool.*") {
		return eventType.startsWith("tool.");
	}
	return watchTrigger === eventType;
}

// ── Shell runner ─────────────────────────────────────────────────────────

function runShellCommand(
	command: string,
	cwd: string,
	timeoutMs: number,
): Promise<{ output: string; exitCode: number }> {
	return new Promise((resolve) => {
		const chunks: Buffer[] = [];
		const child = spawn("sh", ["-c", command], {
			cwd,
			stdio: ["ignore", "pipe", "pipe"],
		});

		child.stdout?.on("data", (chunk: Buffer) => chunks.push(chunk));
		child.stderr?.on("data", (chunk: Buffer) => chunks.push(chunk));

		const timer = setTimeout(() => {
			child.kill("SIGKILL");
		}, timeoutMs);

		child.on("close", (code) => {
			clearTimeout(timer);
			resolve({
				output: Buffer.concat(chunks).toString("utf-8"),
				exitCode: code ?? -1,
			});
		});

		child.on("error", (err) => {
			clearTimeout(timer);
			resolve({ output: err.message, exitCode: -1 });
		});
	});
}

// ── Factory ──────────────────────────────────────────────────────────────

/**
 * Create a watches engine for the given repo root.
 */
export function createWatchesEngine(
	options: WatchesEngineOptions,
): WatchesEngine {
	const { repoRoot } = options;
	const defaultTimeoutMs = options.defaultTimeoutMs ?? 30_000;
	const blockLog: BlockLogEntry[] = [];

	async function processEvent(
		event: WatchTriggerEvent,
		watches: WatchEntry[],
	): Promise<WatchActionResult[]> {
		const results: WatchActionResult[] = [];

		for (const watch of watches) {
			// REQ-PLG-416: check trigger type
			if (!matchesWatchTrigger(watch.trigger, event.type)) {
				results.push({ watchName: watch.name, triggered: false });
				continue;
			}

			// REQ-PLG-418: check file pattern
			if (event.filePath !== undefined && watch.pattern !== undefined) {
				if (!matchesGlobPattern(event.filePath, watch.pattern)) {
					results.push({ watchName: watch.name, triggered: false });
					continue;
				}
			}

			// Determine action type
			const actionType = watch.action === "block" ? "block" : "run";

			if (actionType === "block") {
				// REQ-PLG-419: block action
				const msg =
					watch.message ??
					`File access blocked by watch "${watch.name}" (pattern: ${watch.pattern ?? "*"})`;

				// REQ-PLG-420: log all block actions
				blockLog.push({
					watchName: watch.name,
					filePath: event.filePath,
					message: msg,
					timestamp: new Date(),
				});

				results.push({
					watchName: watch.name,
					triggered: true,
					action: "block",
					blocked: true,
					blockMessage: msg,
				});
			} else if (watch.run) {
				// REQ-PLG-419: run action — execute command (non-blocking in production)
				try {
					const { output } = await runShellCommand(
						watch.run,
						repoRoot,
						defaultTimeoutMs,
					);
					results.push({
						watchName: watch.name,
						triggered: true,
						action: "run",
						output,
					});
				} catch (err) {
					results.push({
						watchName: watch.name,
						triggered: true,
						action: "run",
						output: "",
						error: err instanceof Error ? err.message : String(err),
					});
				}
			} else {
				// Watch triggered but no action defined
				results.push({
					watchName: watch.name,
					triggered: true,
				});
			}
		}

		return results;
	}

	function getBlockLog(): BlockLogEntry[] {
		return [...blockLog];
	}

	return {
		processEvent,
		getBlockLog,
	};
}
