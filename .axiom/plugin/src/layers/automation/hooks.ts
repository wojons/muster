/**
 * Automation hooks engine — executes shell commands and prompt injections on OpenCode lifecycle events.
 *
 * REQ-PLG-410: Execute shell commands in repo root on lifecycle events.
 * REQ-PLG-411: Support on_session_start, on_session_end, on_session_idle,
 *              on_message, on_command, on_error lifecycle events.
 * REQ-PLG-411a: on_session_idle keep-alive via prompt hooks.
 * REQ-PLG-412: Support condition expressions (maturity >= N, backend == X, etc.)
 * REQ-PLG-413: Non-blocking execution — run/cmd hooks run in background.
 * REQ-PLG-413a: prompt hook action type — injects message into OpenCode session;
 *               rate-limited (1/30s per hook name); blocked in on_message.
 * REQ-PLG-414: Configurable timeout (default 30s); kill and log on timeout.
 * REQ-PLG-415: Hook output available via codeops_hook_output tool.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-410 through #REQ-PLG-415, #REQ-PLG-413a
 * axiom:trace work_item=plugin-v2-enhancements-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-413a plan=phase-70-9/task-70-9-3/step-70-9-3-1
 */

import { spawn } from "node:child_process";
import type { HookEntry, HooksConfig } from "../../config-loader";

// ── Types ────────────────────────────────────────────────────────────────

/** Context passed to hook condition evaluation and execution. */
export interface HookContext {
	maturityLevel?: number;
	backend?: string;
	env?: Record<string, string>;
	sessionId?: string;
}

/** Result of a single hook execution. */
export interface HookExecutionResult {
	hookName: string;
	success: boolean;
	output: string;
	exitCode: number;
	timedOut?: boolean;
	skipped?: boolean;
	rateLimited?: boolean;
	error?: string;
	startedAt: Date;
	finishedAt: Date;
	/** True when this was a prompt hook (not a shell command). */
	isPrompt?: boolean;
	/** The prompt message text (only set for prompt hooks). */
	promptMessage?: string;
}

/** A recorded prompt injection. */
export interface PromptInjection {
	hookName: string;
	message: string;
	injectedAt: Date;
}

/** Lifecycle event names supported by the hooks engine. */
export type LifecycleEvent =
	| "on_session_start"
	| "on_session_end"
	| "on_session_idle"
	| "on_message"
	| "on_command"
	| "on_error";

/** Options for creating a hooks engine. */
export interface HooksEngineOptions {
	repoRoot: string;
	defaultTimeoutMs?: number;
	/** Rate-limit window for prompt hooks in ms (default: 30000). */
	promptRateLimitMs?: number;
}

/** The hooks engine interface. */
export interface HooksEngine {
	/** Default timeout in milliseconds (default: 30000). */
	readonly defaultTimeoutMs: number;

	/** Run a single hook synchronously (awaitable). */
	runHook(hook: HookEntry, ctx: HookContext): Promise<HookExecutionResult>;

	/** Fire all hooks for a lifecycle event (awaitable — waits for all). */
	fireEvent(
		event: LifecycleEvent,
		hooks: HooksConfig,
		ctx: HookContext,
	): Promise<HookExecutionResult[]>;

	/** Fire all hooks for a lifecycle event in the background (non-blocking). */
	fireEventNonBlocking(
		event: LifecycleEvent,
		hooks: HooksConfig,
		ctx: HookContext,
	): void;

	/** Get the most recent hook execution result, or null if none. */
	getLastOutput(): HookExecutionResult | null;

	/** Get all hook execution results (history). */
	getOutputHistory(): HookExecutionResult[];

	/** Format hook output as a human-readable string for codeops_hook_output. */
	formatHookOutput(): string;

	/** Get all prompt injections recorded this session. */
	getPromptHistory(): PromptInjection[];
}

// ── Condition evaluation ─────────────────────────────────────────────────

/**
 * Evaluate a hook condition expression.
 *
 * REQ-PLG-412: Supported expressions:
 * - `maturity >= N` — only run at or above maturity level N
 * - `backend == "python"` or `backend == "go"` — only run with specific backend
 * - `env.VAR == "value"` — only run when environment variable matches
 * - Unknown conditions default to true (permissive)
 */
export function evaluateCondition(
	condition: string | undefined,
	ctx: HookContext,
): boolean {
	if (!condition || condition.trim() === "") {
		return true;
	}

	const expr = condition.trim();

	// maturity >= N
	const maturityMatch = expr.match(/^maturity\s*>=\s*(\d+)$/);
	if (maturityMatch) {
		const required = parseInt(maturityMatch[1], 10);
		const actual = ctx.maturityLevel ?? 0;
		return actual >= required;
	}

	// backend == "value" or backend == 'value'
	const backendMatch = expr.match(/^backend\s*==\s*["']([^"']+)["']$/);
	if (backendMatch) {
		const required = backendMatch[1];
		return ctx.backend === required;
	}

	// env.VAR == "value" or env.VAR == 'value'
	const envMatch = expr.match(/^env\.(\w+)\s*==\s*["']([^"']+)["']$/);
	if (envMatch) {
		const varName = envMatch[1];
		const required = envMatch[2];
		const actual = ctx.env?.[varName];
		return actual === required;
	}

	// Unknown condition — permissive (do not block execution)
	return true;
}

// ── Hook runner ──────────────────────────────────────────────────────────

/**
 * Execute a single shell command with timeout.
 * Returns stdout+stderr combined, exit code, and timeout flag.
 */
function runShellCommand(
	command: string,
	cwd: string,
	timeoutMs: number,
): Promise<{ output: string; exitCode: number; timedOut: boolean }> {
	return new Promise((resolve) => {
		const chunks: Buffer[] = [];
		let timedOut = false;

		const child = spawn("sh", ["-c", command], {
			cwd,
			stdio: ["ignore", "pipe", "pipe"],
			detached: true, // create process group so we can kill the entire tree
		});

		child.stdout?.on("data", (chunk: Buffer) => chunks.push(chunk));
		child.stderr?.on("data", (chunk: Buffer) => chunks.push(chunk));

		const timer = setTimeout(() => {
			timedOut = true;
			// Kill the entire process group (negative PID) to ensure child
			// processes like 'sleep' are also terminated. Without this,
			// SIGKILL only kills the shell but leaves children running,
			// and the 'close' event doesn't fire until pipes drain.
			try {
				if (child.pid) process.kill(-child.pid, "SIGKILL");
			} catch {
				child.kill("SIGKILL");
			}
		}, timeoutMs);

		child.on("close", (code) => {
			clearTimeout(timer);
			const output = Buffer.concat(chunks).toString("utf-8");
			resolve({
				output,
				exitCode: timedOut ? -1 : (code ?? -1),
				timedOut,
			});
		});

		child.on("error", (err) => {
			clearTimeout(timer);
			resolve({
				output: err.message,
				exitCode: -1,
				timedOut: false,
			});
		});
	});
}

// ── Factory ──────────────────────────────────────────────────────────────

/**
 * Create a hooks engine for the given repo root.
 *
 * REQ-PLG-410: Hooks execute in repoRoot.
 * REQ-PLG-413a: prompt hooks inject messages; rate-limited 1/30s per hook name.
 * REQ-PLG-414: Default timeout is 30 seconds.
 */
export function createHooksEngine(options: HooksEngineOptions): HooksEngine {
	const { repoRoot } = options;
	const defaultTimeoutMs = options.defaultTimeoutMs ?? 30_000;
	const promptRateLimitMs = options.promptRateLimitMs ?? 30_000;
	const history: HookExecutionResult[] = [];
	const promptHistory: PromptInjection[] = [];
	// Track last prompt injection time per hook name for rate-limiting
	const promptLastFired = new Map<string, number>();

	async function runHook(
		hook: HookEntry,
		ctx: HookContext,
	): Promise<HookExecutionResult> {
		const startedAt = new Date();
		// Cast to extended type to access prompt/cmd fields
		const h = hook as HookEntry & { prompt?: string; cmd?: string };

		// REQ-PLG-412: evaluate condition before running
		if (hook.condition !== undefined) {
			const conditionMet = evaluateCondition(hook.condition, ctx);
			if (!conditionMet) {
				const result: HookExecutionResult = {
					hookName: hook.name,
					success: false,
					output: "",
					exitCode: -1,
					skipped: true,
					startedAt,
					finishedAt: new Date(),
				};
				history.push(result);
				return result;
			}
		}

		// REQ-PLG-413a: handle prompt hook action type
		if (h.prompt !== undefined) {
			return _runPromptHook(h.name, h.prompt, startedAt);
		}

		// REQ-PLG-413a: cmd is an alias for run
		const command = h.cmd ?? hook.run;

		// REQ-PLG-410: run in repo root
		const { output, exitCode, timedOut } = await runShellCommand(
			command,
			repoRoot,
			defaultTimeoutMs,
		);

		const finishedAt = new Date();
		const result: HookExecutionResult = {
			hookName: hook.name,
			success: !timedOut && exitCode === 0,
			output,
			exitCode,
			timedOut: timedOut || undefined,
			startedAt,
			finishedAt,
		};

		// REQ-PLG-415: store in history
		history.push(result);
		return result;
	}

	function _runPromptHook(
		hookName: string,
		message: string,
		startedAt: Date,
	): HookExecutionResult {
		const now = Date.now();
		const lastFired = promptLastFired.get(hookName);

		// REQ-PLG-413a: rate-limit — max 1 injection per promptRateLimitMs per hook name
		if (lastFired !== undefined && now - lastFired < promptRateLimitMs) {
			const result: HookExecutionResult = {
				hookName,
				success: false,
				output: "",
				exitCode: -1,
				rateLimited: true,
				isPrompt: true,
				promptMessage: message,
				startedAt,
				finishedAt: new Date(),
			};
			history.push(result);
			return result;
		}

		// Record the injection
		promptLastFired.set(hookName, now);
		promptHistory.push({ hookName, message, injectedAt: new Date() });

		const result: HookExecutionResult = {
			hookName,
			success: true,
			output: "",
			exitCode: 0,
			isPrompt: true,
			promptMessage: message,
			startedAt,
			finishedAt: new Date(),
		};
		history.push(result);
		return result;
	}

	async function fireEvent(
		event: LifecycleEvent,
		hooks: HooksConfig,
		ctx: HookContext,
	): Promise<HookExecutionResult[]> {
		const eventHooks = hooks[event] ?? [];

		// REQ-PLG-413a: reject prompt hooks in on_message (infinite loop prevention)
		if (event === "on_message") {
			for (const hook of eventHooks) {
				const h = hook as HookEntry & { prompt?: string };
				if (h.prompt !== undefined) {
					throw new Error(
						`Hook "${hook.name}": prompt hooks are not allowed in on_message events ` +
							`(would create an infinite loop). Use on_session_idle instead.`,
					);
				}
			}
		}

		const results: HookExecutionResult[] = [];
		for (const hook of eventHooks) {
			const result = await runHook(hook, ctx);
			results.push(result);
		}
		return results;
	}

	function fireEventNonBlocking(
		event: LifecycleEvent,
		hooks: HooksConfig,
		ctx: HookContext,
	): void {
		// REQ-PLG-413: fire in background, do not block
		void fireEvent(event, hooks, ctx).catch(() => {
			// Swallow errors — non-blocking per REQ-PLG-037
		});
	}

	function getLastOutput(): HookExecutionResult | null {
		if (history.length === 0) return null;
		return history[history.length - 1];
	}

	function getOutputHistory(): HookExecutionResult[] {
		return [...history];
	}

	function getPromptHistory(): PromptInjection[] {
		return [...promptHistory];
	}

	function formatHookOutput(): string {
		if (history.length === 0) {
			return "No hooks have run in this session.";
		}
		const last = history[history.length - 1];
		const status = last.skipped
			? "skipped"
			: last.rateLimited
				? "rate-limited"
				: last.isPrompt
					? "prompt-injected"
					: last.timedOut
						? "timed out"
						: last.success
							? "success"
							: "failed";
		const lines = [`Hook: ${last.hookName}`, `Status: ${status}`];
		if (last.isPrompt && last.promptMessage) {
			lines.push(`Prompt: ${last.promptMessage}`);
		} else {
			lines.push(`Exit code: ${last.exitCode}`);
			if (last.output.trim()) {
				lines.push(`Output:\n${last.output.trim()}`);
			}
		}
		lines.push(`Started: ${last.startedAt.toISOString()}`);
		lines.push(`Finished: ${last.finishedAt.toISOString()}`);
		if (history.length > 1) {
			lines.push(`\n(${history.length} hooks run this session)`);
		}
		return lines.join("\n");
	}

	return {
		defaultTimeoutMs,
		runHook,
		fireEvent,
		fireEventNonBlocking,
		getLastOutput,
		getOutputHistory,
		formatHookOutput,
		getPromptHistory,
	};
}
