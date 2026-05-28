/**
 * Integration configuration — scaffolds and manages .axiom/axiom.config.yaml
 * for Jira project key, GitHub settings, and repo-level configuration.
 *
 * REQ-PLG-046: Scaffold .axiom/axiom.config.yaml with repo_id, jira.project_key,
 *              confidence weights/thresholds. Idempotent — never overwrites existing.
 * REQ-PLG-047: Integration configuration step — validate Jira keys, skip support,
 *              multi-repo independence, GitHub default_base_branch offer.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-046, #REQ-PLG-047
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-046 plan=phase-70-1/task-70-1-8/step-70-1-8-2
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

// ── Types ───────────────────────────────────────────────────────────────

/** Jira integration section of the config. */
export interface JiraConfig {
	project_key: string | null;
}

/** GitHub integration section of the config. */
export interface GithubConfig {
	default_base_branch?: string;
}

/** Confidence weights section. */
export interface ConfidenceWeights {
	test_signal: number;
	trace_completeness: number;
	spec_alignment: number;
}

/** Confidence thresholds section. */
export interface ConfidenceThresholds {
	pass: number;
	warn: number;
}

/** Confidence section of the config. */
export interface ConfidenceConfig {
	weights: ConfidenceWeights;
	thresholds: ConfidenceThresholds;
}

/** Top-level Axiom config shape (.axiom/axiom.config.yaml). */
export interface CodeOpsConfig {
	repo_id?: string;
	jira?: JiraConfig;
	github?: GithubConfig;
	confidence?: ConfidenceConfig;
	[key: string]: unknown;
}

/** Result of configureJiraProjectKey(). */
export interface JiraConfigResult {
	/** Whether a Jira key was successfully configured */
	configured: boolean;
	/** The project key that was set, or null if skipped */
	projectKey: string | null;
	/** Whether the operation was skipped (existing config not overwritten) */
	skipped?: boolean;
	/** Whether an existing config with null key was updated */
	updatedExisting?: boolean;
	/** Validation error message if the key was invalid */
	validationError?: string;
}

/** Options for configureGithubSettings(). */
export interface GithubSettingsOptions {
	defaultBranch: string | null;
}

/** Result of configureGithubSettings(). */
export interface GithubConfigResult {
	/** Whether GitHub settings were configured */
	configured: boolean;
	/** The default base branch that was set */
	defaultBaseBranch?: string;
	/** Whether the operation was skipped */
	skipped?: boolean;
	/** Reason for skipping */
	reason?: string;
}

// ── Constants ───────────────────────────────────────────────────────────

const CONFIG_RELATIVE_PATH = join(".axiom", "axiom.config.yaml");

const DEFAULT_CONFIDENCE: ConfidenceConfig = {
	weights: {
		test_signal: 0.4,
		trace_completeness: 0.3,
		spec_alignment: 0.3,
	},
	thresholds: {
		pass: 80,
		warn: 60,
	},
};

// ── Minimal YAML helpers ────────────────────────────────────────────────
// The config format is simple enough that we avoid a full YAML library.
// We handle: scalars, one-level nested objects, and preserve unknown keys.

/**
 * Serialize a CodeOpsConfig to YAML string.
 * Produces clean, human-readable YAML without a library dependency.
 */
function serializeConfigYaml(config: CodeOpsConfig): string {
	const lines: string[] = [];

	// repo_id
	if (config.repo_id !== undefined) {
		lines.push(`repo_id: ${yamlScalar(config.repo_id)}`);
	}

	// jira section
	if (config.jira !== undefined) {
		lines.push("jira:");
		lines.push(`  project_key: ${yamlScalar(config.jira.project_key)}`);
	}

	// github section
	if (config.github !== undefined) {
		lines.push("github:");
		if (config.github.default_base_branch !== undefined) {
			lines.push(
				`  default_base_branch: ${yamlScalar(config.github.default_base_branch)}`,
			);
		}
	}

	// confidence section
	if (config.confidence !== undefined) {
		lines.push("confidence:");
		lines.push("  weights:");
		lines.push(`    test_signal: ${config.confidence.weights.test_signal}`);
		lines.push(
			`    trace_completeness: ${config.confidence.weights.trace_completeness}`,
		);
		lines.push(
			`    spec_alignment: ${config.confidence.weights.spec_alignment}`,
		);
		lines.push("  thresholds:");
		lines.push(`    pass: ${config.confidence.thresholds.pass}`);
		lines.push(`    warn: ${config.confidence.thresholds.warn}`);
	}

	// Preserve any unknown top-level keys
	for (const key of Object.keys(config)) {
		if (["repo_id", "jira", "github", "confidence"].includes(key)) continue;
		const value = config[key];
		if (typeof value === "object" && value !== null && !Array.isArray(value)) {
			lines.push(`${key}:`);
			for (const [subKey, subVal] of Object.entries(
				value as Record<string, unknown>,
			)) {
				lines.push(`  ${subKey}: ${yamlScalar(subVal)}`);
			}
		} else {
			lines.push(`${key}: ${yamlScalar(value)}`);
		}
	}

	return `${lines.join("\n")}\n`;
}

/** Format a value as a YAML scalar. */
function yamlScalar(value: unknown): string {
	if (value === null || value === undefined) return "null";
	if (typeof value === "boolean") return value ? "true" : "false";
	if (typeof value === "number") return String(value);
	if (typeof value === "string") {
		// Quote strings that could be misinterpreted
		if (
			value === "" ||
			value === "null" ||
			value === "true" ||
			value === "false" ||
			/^[\d]/.test(value) ||
			/[:#{}[\],&*?|>!%@`]/.test(value) ||
			/\s/.test(value)
		) {
			return `"${value.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
		}
		return value;
	}
	return String(value);
}

/**
 * Parse a simple YAML config string into a CodeOpsConfig.
 * Handles the subset of YAML used by axiom.config.yaml:
 * top-level keys, one-level nesting, scalars.
 */
function parseConfigYaml(content: string): CodeOpsConfig {
	const config: CodeOpsConfig = {};
	const lines = content.split("\n");
	let currentSection: string | null = null;
	let currentSubSection: string | null = null;

	for (const rawLine of lines) {
		// Skip comments and empty lines
		const line = rawLine.replace(/#.*$/, "");
		if (line.trim() === "") continue;

		const indent = line.length - line.trimStart().length;
		const trimmed = line.trim();

		// Top-level key (no indent)
		if (indent === 0) {
			currentSubSection = null;
			const colonIdx = trimmed.indexOf(":");
			if (colonIdx === -1) continue;

			const key = trimmed.slice(0, colonIdx).trim();
			const rest = trimmed.slice(colonIdx + 1).trim();

			if (rest === "" || rest === "|" || rest === ">") {
				// Section header
				currentSection = key;
				if (!(key in config) || typeof config[key] !== "object") {
					config[key] = {};
				}
			} else {
				// Top-level scalar
				currentSection = null;
				config[key] = parseYamlValue(rest);
			}
		} else if (indent >= 2 && currentSection) {
			const colonIdx = trimmed.indexOf(":");
			if (colonIdx === -1) continue;

			const key = trimmed.slice(0, colonIdx).trim();
			const rest = trimmed.slice(colonIdx + 1).trim();

			if (indent >= 4 && currentSubSection) {
				// Sub-sub-section value (e.g., confidence.weights.test_signal)
				const section = config[currentSection] as Record<string, unknown>;
				if (
					section &&
					typeof section[currentSubSection] === "object" &&
					section[currentSubSection] !== null
				) {
					(section[currentSubSection] as Record<string, unknown>)[key] =
						parseYamlValue(rest);
				}
			} else if (rest === "" || rest === "|" || rest === ">") {
				// Sub-section header (e.g., confidence.weights)
				currentSubSection = key;
				const section = config[currentSection] as Record<string, unknown>;
				if (section) {
					section[key] = {};
				}
			} else {
				// Section value
				currentSubSection = null;
				const section = config[currentSection] as Record<string, unknown>;
				if (section) {
					section[key] = parseYamlValue(rest);
				}
			}
		}
	}

	return config;
}

/** Parse a YAML scalar value string. */
function parseYamlValue(raw: string): unknown {
	const trimmed = raw.trim();
	if (trimmed === "null" || trimmed === "~") return null;
	if (trimmed === "true") return true;
	if (trimmed === "false") return false;

	// Quoted string
	if (
		(trimmed.startsWith('"') && trimmed.endsWith('"')) ||
		(trimmed.startsWith("'") && trimmed.endsWith("'"))
	) {
		return trimmed.slice(1, -1);
	}

	// Number
	const num = Number(trimmed);
	if (!Number.isNaN(num) && trimmed !== "") return num;

	return trimmed;
}

// ── Public API ──────────────────────────────────────────────────────────

/**
 * Validate a Jira project key against the standard format.
 * REQ-PLG-047 §2: ^[A-Z][A-Z0-9_]{1,9}$
 *
 * @param key - The project key to validate
 * @returns true if valid, false otherwise
 */
export function validateJiraProjectKey(key: string): boolean {
	return /^[A-Z][A-Z0-9_]{1,9}$/.test(key);
}

/**
 * Read .axiom/axiom.config.yaml from a repo directory.
 *
 * @param repoRoot - Absolute path to the repository root
 * @returns Parsed config, or null if the file does not exist
 */
export function readConfigYaml(repoRoot: string): CodeOpsConfig | null {
	const configPath = join(repoRoot, CONFIG_RELATIVE_PATH);
	if (!existsSync(configPath)) return null;

	try {
		const content = readFileSync(configPath, "utf-8");
		return parseConfigYaml(content);
	} catch {
		return null;
	}
}

/**
 * Write a CodeOpsConfig to .axiom/axiom.config.yaml.
 * Creates the .axiom/ directory if it does not exist.
 *
 * @param repoRoot - Absolute path to the repository root
 * @param config - The config object to write
 */
export function writeConfigYaml(repoRoot: string, config: CodeOpsConfig): void {
	const configPath = join(repoRoot, CONFIG_RELATIVE_PATH);
	const dir = join(repoRoot, ".axiom");
	mkdirSync(dir, { recursive: true });
	writeFileSync(configPath, serializeConfigYaml(config));
}

/**
 * Configure the Jira project key in .axiom/axiom.config.yaml.
 *
 * REQ-PLG-046: Scaffolds config with repo_id, jira.project_key, confidence defaults.
 *              Idempotent — does not overwrite existing config.
 * REQ-PLG-047: Validates key format, supports skip (null), updates null keys in
 *              existing configs.
 *
 * @param repoRoot - Absolute path to the repository root
 * @param projectKey - Jira project key, or null to skip
 * @returns JiraConfigResult describing what happened
 */
export async function configureJiraProjectKey(
	repoRoot: string,
	projectKey: string | null,
): Promise<JiraConfigResult> {
	// Validate non-null keys
	if (projectKey !== null && !validateJiraProjectKey(projectKey)) {
		return {
			configured: false,
			projectKey: null,
			validationError: `Invalid Jira project key "${projectKey}". Must match ^[A-Z][A-Z0-9_]{1,9}$ (e.g., PROJ, BACKEND, FE).`,
		};
	}

	// Check for existing config
	const existing = readConfigYaml(repoRoot);

	if (existing !== null) {
		const existingJiraKey = existing.jira?.project_key;

		// REQ-PLG-046: If existing config has a non-null key, skip entirely
		if (existingJiraKey !== null && existingJiraKey !== undefined) {
			return {
				configured: false,
				projectKey: null,
				skipped: true,
			};
		}

		// REQ-PLG-046: Existing config with null key — update only jira.project_key
		if (projectKey !== null) {
			existing.jira = { ...existing.jira, project_key: projectKey };
			writeConfigYaml(repoRoot, existing);
			return {
				configured: true,
				projectKey,
				updatedExisting: true,
			};
		}

		// Existing config with null key, user also skips — no change needed
		return {
			configured: false,
			projectKey: null,
			skipped: true,
		};
	}

	// No existing config — scaffold a new one
	const config: CodeOpsConfig = {
		repo_id: deriveRepoId(repoRoot),
		jira: { project_key: projectKey },
		confidence: DEFAULT_CONFIDENCE,
	};

	writeConfigYaml(repoRoot, config);

	if (projectKey === null) {
		return {
			configured: false,
			projectKey: null,
		};
	}

	return {
		configured: true,
		projectKey,
	};
}

/**
 * Configure GitHub settings in .axiom/axiom.config.yaml.
 *
 * REQ-PLG-047 §5: Offers to set github.default_base_branch when it differs
 *                  from "main". Does not overwrite existing settings.
 *
 * @param repoRoot - Absolute path to the repository root
 * @param options - GitHub settings options
 * @returns GithubConfigResult describing what happened
 */
export async function configureGithubSettings(
	repoRoot: string,
	options: GithubSettingsOptions,
): Promise<GithubConfigResult> {
	const { defaultBranch } = options;

	// Skip if null (user skipped) or "main" (default, no config needed)
	if (defaultBranch === null) {
		return {
			configured: false,
			skipped: true,
			reason: "GitHub configuration skipped by user",
		};
	}

	if (defaultBranch === "main") {
		return {
			configured: false,
			skipped: true,
			reason: "Default base branch is already main — no configuration needed",
		};
	}

	// Check for existing config
	const existing = readConfigYaml(repoRoot);

	if (existing !== null) {
		// Don't overwrite existing github.default_base_branch
		if (existing.github?.default_base_branch !== undefined) {
			return {
				configured: false,
				skipped: true,
				reason: `github.default_base_branch already set to "${existing.github.default_base_branch}"`,
			};
		}

		// Add github section to existing config
		existing.github = {
			...existing.github,
			default_base_branch: defaultBranch,
		};
		writeConfigYaml(repoRoot, existing);
		return {
			configured: true,
			defaultBaseBranch: defaultBranch,
		};
	}

	// No existing config — scaffold a new one with github settings
	const config: CodeOpsConfig = {
		repo_id: deriveRepoId(repoRoot),
		github: { default_base_branch: defaultBranch },
	};

	writeConfigYaml(repoRoot, config);
	return {
		configured: true,
		defaultBaseBranch: defaultBranch,
	};
}

// ── Helpers ─────────────────────────────────────────────────────────────

/**
 * Derive repo_id from git remote or directory name.
 * REQ-PLG-046: Same logic as install.py — org/repo from git remote.
 */
function deriveRepoId(repoRoot: string): string {
	try {
		const { execSync } = require("node:child_process");
		const remote = execSync("git config --get remote.origin.url", {
			cwd: repoRoot,
			encoding: "utf-8",
			stdio: ["pipe", "pipe", "pipe"],
		}).trim();

		// Parse org/repo from various URL formats
		// SSH: git@github.com:org/repo.git
		// HTTPS: https://github.com/org/repo.git
		const sshMatch = remote.match(/[:/]([^/]+\/[^/]+?)(?:\.git)?$/);
		if (sshMatch) return sshMatch[1];

		return remote;
	} catch {
		// Fallback to directory name
		const { basename } = require("node:path");
		return basename(repoRoot);
	}
}
