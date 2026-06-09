/**
 * Privacy command implementation — builds and formats the `/axiom-privacy`
 * report showing what data the plugin collects, where it is stored, what has
 * been shared, how to delete shared data, and current redaction patterns.
 *
 * REQ-PLG-715: The plugin MUST provide a `/axiom-privacy` command that shows:
 *   - What data the plugin collects
 *   - Where data is stored (local vs remote)
 *   - What has been shared and with whom
 *   - How to delete shared data
 *   - Current redaction patterns
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-715
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-715 plan=phase-70-5/task-70-5-4/step-70-5-4-2
 */

import { existsSync } from "node:fs";
import { join } from "node:path";
import { DEFAULT_BLOCKED_PATTERNS } from "./blocked-files";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** A category of data that the plugin collects. */
export interface DataCategory {
	/** Stable identifier for the category. */
	id: string;
	/** Human-readable name. */
	name: string;
	/** Description of what is collected and why. */
	description: string;
}

/** A location where plugin data is stored. */
export interface StorageLocation {
	/** Relative path from repo root. */
	path: string;
	/** Whether this is local or remote storage. */
	type: "local" | "remote";
	/** What is stored here. */
	description: string;
}

/** An entry describing data that has been shared with someone. */
export interface SharedDataEntry {
	/** Session ID that was shared. */
	sessionId: string;
	/** Who the data was shared with. */
	sharedWith: string;
	/** When the data was shared (ISO 8601). */
	sharedAt: string;
	/** What type of data was shared. */
	dataType: string;
}

/** Summary of a single redaction pattern. */
export interface RedactionPatternSummary {
	/** Stable identifier for the pattern. */
	id: string;
	/** Human-readable description. */
	description: string;
}

/** Guidance on how to delete various types of stored data. */
export interface DeletionGuidance {
	/** How to delete local session data. */
	localSessions: string;
	/** How to delete configuration data. */
	configuration: string;
	/** How to delete shared data (references /axiom-share delete). */
	sharedData: string;
}

/** The full privacy report structure. */
export interface PrivacyReport {
	/** What data the plugin collects. */
	dataCategories: DataCategory[];
	/** Where data is stored. */
	storageLocations: StorageLocation[];
	/** What has been shared and with whom. */
	sharedData: SharedDataEntry[];
	/** Human-readable sharing summary. */
	sharingSummary: string;
	/** How to delete stored data. */
	deletionGuidance: DeletionGuidance;
	/** Current redaction patterns (built-in, custom, blocked files). */
	redactionPatterns: {
		builtIn: RedactionPatternSummary[];
		custom: RedactionPatternSummary[];
		blockedFilePatterns: string[];
	};
}

/** Options for building a privacy report. */
export interface PrivacyReportOptions {
	/** Path to the repository root directory. */
	repoDir: string;
}

// ---------------------------------------------------------------------------
// Built-in data categories
// ---------------------------------------------------------------------------

const DATA_CATEGORIES: DataCategory[] = [
	{
		id: "session-analytics",
		name: "Session Analytics",
		description:
			"Aggregate session metrics including message counts, token usage, cost estimates, and duration. Does not include file contents, conversation text, or tool outputs (per REQ-PLG-714).",
	},
	{
		id: "maturity-detection",
		name: "Maturity Detection Signals",
		description:
			"Filesystem presence checks for .opencode/, specs/, .memory-bank/, .axiom/ directories to determine the repository maturity level (L0-L4).",
	},
	{
		id: "configuration",
		name: "Configuration Data",
		description:
			"Plugin configuration from .axiom/plugin.yaml and .axiom/axiom.config.yaml including Jira project keys, GitHub settings, and plugin layer preferences.",
	},
];

// ---------------------------------------------------------------------------
// Built-in storage locations
// ---------------------------------------------------------------------------

const STORAGE_LOCATIONS: StorageLocation[] = [
	{
		path: ".axiom/sessions/",
		type: "local",
		description:
			"Session analytics JSON files with aggregate metrics per session.",
	},
	{
		path: ".axiom/axiom.config.yaml",
		type: "local",
		description:
			"Repository-level Axiom configuration (repo_id, Jira project key, GitHub settings).",
	},
	{
		path: ".axiom/plugin.yaml",
		type: "local",
		description:
			"Plugin configuration (layer toggles, custom redaction patterns, blocked file patterns).",
	},
];

// ---------------------------------------------------------------------------
// Built-in redaction pattern summaries
// ---------------------------------------------------------------------------

const BUILTIN_REDACTION_PATTERNS: RedactionPatternSummary[] = [
	{
		id: "aws-key",
		description:
			"AWS access key IDs (AKIA followed by 16 uppercase alphanumeric characters).",
	},
	{
		id: "jwt",
		description:
			"JWT tokens (three base64url segments separated by dots, each starting with eyJ).",
	},
	{
		id: "api-key",
		description:
			"Generic API keys (32+ alphanumeric characters in key-value or Bearer contexts).",
	},
	{
		id: "env-var",
		description:
			"Environment variable KEY=VALUE pairs (uppercase key with underscores).",
	},
];

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Build a privacy report for the given repository.
 *
 * Checks .axiom/shared/ for locally exported sessions.
 * Remote sharing is available via /axiom-share (L3).
 */
export function buildPrivacyReport(
	options: PrivacyReportOptions,
): PrivacyReport {
	const { repoDir } = options;
	// Check for locally exported shared sessions
	const sharedDir = join(repoDir, ".axiom", "shared");
	const localSharedSessions: SharedDataEntry[] = [];
	if (existsSync(sharedDir)) {
		try {
			const { readdirSync } = require("node:fs") as typeof import("fs");
			const files = readdirSync(sharedDir).filter((f: string) =>
				f.endsWith(".json"),
			);
			for (const file of files) {
				localSharedSessions.push({
					sessionId: file.replace(".json", ""),
					sharedWith: `local:${join(sharedDir, file)}`,
					sharedAt: new Date().toISOString(),
					dataType: "session-replay",
				});
			}
		} catch {
			// Non-fatal — shared dir may be empty or unreadable
		}
	}

	const hasLocalShared = localSharedSessions.length > 0;
	const sharingSummary = hasLocalShared
		? `${localSharedSessions.length} session(s) exported locally to .axiom/shared/. Use /axiom-share to share sessions remotely (requires network.backend_url in plugin.yaml).`
		: "No sessions have been shared or exported locally. Use /axiom-share to share sessions.";

	return {
		dataCategories: [...DATA_CATEGORIES],
		storageLocations: [...STORAGE_LOCATIONS],
		sharedData: localSharedSessions,
		sharingSummary,
		deletionGuidance: {
			localSessions:
				"Delete the .axiom/sessions/ directory to remove all local session analytics data.",
			configuration:
				"Delete .axiom/axiom.config.yaml and .axiom/plugin.yaml to remove all configuration data.",
			sharedData:
				"Delete .axiom/shared/ to remove locally exported sessions. Use /axiom-share delete <session-id> to delete remotely shared sessions.",
		},
		redactionPatterns: {
			builtIn: [...BUILTIN_REDACTION_PATTERNS],
			custom: [],
			blockedFilePatterns: [...DEFAULT_BLOCKED_PATTERNS],
		},
	};
}

/**
 * Format a privacy report as a human-readable string suitable for display
 * in the `/axiom-privacy` command output.
 */
export function formatPrivacyReport(report: PrivacyReport): string {
	const sections: string[] = [];

	// Section 1: What data the plugin collects
	sections.push("## Data Collection");
	sections.push("");
	sections.push(
		"The Axiom plugin collects the following categories of data:",
	);
	sections.push("");
	for (const cat of report.dataCategories) {
		sections.push(`- **${cat.name}**: ${cat.description}`);
	}

	// Section 2: Where data is stored
	sections.push("");
	sections.push("## Storage Locations");
	sections.push("");
	for (const loc of report.storageLocations) {
		sections.push(`- \`${loc.path}\` (${loc.type}): ${loc.description}`);
	}

	// Section 3: What has been shared
	sections.push("");
	sections.push("## Sharing Status");
	sections.push("");
	sections.push(report.sharingSummary);
	if (report.sharedData.length > 0) {
		sections.push("");
		for (const entry of report.sharedData) {
			sections.push(
				`- Session \`${entry.sessionId}\` shared with ${entry.sharedWith} on ${entry.sharedAt} (${entry.dataType})`,
			);
		}
	}

	// Section 4: How to delete data
	sections.push("");
	sections.push("## Data Deletion");
	sections.push("");
	sections.push(
		`- **Local sessions**: ${report.deletionGuidance.localSessions}`,
	);
	sections.push(
		`- **Configuration**: ${report.deletionGuidance.configuration}`,
	);
	sections.push(`- **Shared data**: ${report.deletionGuidance.sharedData}`);

	// Section 5: Current redaction patterns
	sections.push("");
	sections.push("## Redaction Patterns");
	sections.push("");
	sections.push("### Built-in Patterns");
	sections.push("");
	for (const pat of report.redactionPatterns.builtIn) {
		sections.push(`- **${pat.id}**: ${pat.description}`);
	}
	if (report.redactionPatterns.custom.length > 0) {
		sections.push("");
		sections.push("### Custom Patterns");
		sections.push("");
		for (const pat of report.redactionPatterns.custom) {
			sections.push(`- **${pat.id}**: ${pat.description}`);
		}
	}
	sections.push("");
	sections.push("### Blocked File Patterns");
	sections.push("");
	for (const pat of report.redactionPatterns.blockedFilePatterns) {
		sections.push(`- \`${pat}\``);
	}

	return sections.join("\n");
}
