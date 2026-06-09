/**
 * Automation scripts engine — runs named scripts from .axiom/plugin.yaml.
 *
 * REQ-PLG-421: /axiom-script command lists and runs named scripts.
 * REQ-PLG-422: Scripts support positional ($1, $2) and named ($arg_name) args.
 * REQ-PLG-423: requires_confirmation: true prompts before execution.
 * REQ-PLG-424: Script output captured and returned as tool output.
 * REQ-PLG-425: Scripts NOT auto-registered as tools (avoid naming collisions).
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-421 through #REQ-PLG-425
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-421 plan=phase-70-4/task-70-4-4/step-70-4-4-2
 */

import { spawn } from "node:child_process";
import type { ScriptEntry } from "../../config-loader";

// ── Types ────────────────────────────────────────────────────────────────

/** Result of running a script. */
export interface ScriptRunResult {
	scriptName: string;
	success: boolean;
	output: string;
	exitCode: number;
	error?: string;
	needsConfirmation?: boolean;
	confirmationPrompt?: string;
}

/** Script listing entry. */
export interface ScriptListEntry {
	name: string;
	description?: string;
	requiresConfirmation: boolean;
}

/** Options for runScript. */
export interface RunScriptOptions {
	confirmed?: boolean;
}

/** Options for creating a scripts engine. */
export interface ScriptsEngineOptions {
	repoRoot: string;
	defaultTimeoutMs?: number;
}

/** The scripts engine interface. */
export interface ScriptsEngine {
	/** List all scripts with their names and descriptions. */
	listScripts(scripts: Record<string, ScriptEntry>): ScriptListEntry[];

	/** Run a named script with optional positional and named args. */
	runScript(
		name: string,
		scripts: Record<string, ScriptEntry>,
		positionalArgs: string[],
		namedArgs?: Record<string, string>,
		options?: RunScriptOptions,
	): Promise<ScriptRunResult>;

	/** Format the script list as a human-readable string. */
	formatScriptList(scripts: Record<string, ScriptEntry>): string;

	/** Check if a script requires confirmation before execution. */
	requiresConfirmation(script: ScriptEntry): boolean;

	/** Get the confirmation prompt for a script. */
	getConfirmationPrompt(name: string, script: ScriptEntry): string;

	/**
	 * Get tool names for scripts.
	 * REQ-PLG-425: Always returns [] — scripts are NOT auto-registered as tools.
	 */
	getToolNames(scripts: Record<string, ScriptEntry>): string[];
}

// ── Argument substitution ────────────────────────────────────────────────

/**
 * Substitute positional ($1, $2, ...) and named ($arg_name) arguments
 * in a command string.
 *
 * REQ-PLG-422: Scripts support positional and named argument substitution.
 */
export function substituteArgs(
	command: string,
	positionalArgs: string[],
	namedArgs: Record<string, string> = {},
): string {
	let result = command;

	// Replace named args first ($arg_name) — must come before positional
	// to avoid partial matches like $1 in $1st
	for (const [key, value] of Object.entries(namedArgs)) {
		result = result.replace(new RegExp(`\\$${key}\\b`, "g"), value);
	}

	// Replace positional args ($1, $2, ...)
	for (let i = 0; i < positionalArgs.length; i++) {
		result = result.replace(
			new RegExp(`\\$${i + 1}\\b`, "g"),
			positionalArgs[i],
		);
	}

	return result;
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
 * Create a scripts engine for the given repo root.
 */
export function createScriptsEngine(
	options: ScriptsEngineOptions,
): ScriptsEngine {
	const { repoRoot } = options;
	const defaultTimeoutMs = options.defaultTimeoutMs ?? 60_000;

	function listScripts(
		scripts: Record<string, ScriptEntry>,
	): ScriptListEntry[] {
		return Object.entries(scripts).map(([name, entry]) => ({
			name,
			description: entry.description,
			requiresConfirmation: entry.requires_confirmation === true,
		}));
	}

	async function runScript(
		name: string,
		scripts: Record<string, ScriptEntry>,
		positionalArgs: string[],
		namedArgs: Record<string, string> = {},
		options: RunScriptOptions = {},
	): Promise<ScriptRunResult> {
		const script = scripts[name];

		if (!script) {
			return {
				scriptName: name,
				success: false,
				output: "",
				exitCode: -1,
				error: `Script "${name}" not found. Available scripts: ${Object.keys(scripts).join(", ") || "(none)"}`,
			};
		}

		// REQ-PLG-423: check confirmation requirement
		if (script.requires_confirmation && !options.confirmed) {
			const prompt = getConfirmationPrompt(name, script);
			return {
				scriptName: name,
				success: false,
				output: "",
				exitCode: -1,
				needsConfirmation: true,
				confirmationPrompt: prompt,
			};
		}

		// REQ-PLG-422: substitute args
		const command = substituteArgs(script.run, positionalArgs, namedArgs);

		// REQ-PLG-424: capture output
		const { output, exitCode } = await runShellCommand(
			command,
			repoRoot,
			defaultTimeoutMs,
		);

		return {
			scriptName: name,
			success: exitCode === 0,
			output,
			exitCode,
		};
	}

	function formatScriptList(scripts: Record<string, ScriptEntry>): string {
		const entries = Object.entries(scripts);
		if (entries.length === 0) {
			return "No scripts configured. Add scripts to .axiom/plugin.yaml.";
		}
		const lines = ["Available scripts:", ""];
		for (const [name, entry] of entries) {
			const confirmation = entry.requires_confirmation
				? " [requires confirmation]"
				: "";
			const desc = entry.description ? ` — ${entry.description}` : "";
			lines.push(`  /axiom-script ${name}${desc}${confirmation}`);
		}
		return lines.join("\n");
	}

	function requiresConfirmation(script: ScriptEntry): boolean {
		return script.requires_confirmation === true;
	}

	function getConfirmationPrompt(name: string, script: ScriptEntry): string {
		const desc = script.description
			? `This will ${script.description.toLowerCase()}.`
			: `This will run: ${script.run}`;
		return `Run ${name}? ${desc} [yes/no]`;
	}

	function getToolNames(_scripts: Record<string, ScriptEntry>): string[] {
		// REQ-PLG-425: Scripts are NOT auto-registered as tools
		return [];
	}

	return {
		listScripts,
		runScript,
		formatScriptList,
		requiresConfirmation,
		getConfirmationPrompt,
		getToolNames,
	};
}
