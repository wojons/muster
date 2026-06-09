/**
 * Script-based source adapter for the Listen/Notify Plugin.
 *
 * REQ-LNP-020: Script-based listener support.
 * REQ-LNP-021: Script listener configuration parameters.
 * REQ-LNP-022: Script security validation and allowlist.
 * REQ-LNP-023: Script execution environment constraints.
 * REQ-LNP-025: Capture stdout and stderr.
 * REQ-LNP-026: Poll interval support.
 * REQ-LNP-027: Timeout handling — kill process on timeout.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#3.3
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-3/task-3.3
 */

import { spawn } from "node:child_process";
import { resolve } from "node:path";
import type { ScriptListenerConfig } from "../../../config-loader";
import type { EventCallback, ListenerAdapter } from "../types";
import {
	emitListenerLog,
	generateEventId,
	truncatePayload,
} from "../types";

// ── Security Allowlist ───────────────────────────────────────────────────

/**
 * Blocked command patterns per REQ-LNP-022.
 * These patterns are checked against the full command string.
 */
const BLOCKED_PATTERNS: RegExp[] = [
	/rm\s+-rf\s+\//,                    // recursive delete from root
	/rm\s+--no-preserve-root/,          // force delete root
	/curl\s+.*https?:\/\//,             // curl to arbitrary URLs (allowlist needed)
	/wget\s+.*https?:\/\//,             // wget to arbitrary URLs
	/\bssh\b/,                          // ssh connections
	/\bscp\b/,                          // scp file transfers
	/\$AWS_ACCESS_KEY_ID/,              // credential env var access
	/\$AWS_SECRET_ACCESS_KEY/,          // credential env var access
	/\$GITHUB_TOKEN/,                   // credential env var access
	/\$AXIOM_API_TOKEN/,              // credential env var access
	/process\.env\.(AWS|GITHUB|TOKEN)/i, // JS credential access
];

/** Credential environment variables that MUST NOT be passed to scripts (REQ-LNP-023). */
const BLOCKED_ENV_VARS = [
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"GITHUB_TOKEN",
	"AXIOM_API_TOKEN",
	"NPM_TOKEN",
	"DOCKER_PASSWORD",
	"DATABASE_URL",
	"DATABASE_PASSWORD",
];

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating a script adapter. */
export interface ScriptAdapterOptions {
	listenerName: string;
	config: ScriptListenerConfig;
	/** Repo root for working directory constraint (REQ-LNP-023). */
	repoRoot: string;
}

/** Result of script security validation. */
export interface ScriptValidationResult {
	valid: boolean;
	reason?: string;
}

// ── Security Validation ──────────────────────────────────────────────────

/**
 * Validate a script command against the security allowlist.
 * REQ-LNP-022: Scripts containing blocked commands MUST be rejected.
 */
export function validateScript(
	command: string,
	args: string[],
): ScriptValidationResult {
	const fullCommand = [command, ...args].join(" ");

	for (const pattern of BLOCKED_PATTERNS) {
		if (pattern.test(fullCommand)) {
			return {
				valid: false,
				reason: `Script contains blocked pattern: ${pattern.source}`,
			};
		}
	}

	return { valid: true };
}

/**
 * Build a sanitized environment for script execution.
 * REQ-LNP-023: No credential environment variables passed to scripts.
 */
export function buildSafeEnv(): Record<string, string> {
	const env: Record<string, string> = {};
	for (const [key, value] of Object.entries(process.env)) {
		if (value === undefined) continue;
		if (BLOCKED_ENV_VARS.includes(key)) continue;
		env[key] = value;
	}
	// Mark the process as a Axiom listener script (REQ-LNP-010 edge case 9)
	env.AXIOM_LISTENER = "true";
	return env;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create a script-based listener adapter.
 * REQ-LNP-020: Executes a user-defined command and captures output as events.
 */
export function createScriptAdapter(
	options: ScriptAdapterOptions,
): ListenerAdapter {
	const { listenerName, config, repoRoot } = options;
	const {
		command,
		args,
		timeout_seconds = 3600,
		working_directory,
		poll_interval_seconds,
	} = config;

	let running = false;
	let healthy = false;
	let statusDesc = "stopped";
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	// REQ-LNP-023: Working directory MUST be locked to repo root
	const workDir = working_directory
		? resolve(repoRoot, working_directory)
		: repoRoot;

	// Ensure working directory is within repo root
	if (!workDir.startsWith(repoRoot)) {
		statusDesc = `error: working_directory escapes repo root`;
		healthy = false;
	}

	async function runScript(onEvent: EventCallback): Promise<void> {
		// REQ-LNP-022: Validate script before execution
		const validation = validateScript(command, args);
		if (!validation.valid) {
			emitListenerLog({
				event: "listener.agent_script_rejected",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				reason: validation.reason,
			});
			healthy = false;
			statusDesc = `rejected: ${validation.reason}`;
			return;
		}

		return new Promise<void>((resolve) => {
			// REQ-LNP-023: Sanitized environment (no credentials)
			const safeEnv = buildSafeEnv();

			const child = spawn(command, args, {
				cwd: workDir,
				env: safeEnv,
				stdio: ["ignore", "pipe", "pipe"],
			});
			// unref() allows the process to exit even if the child is still running
			// (important for test environments to avoid hanging after tests complete).
			child.unref();

			let stdout = "";
			let stderr = "";
			let timedOut = false;

			// REQ-LNP-027: Kill process on timeout
			const timeoutHandle = setTimeout(() => {
				timedOut = true;
				child.kill("SIGTERM");
				const killHandle = setTimeout(() => child.kill("SIGKILL"), 5000);
				if (killHandle && typeof killHandle.unref === "function") killHandle.unref();
			}, timeout_seconds * 1000);
			if (timeoutHandle && typeof timeoutHandle.unref === "function") timeoutHandle.unref();

			// REQ-LNP-025: Capture stdout and stderr
			child.stdout?.on("data", (chunk: Buffer) => {
				stdout += chunk.toString();
			});
			child.stderr?.on("data", (chunk: Buffer) => {
				stderr += chunk.toString();
			});

			child.on("close", async (exitCode) => {
				clearTimeout(timeoutHandle);

				if (timedOut) {
					// REQ-LNP-027: Emit timeout notification
					emitListenerLog({
						event: "listener.stopped",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						reason: "timeout",
					});
					// Inject timeout notification into agent context
					const timeoutEvent = {
						id: generateEventId(),
						listener_name: listenerName,
						source_type: "script" as const,
						source_ref: `${command} ${args.join(" ")}`,
						received_at: new Date().toISOString(),
						payload: {
							type: "listener.timeout",
							message: `Script listener "${listenerName}" exceeded timeout of ${timeout_seconds}s and was killed`,
							command,
							args,
						},
						delivery: {
							mode: "interruptive" as const,
							status: "pending" as const,
							attempts: 0,
							last_attempt_at: null,
						},
					};
					await onEvent(timeoutEvent).catch(() => {});
					resolve();
					return;
				}

				if (!stdout && !stderr) {
					resolve();
					return;
				}

				// Build event from script output
				let payload: unknown;
				try {
					payload = JSON.parse(stdout);
				} catch {
					payload = {
						stdout: stdout.trim(),
						stderr: stderr.trim(),
						exit_code: exitCode,
					};
				}

				const { payload: truncated, truncated: wasTruncated } =
					truncatePayload(payload);

				const event = {
					id: generateEventId(),
					listener_name: listenerName,
					source_type: "script" as const,
					source_ref: `${command} ${args.join(" ")}`,
					received_at: new Date().toISOString(),
					payload: truncated,
					truncated: wasTruncated,
					delivery: {
						mode: "non-interruptive" as const,
						status: "pending" as const,
						attempts: 0,
						last_attempt_at: null,
					},
				};

				emitListenerLog({
					event: "listener.event_received",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					event_id: event.id,
					truncated: wasTruncated,
				});

				await onEvent(event).catch((err) => {
					const msg = err instanceof Error ? err.message : String(err);
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						error: `Event processing failed: ${msg}`,
					});
				});

				resolve();
			});

			child.on("error", (err) => {
				clearTimeout(timeoutHandle);
				healthy = false;
				statusDesc = `error: ${err.message}`;
				emitListenerLog({
					event: "listener.error",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					error: err.message,
				});
				resolve();
			});
		});
	}

	function schedulePoll(onEvent: EventCallback): void {
		if (!running || !poll_interval_seconds) return;
		pollTimer = setTimeout(async () => {
			await runScript(onEvent);
			schedulePoll(onEvent);
		}, poll_interval_seconds * 1000);
	}

	return {
		async start(onEvent: EventCallback): Promise<void> {
			if (running) return;

			// Check working directory constraint
			if (!workDir.startsWith(repoRoot)) {
				emitListenerLog({
					event: "listener.error",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					error: `working_directory "${working_directory}" escapes repo root — listener rejected`,
				});
				return;
			}

			running = true;
			healthy = true;
			statusDesc = "starting";

			emitListenerLog({
				event: "listener.started",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
			});

			// Run script immediately
			await runScript(onEvent);

			if (running) {
				statusDesc = "running";
				// REQ-LNP-026: Poll interval for continuous scripts
				if (poll_interval_seconds) {
					schedulePoll(onEvent);
				}
			}
		},

		async stop(): Promise<void> {
			running = false;
			healthy = false;
			if (pollTimer) {
				clearTimeout(pollTimer);
				pollTimer = null;
			}
			statusDesc = "stopped";
			emitListenerLog({
				event: "listener.stopped",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
			});
		},

		isHealthy(): boolean {
			return healthy && running;
		},

		statusDescription(): string {
			return statusDesc;
		},
	};
}
