/**
 * Custom tool registry — registers and executes L6 custom tools from config.
 *
 * REQ-PLG-500: Register custom tools from .axiom/plugin.yaml.
 * REQ-PLG-501: Tools support name, description, run, parameters.
 * REQ-PLG-502: Capture stdout+stderr and return as tool result.
 * REQ-PLG-503: Configurable timeout (default 60s).
 * REQ-PLG-508: Total tools must not exceed 20.
 * REQ-PLG-509: Tool registration is idempotent.
 * REQ-PLG-720: Config must be approved before automation runs.
 * REQ-PLG-721: Approved configs tracked by SHA-256 hash in .axiom/.plugin-trust.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-500 through #REQ-PLG-509,
 *       #REQ-PLG-720, #REQ-PLG-721
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-500 plan=phase-70-4/task-70-4-5/step-70-4-5-2
 */

import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ToolEntry } from "../../config-loader";

// ── Types ────────────────────────────────────────────────────────────────

/** A resolved tool definition ready for registration. */
export interface ToolDefinition {
	name: string;
	description: string;
	run: string;
	parameters?: Record<
		string,
		{ type?: string; description?: string; default?: unknown }
	>;
}

/** Result of executing a custom tool. */
export interface ToolExecutionResult {
	toolName: string;
	success: boolean;
	output: string;
	exitCode: number;
	error?: string;
}

/** Result of checking the tool count limit. */
export interface ToolCountCheckResult {
	ok: boolean;
	total: number;
	warning?: string;
}

/** Options for creating a tool registry. */
export interface ToolRegistryOptions {
	repoRoot: string;
	defaultTimeoutMs?: number;
}

/** The tool registry interface. */
export interface ToolRegistry {
	readonly defaultTimeoutMs: number;

	/** Execute a custom tool with the given parameters. */
	executeTool(
		tool: ToolEntry,
		params: Record<string, unknown>,
	): Promise<ToolExecutionResult>;

	/** Check if total tool count (built-in + custom) exceeds 20. */
	checkToolCountLimit(
		customTools: ToolEntry[],
		builtInCount: number,
	): ToolCountCheckResult;
}

// ── Tool definition builder ──────────────────────────────────────────────

/**
 * Build tool definitions from a list of ToolEntry objects.
 * Deduplicates by name (REQ-PLG-509).
 */
export function buildToolDefinitions(tools: ToolEntry[]): ToolDefinition[] {
	const seen = new Set<string>();
	const defs: ToolDefinition[] = [];

	for (const tool of tools) {
		if (seen.has(tool.name)) {
			continue; // REQ-PLG-509: deduplicate
		}
		seen.add(tool.name);
		defs.push({
			name: tool.name,
			description: tool.description ?? tool.name,
			run: tool.run,
			parameters: tool.parameters,
		});
	}

	return defs;
}

// ── Config trust system ──────────────────────────────────────────────────

const TRUST_FILE = ".axiom/.plugin-trust";

/**
 * Compute SHA-256 hash of config file content.
 * REQ-PLG-721: Approved configs tracked by content hash.
 */
export function computeConfigHash(content: string): string {
	return createHash("sha256").update(content).digest("hex");
}

/**
 * Check if a config file content has been approved.
 * REQ-PLG-720: Config must be approved before automation runs.
 * REQ-PLG-721: Approval tracked by SHA-256 hash in .axiom/.plugin-trust.
 */
export function isConfigApproved(repoRoot: string, content: string): boolean {
	const trustPath = join(repoRoot, TRUST_FILE);
	if (!existsSync(trustPath)) {
		return false;
	}
	const stored = readFileSync(trustPath, "utf-8").trim();
	const expected = computeConfigHash(content);
	return stored.includes(expected);
}

/**
 * Approve a config file by writing its hash to .axiom/.plugin-trust.
 * REQ-PLG-721: Overwrites previous approval when config changes.
 */
export function approveConfig(repoRoot: string, content: string): void {
	const trustDir = join(repoRoot, ".axiom");
	mkdirSync(trustDir, { recursive: true });
	const trustPath = join(repoRoot, TRUST_FILE);
	const hash = computeConfigHash(content);
	writeFileSync(trustPath, `sha256:${hash}\n`, "utf-8");
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

// ── Parameter substitution ───────────────────────────────────────────────

function substituteParams(
	command: string,
	params: Record<string, unknown>,
): string {
	let result = command;
	for (const [key, value] of Object.entries(params)) {
		result = result.replace(
			new RegExp(`\\$${key}\\b`, "g"),
			String(value ?? ""),
		);
	}
	return result;
}

// ── Factory ──────────────────────────────────────────────────────────────

/**
 * Create a tool registry for the given repo root.
 */
export function createToolRegistry(options: ToolRegistryOptions): ToolRegistry {
	const { repoRoot } = options;
	const defaultTimeoutMs = options.defaultTimeoutMs ?? 60_000;

	async function executeTool(
		tool: ToolEntry,
		params: Record<string, unknown>,
	): Promise<ToolExecutionResult> {
		// REQ-PLG-501: substitute parameters in command
		const command = substituteParams(tool.run, params);

		// REQ-PLG-502: capture stdout+stderr
		const { output, exitCode } = await runShellCommand(
			command,
			repoRoot,
			defaultTimeoutMs,
		);

		return {
			toolName: tool.name,
			success: exitCode === 0,
			output,
			exitCode,
		};
	}

	function checkToolCountLimit(
		customTools: ToolEntry[],
		builtInCount: number,
	): ToolCountCheckResult {
		const total = builtInCount + customTools.length;
		if (total > 20) {
			return {
				ok: false,
				total,
				warning: `Total tool count (${total}) exceeds the maximum of 20. Built-in tools take priority, then config tools. Some custom tools may not be registered. Consider reducing the number of custom tools in .axiom/plugin.yaml.`,
			};
		}
		return { ok: true, total };
	}

	return {
		defaultTimeoutMs,
		executeTool,
		checkToolCountLimit,
	};
}
